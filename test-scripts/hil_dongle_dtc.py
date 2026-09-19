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
    python hil_dongle_dtc.py --ble-only                     # só manda 03/07/0A via BLE

Protocolo serial de controle de DTC (DTCUpdateSerialControl.ino, 9 bytes):
    [0] cmd:  S(set)/R(remove)/Z(zera lista)/L(seta MIL)/Q(força freeze frame)
    [1] ecu:  E(ECM)/T(TCM)
    [2] list: C(confirmed)/P(pending)/M(permanent)
    [3..6]    DTC em 4 hex chars (ex.: 0301 = P0301)
    [7]       '-' (filler)
    [8]       '\n'
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
        pass  # já está limpo
    elif scenario == "falha_unica":
        ser.write(dtc_msg("S", "E", "C", "0301"))  # P0301 confirmado na ECM
        time.sleep(0.03)
    elif scenario == "multiframe":
        for dtc in ("0301", "0171", "0133"):  # 3 DTCs -> força First Frame + CF
            ser.write(dtc_msg("S", "E", "C", dtc))
            time.sleep(0.03)
    elif scenario == "multi_ecu":
        ser.write(dtc_msg("S", "E", "C", "0301"))  # ECM
        time.sleep(0.03)
        ser.write(dtc_msg("S", "T", "C", "0700"))  # TCM
        time.sleep(0.03)
    else:
        raise SystemExit(f"Cenário desconhecido: {scenario}")


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
            print(f"  {label:14s} {cmd!r:6s} -> {resp!r}")
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
        help="Pula o setup do simulador — só manda 03/07/0A via BLE",
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
    asyncio.run(query_dongle(commands))


if __name__ == "__main__":
    main()
