"""Teste hardware-in-the-loop: dongle (BLE, firmware -e twai) lendo DTC do
simulador ECUSim (injeção via canal serial de controle).

Requer dois dispositivos físicos conectados via CAN entre si:
  - Arduino rodando `simulador/ECUSim` numa porta serial (--com, default COM8).
  - ESP32 rodando o firmware `esp32-firmware` no profile `-e twai` (sem PSK —
    ver test-scripts/README.md se estiver usando um profile `_psk`) e
    anunciando via BLE.

Uso:
    python hil_dongle_dtc.py --com COM8 --scenario limpo
    python hil_dongle_dtc.py --com COM8 --scenario falha_unica
    python hil_dongle_dtc.py --com COM8 --scenario multiframe
    python hil_dongle_dtc.py --com COM8 --scenario multi_ecu
    python hil_dongle_dtc.py --com COM8 --scenario clear    # só limpa, sem consultar o dongle
    python hil_dongle_dtc.py --ble-only                     # reconsulta sem mexer no simulador
    python hil_dongle_dtc.py --com COM8 --scenario falha_unica --skip-freeze-frame  # só 03/07/0A

Por padrão, toda consulta ao dongle inclui tanto a lista de DTCs (Modo
03/07/0A) quanto o freeze frame (Modo 02, frame 0 — o único que o
protocolo real suporta): primeiro `"0202"` para descobrir qual DTC
originou o congelamento armazenado, depois um comando por PID congelado
(`"0204"`, `"0205"`, `"020C"`, `"020D"`, `"020F"`, `"0211"`). Use
`--skip-freeze-frame` para consultar só a lista de DTCs.

Protocolo serial de controle de DTC (DTCUpdateSerialControl.ino, 9 bytes):
    [0] cmd:  S(set)/R(remove)/Z(zera lista)/L(seta MIL)/Q(força freeze frame)
    [1] ecu:  E(ECM)/T(TCM)
    [2] list: C(confirmed)/P(pending)/M(permanent)
    [3..6]    DTC em 4 hex chars (ex.: 0301 = P0301)
    [7]       '-' (filler)
    [8]       '\n'

Protocolo ELM327 do freeze frame (Modo 02, dongle): comando `"02"+PID`
(ex.: `"0202"`), resposta `"42 <PID> 00 <dados...>"` — o byte do meio é o
número do frame, sempre `00` (não há histórico de frames antigos, nem no
simulador nem no dongle). `"NO DATA"` quando não há freeze frame
armazenado (cenário `limpo`) ou o PID não faz parte do conjunto
congelado.

Regra de prioridade do simulador (`captureFreezeFrame`,
`ECUStateModel.ino`): um DTC do grupo misfire/combustível (P0301, P0171 —
ver `DTC_FLAG_MISFIRE_FUEL` em `DTCMap_Definition.h`) sempre sobrescreve o
freeze frame armazenado; um DTC de prioridade baixa (ex.: P0133) só grava
se ainda não houver nenhum. No cenário `multiframe` (P0301, P0171, P0133
confirmados nessa ordem), o freeze frame final é do **P0171** (o último
de alta prioridade a ser confirmado) — não do P0301.
"""

import argparse
import asyncio
import time

import serial
from bleak import BleakClient, BleakScanner

BLE_SERVICE_UUID = "6e400001-b5a3-f393-e0a9-e50e24dcca9e"
BLE_RX_UUID = "6E400002-B5A3-F393-E0A9-E50E24DCCA9E"
BLE_TX_UUID = "6E400003-B5A3-F393-E0A9-E50E24DCCA9E"
DEVICE_NAME = "TCCeltaDongle"


# ── Canal serial de controle do simulador ────────────────────────────────

def dtc_msg(cmd, ecu, list_char, dtc_hex="0000"):
    assert len(dtc_hex) == 4
    return f"{cmd}{ecu}{list_char}{dtc_hex}-\n".encode()


def clear_all(ser):
    """Zera confirmed/pending/permanent nas duas ECUs simuladas."""
    for ecu in "ET":
        for lst in "CPM":
            ser.write(dtc_msg("Z", ecu, lst))
            time.sleep(0.03)


def apply_scenario(ser, scenario):
    clear_all(ser)
    if scenario == "limpo":
        pass  # já está limpo — freeze frame também deve vir "NO DATA"
    elif scenario == "falha_unica":
        # P0301 (alta prioridade) confirmado na ECM -> freeze frame capturado,
        # origem = P0301.
        ser.write(dtc_msg("S", "E", "C", "0301"))
        time.sleep(0.03)
    elif scenario == "multiframe":
        # 3 DTCs -> força First Frame + CF na leitura da lista. P0301 e P0171
        # são ambos de alta prioridade (misfire/combustível) e cada um
        # sobrescreve o freeze frame ao ser confirmado; P0133 é baixa
        # prioridade e não sobrescreve. Confirmados nesta ordem, o freeze
        # frame final é do P0171 (o último de alta prioridade), não do P0301.
        for dtc in ("0301", "0171", "0133"):
            ser.write(dtc_msg("S", "E", "C", dtc))
            time.sleep(0.03)
    elif scenario == "multi_ecu":
        # ECM e TCM têm freeze frame próprio e independente; o dongle só lê o
        # que responder primeiro ao broadcast (ECM, na prática) — o freeze
        # frame do TCM (origem P0700) não é visível nesta consulta.
        ser.write(dtc_msg("S", "E", "C", "0301"))  # ECM
        time.sleep(0.03)
        ser.write(dtc_msg("S", "T", "C", "0700"))  # TCM
        time.sleep(0.03)
    else:
        raise SystemExit(f"Cenário desconhecido: {scenario}")


# PIDs congelados pelo simulador no freeze frame (conjunto fixo — ver
# DTCMap_Definition.h, FREEZE_FRAME_PIDS — não há descoberta via PID 0x00
# no Modo 02, tem que ser essa lista hardcoded).
FREEZE_FRAME_PIDS = [
    ("Carga do motor", "04"),
    ("Temp. arrefecimento", "05"),
    ("RPM", "0C"),
    ("Velocidade", "0D"),
    ("Temp. ar admissão", "0F"),
    ("Posição borboleta", "11"),
]


def freeze_frame_commands():
    """Comandos ELM327 do Modo 02 (frame 0): primeiro descobre o DTC de
    origem (PID 0x02), depois cada PID congelado."""
    commands = [("Freeze frame - DTC origem (0202)", "0202")]
    for label, pid in FREEZE_FRAME_PIDS:
        commands.append((f"Freeze frame - {label} (02{pid})", f"02{pid}"))
    return commands


# ── Canal BLE de comando do dongle ────────────────────────────────────────

class DongleSession:
    def __init__(self):
        self._buffer = ""
        self._event = asyncio.Event()
        self._last = ""

    def on_notify(self, _handle, data: bytearray):
        self._buffer += bytes(data).decode(errors="replace")
        if ">" in self._buffer:
            self._last = self._buffer
            self._buffer = ""
            self._event.set()

    async def send(self, client, cmd, timeout=3.0):
        self._event.clear()
        await client.write_gatt_char(BLE_RX_UUID, (cmd + "\r").encode(), response=True)
        try:
            await asyncio.wait_for(self._event.wait(), timeout)
        except asyncio.TimeoutError:
            return "<TIMEOUT — sem resposta do dongle>"
        return self._last.replace("\r", " ").strip()


def _matches_dongle(device, adv):
    if device.name == DEVICE_NAME or adv.local_name == DEVICE_NAME:
        return True
    return BLE_SERVICE_UUID in [u.lower() for u in (adv.service_uuids or [])]


async def query_dongle(commands):
    print(f"Procurando dispositivo BLE '{DEVICE_NAME}' (por nome ou service UUID)...")
    device = await BleakScanner.find_device_by_filter(_matches_dongle, timeout=12.0)
    if device is None:
        raise SystemExit(
            f"Dispositivo '{DEVICE_NAME}' não encontrado via BLE. "
            "Confirme que o dongle está ligado, com o firmware -e twai gravado, "
            "e que nenhum outro cliente (app, nRF Connect) está conectado nele."
        )
    print(f"Encontrado: {device.address}")

    session = DongleSession()
    async with BleakClient(device) as client:
        await client.start_notify(BLE_TX_UUID, session.on_notify)
        for label, cmd in commands:
            resp = await session.send(client, cmd)
            print(f"  {label:42s} {cmd!r:8s} -> {resp!r}")
        await client.stop_notify(BLE_TX_UUID)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--com", default="COM8", help="Porta serial do simulador (default: COM8)")
    parser.add_argument("--baud", type=int, default=115200)
    parser.add_argument(
        "--scenario",
        choices=["limpo", "falha_unica", "multiframe", "multi_ecu", "clear"],
        help="Cenário de DTC a montar no simulador antes de consultar o dongle",
    )
    parser.add_argument(
        "--ble-only", action="store_true",
        help="Pula o setup do simulador — só reconsulta o dongle via BLE",
    )
    parser.add_argument(
        "--skip-freeze-frame", action="store_true",
        help="Não consulta o freeze frame (Modo 02) — só a lista de DTCs (03/07/0A)",
    )
    args = parser.parse_args()

    if not args.ble_only:
        if not args.scenario:
            raise SystemExit("Use --scenario <nome> ou --ble-only")
        print(f"Abrindo {args.com} @ {args.baud} para configurar o cenário '{args.scenario}'...")
        ser = serial.Serial(args.com, args.baud, timeout=1)
        time.sleep(2)  # aguarda o Uno sair do reset causado pela abertura da porta
        try:
            apply_scenario(ser, args.scenario)
            print("Cenário aplicado.")
        finally:
            ser.close()
        if args.scenario == "clear":
            return

    commands = [("Confirmados (03)", "03"), ("Pendentes (07)", "07"), ("Permanentes (0A)", "0A")]
    if not args.skip_freeze_frame:
        commands += freeze_frame_commands()
    asyncio.run(query_dongle(commands))


if __name__ == "__main__":
    main()
