"""Sessão interativa para configurar valores de PID no ECUSim via serial.

Mantém a porta serial aberta durante toda a sessão (evita o reset do Uno que
aconteceria se abríssemos a porta a cada comando) e converte comandos
amigáveis para o payload hex de 10 dígitos que o firmware espera
(veja simulador/ECUSim/PIDUpdateSerialControl.ino).

Uso:
    python send_payload.py --port COM8 [--baud 115200]

Comandos aceitos no prompt:
    NOME=valor
        Seta um PID pelo valor físico (RPM, km/h, °C, %, ...). O nome é
        convertido para o PID e os bytes crus via a fórmula inversa da usada
        pelo app (mobile-app/lib/src/domain/obd2/obd2_pid.dart).
        Ex.: RPM=3000

    NOME=valor;NOME=valor;...
        Vários comandos na mesma linha, separados por ";" — cada um vira uma
        mensagem serial própria (o protocolo só permite 1 PID por mensagem).
        Ex.: RPM=3000;SPEED=80;COOLANT_TEMP=90

    <10 hex>
        Payload já pronto, enviado como está (ex.: para setar um PID sem
        nome amigável, ou uma mensagem de DTC já montada manualmente).
        Ex.: 0C2EE00000

    DTC=codigo[:ECU[:LISTA]]
        Seta (adiciona) um DTC numa lista. codigo aceita o formato "P0301"
        (a letra P/C/B/U é descartada) ou hex puro "0301". ECU é E (ECM,
        padrão) ou T (TCM). LISTA é C (Confirmed, padrão), P (Pending) ou
        M (Permanent). Setar num a lista C também dispara a captura do
        freeze frame automaticamente (mesmo comportamento do firmware).
        Ex.: DTC=P0301           (Confirmed, ECM)
             DTC=P0171:E:P       (Pending, ECM)
             DTC=P0700:T         (Confirmed, TCM)

    DTC.CLEAR=codigo[:ECU[:LISTA]]
        Remove um DTC específico de uma lista. Mesmos parâmetros de DTC=.
        Ex.: DTC.CLEAR=P0301

    DTC.CLEARALL=[ECU[:LISTA]]
        Limpa uma lista inteira (ECU/LISTA com os mesmos padrões E/C).
        Use LISTA=ALL para limpar Confirmed+Pending+Permanent da ECU de
        uma vez.
        Ex.: DTC.CLEARALL=E:C
             DTC.CLEARALL=T:ALL

    MIL=ON|OFF[:ECU]
        Acende/apaga a luz de falha (MIL) de uma ECU (padrão ECU=E).
        Ex.: MIL=ON
             MIL=OFF:T

    FREEZE=codigo[:ECU]
        Força a captura do freeze frame agora, com o código de DTC dado,
        usando os valores ATUAIS do PID_Value_Map — sem mexer em nenhuma
        lista de DTC. Os parâmetros capturados no freeze frame são os PIDs
        ENGINE_LOAD, COOLANT_TEMP, RPM, SPEED, INTAKE_AIR_TEMP e THROTTLE:
        sete-os antes (com ";", igual a qualquer PID) e então force a
        captura.
        Ex.: ENGINE_LOAD=40;COOLANT_TEMP=95;RPM=2500;SPEED=60;INTAKE_AIR_TEMP=30;THROTTLE=25;FREEZE=P0301

    list
        Lista todos os PIDs suportados, o catálogo de DTCs conhecidos e os
        PIDs capturados no freeze frame.

    help
        Mostra este resumo de comandos.

    close | quit | exit
        Fecha a porta serial e encerra a sessão (o "/" na frente é opcional
        em todos os comandos acima, ex.: "/close" também funciona).
"""

import argparse
import time

import serial

# Nome amigável -> (PID, quantidade de bytes de valor, função de conversão
# valor físico -> bytes crus). As fórmulas são o inverso exato das usadas em
# mobile-app/lib/src/domain/obd2/obd2_pid.dart (Obd2Pid.decode).


def _byte(v):
    """Clampa e arredonda para um único byte (0-255)."""
    return max(0, min(255, int(round(v))))


def _word(v):
    """Clampa/arredonda para 16 bits e retorna (A, B) = (byte alto, byte baixo)."""
    raw = max(0, min(65535, int(round(v))))
    return (raw >> 8) & 0xFF, raw & 0xFF


PIDS = {
    "ENGINE_LOAD": (0x04, lambda v: (_byte(v * 2.55),)),
    "COOLANT_TEMP": (0x05, lambda v: (_byte(v + 40),)),
    "SHORT_FUEL_TRIM1": (0x06, lambda v: (_byte((v + 100) * 128 / 100),)),
    "LONG_FUEL_TRIM1": (0x07, lambda v: (_byte((v + 100) * 128 / 100),)),
    "FUEL_PRESSURE": (0x0A, lambda v: (_byte(v / 3),)),
    "INTAKE_PRESSURE": (0x0B, lambda v: (_byte(v),)),
    "RPM": (0x0C, lambda v: _word(v * 4)),
    "SPEED": (0x0D, lambda v: (_byte(v),)),
    "TIMING_ADVANCE": (0x0E, lambda v: (_byte((v + 64) * 2),)),
    "INTAKE_AIR_TEMP": (0x0F, lambda v: (_byte(v + 40),)),
    "MAF": (0x10, lambda v: _word(v * 100)),
    "THROTTLE": (0x11, lambda v: (_byte(v * 2.55),)),
    "O2_S1_VOLTAGE": (0x14, lambda v: (_byte(v * 200), 0)),
    "O2_S2_VOLTAGE": (0x15, lambda v: (_byte(v * 200), 0)),
    "ENGINE_RUNTIME": (0x1F, lambda v: _word(v)),
    "DIST_MIL": (0x21, lambda v: _word(v)),
    "EGR_COMMANDED": (0x2C, lambda v: (_byte(v * 255 / 100),)),
    "EGR_ERROR": (0x2D, lambda v: (_byte((v + 100) * 128 / 100),)),
    "EVAP_PURGE": (0x2E, lambda v: (_byte(v * 255 / 100),)),
    "FUEL_LEVEL": (0x2F, lambda v: (_byte(v * 255 / 100),)),
    "WARMUPS": (0x30, lambda v: (_byte(v),)),
    "DIST_SINCE_CLEAR": (0x31, lambda v: _word(v)),
    "EVAP_PRESSURE": (0x32, lambda v: _word(v * 4)),
    "BARO_PRESSURE": (0x33, lambda v: (_byte(v),)),
    "CATALYST_TEMP": (0x3C, lambda v: _word((v + 40) * 10)),
    "VOLTAGE": (0x42, lambda v: _word(v * 1000)),
    "ABS_LOAD": (0x43, lambda v: _word(v * 255 / 100)),
    "EQUIV_RATIO": (0x44, lambda v: _word(v * 65536 / 2)),
    "THROTTLE_REL": (0x45, lambda v: (_byte(v * 255 / 100),)),
    "AMBIENT_TEMP": (0x46, lambda v: (_byte(v + 40),)),
    "THROTTLE_B": (0x47, lambda v: (_byte(v * 255 / 100),)),
    "PEDAL_D": (0x49, lambda v: (_byte(v * 255 / 100),)),
    "THROTTLE_ACTUATOR": (0x4C, lambda v: (_byte(v * 255 / 100),)),
    "TIME_MIL_ON": (0x4D, lambda v: _word(v)),
    "TIME_SINCE_CLEAR": (0x4E, lambda v: _word(v)),
    "ETHANOL": (0x52, lambda v: (_byte(v * 255 / 100),)),
    "PEDAL_REL": (0x5A, lambda v: (_byte(v * 255 / 100),)),
    "OIL_TEMP": (0x5C, lambda v: (_byte(v + 40),)),
    "INJECTION_TIMING": (0x5D, lambda v: _word((v + 210) * 128)),
    "FUEL_RATE": (0x5E, lambda v: _word(v * 20)),
    "TORQUE_DEMAND": (0x61, lambda v: (_byte(v + 125),)),
    "TORQUE_ACTUAL": (0x62, lambda v: (_byte(v + 125),)),
    "TORQUE_REF": (0x63, lambda v: _word(v)),
}


def build_pid_payload(pid, value_bytes):
    """Monta os 10 dígitos hex esperados por PIDUpdateSerialControl.ino:
    PID(2 hex) + até 4 bytes de valor(2 hex cada), preenchendo com 00."""
    padded = (list(value_bytes) + [0, 0, 0, 0])[:4]
    return "{:02X}{:02X}{:02X}{:02X}{:02X}".format(pid, *padded)


# ── Canal de DTC/freeze frame (DTCUpdateSerialControl.ino) ─────────────────────
# Mensagem de 8 caracteres + LF (DTC_SERIAL_MSG_LENGTH = 9):
#   [0]=comando (S/R/Z/L/Q) [1]=ECU (E/T) [2]=lista (C/P/M) [3..6]=DTC hex (4
#   dígitos) ou dígito de MIL em [3]  [7]=preenchimento não usado

DTC_CATALOG = {
    0x0301: "P0301 - Cylinder 1 Misfire Detected",
    0x0171: "P0171 - System Too Lean (Bank 1)",
    0x0133: "P0133 - O2 Sensor Circuit Slow Response",
    0x0420: "P0420 - Catalyst System Efficiency Below Threshold",
    0x0700: "P0700 - Transmission Control System Malfunction",
}

# PIDs capturados no freeze frame (mesma ordem/composição de
# DTCMap_Definition.h:FREEZE_FRAME_PIDS), pelos nomes amigáveis já definidos
# em PIDS acima.
FREEZE_FRAME_PIDS = ["ENGINE_LOAD", "COOLANT_TEMP", "RPM", "SPEED", "INTAKE_AIR_TEMP", "THROTTLE"]

ECU_NAMES = {"E": "ECM", "T": "TCM"}
DTC_LIST_NAMES = {"C": "Confirmed", "P": "Pending", "M": "Permanent"}


def parse_dtc_code(text):
    """Aceita 'P0301' (letra P/C/B/U descartada) ou hex puro '0301'."""
    text = text.strip().upper()
    if not text:
        raise ValueError("código de DTC vazio")
    if text[0] in ("P", "C", "B", "U") and len(text) > 1:
        text = text[1:]
    return int(text, 16)


def parse_ecu(token):
    token = (token or "E").strip().upper()
    if token not in ECU_NAMES:
        raise ValueError(f"ECU inválida '{token}' (use E=ECM ou T=TCM)")
    return token


def parse_dtc_list(token):
    token = (token or "C").strip().upper()
    if token not in DTC_LIST_NAMES:
        raise ValueError(f"lista de DTC inválida '{token}' (use C=Confirmed, P=Pending, M=Permanent)")
    return token


def build_dtc_payload(cmd, ecu="E", list_sel="C", dtc_code=None, mil=None):
    buf = [cmd, ecu, list_sel, "0", "0", "0", "0", "0"]
    if cmd in ("S", "R", "Q"):
        buf[3:7] = list("{:04X}".format(dtc_code))
    elif cmd == "L":
        buf[3] = "1" if mil else "0"
    return "".join(buf)


def resolve_dtc_command(name, value):
    """Trata os comandos DTC=/DTC.CLEAR=/DTC.CLEARALL=/MIL=/FREEZE=.
    Retorna uma lista de payloads, ou None em caso de erro (já reportado)."""
    parts = value.split(":")
    try:
        if name in ("DTC", "DTC.CLEAR"):
            code = parse_dtc_code(parts[0])
            ecu = parse_ecu(parts[1] if len(parts) > 1 else None)
            list_sel = parse_dtc_list(parts[2] if len(parts) > 2 else None)
            cmd = "S" if name == "DTC" else "R"
            return [build_dtc_payload(cmd, ecu, list_sel, dtc_code=code)]

        if name == "DTC.CLEARALL":
            ecu = parse_ecu(parts[0] if parts and parts[0] else None)
            list_token = (parts[1] if len(parts) > 1 else "C").strip().upper()
            if list_token == "ALL":
                return [build_dtc_payload("Z", ecu, l) for l in ("C", "P", "M")]
            return [build_dtc_payload("Z", ecu, parse_dtc_list(list_token))]

        if name == "MIL":
            onoff = parts[0].strip().upper()
            if onoff in ("ON", "1", "TRUE"):
                mil = True
            elif onoff in ("OFF", "0", "FALSE"):
                mil = False
            else:
                raise ValueError(f"valor de MIL inválido '{parts[0]}' (use ON/OFF)")
            ecu = parse_ecu(parts[1] if len(parts) > 1 else None)
            return [build_dtc_payload("L", ecu, mil=mil)]

        if name == "FREEZE":
            code = parse_dtc_code(parts[0])
            ecu = parse_ecu(parts[1] if len(parts) > 1 else None)
            return [build_dtc_payload("Q", ecu, dtc_code=code)]
    except (ValueError, IndexError) as e:
        print(f"Comando {name} inválido: {e}")
        return None

    return None  # nunca alcançado; guard-clause para o linter


DTC_COMMAND_NAMES = ("DTC", "DTC.CLEAR", "DTC.CLEARALL", "MIL", "FREEZE")


def resolve_payload(line):
    """Converte um comando digitado numa lista de payloads a enviar (sem LF —
    quem chama adiciona), ou None se não reconhecido/erro (já reportado).
    Aceita `NOME=valor` (PID), os comandos de DTC/MIL/FREEZE, ou um payload
    hex bruto já pronto."""
    if "=" in line:
        name, _, raw_value = line.partition("=")
        name = name.strip().upper()
        value = raw_value.strip()

        if name in DTC_COMMAND_NAMES:
            return resolve_dtc_command(name, value)

        if name not in PIDS:
            print(
                f"PID/comando desconhecido: '{name}'. Use 'list' para ver os nomes disponíveis."
            )
            return None
        try:
            fvalue = float(value.replace(",", "."))
        except ValueError:
            print(f"Valor inválido: '{value}'")
            return None
        pid, encode = PIDS[name]
        return [build_pid_payload(pid, encode(fvalue))]

    # Payload hex já pronto ("0C2EE00000" manual, ou uma msg de DTC montada à mão)
    return [line]


def main():
    parser = argparse.ArgumentParser(
        description="Sessão interativa para enviar comandos ao ECUSim via serial"
    )
    parser.add_argument("--port", default="COM8", help="Serial port to use (default: COM8)")
    parser.add_argument(
        "--baud", type=int, default=115200, help="Baud rate (default: 115200)"
    )
    args = parser.parse_args()

    ser = serial.Serial(args.port, args.baud, timeout=1)
    time.sleep(2)  # aguarda o Uno sair do reset/bootloader causado pela abertura da porta
    print(f"Conectado em {args.port} @ {args.baud}. Digite /help para ajuda.")

    try:
        while True:
            try:
                line = input("> ").strip()
            except EOFError:
                break

            if not line:
                continue

            command = line.lstrip("/").lower()

            if command in ("close", "quit", "exit"):
                break

            if command == "help":
                print("Comandos:")
                print("  NOME=valor              ex.: RPM=3000, SPEED=80, COOLANT_TEMP=90")
                print("  NOME=valor;NOME=valor   ex.: RPM=3000;SPEED=80;COOLANT_TEMP=90")
                print("  <10 hex>                payload bruto, ex.: 0C2EE00000")
                print("  DTC=codigo[:ECU[:LISTA]]        ex.: DTC=P0301  |  DTC=P0171:E:P")
                print("  DTC.CLEAR=codigo[:ECU[:LISTA]]  ex.: DTC.CLEAR=P0301")
                print("  DTC.CLEARALL=[ECU[:LISTA|ALL]]  ex.: DTC.CLEARALL=T:ALL")
                print("  MIL=ON|OFF[:ECU]        ex.: MIL=ON  |  MIL=OFF:T")
                print("  FREEZE=codigo[:ECU]     ex.: FREEZE=P0301 (usa os PIDs já setados)")
                print("  list                    lista PIDs, catálogo de DTCs e PIDs do freeze frame")
                print("  close                   fecha a porta serial e encerra")
                continue

            if command == "list":
                print("PIDs suportados:")
                for name, (pid, _) in sorted(PIDS.items(), key=lambda kv: kv[1][0]):
                    print(f"  0x{pid:02X}  {name}")
                print("\nCatálogo de DTCs conhecidos (qualquer código hex também é aceito):")
                for code, desc in sorted(DTC_CATALOG.items()):
                    print(f"  0x{code:04X}  {desc}")
                print("\nECUs:      " + ", ".join(f"{k}={v}" for k, v in ECU_NAMES.items()))
                print("Listas:    " + ", ".join(f"{k}={v}" for k, v in DTC_LIST_NAMES.items()))
                print("Freeze frame captura os PIDs: " + ", ".join(FREEZE_FRAME_PIDS))
                continue

            for segment in line.split(";"):
                segment = segment.strip()
                if not segment:
                    continue

                payloads = resolve_payload(segment)
                if not payloads:
                    continue

                for payload in payloads:
                    ser.write(payload.encode() + b"\n")
                    print(f"Enviado: {payload}")
    except KeyboardInterrupt:
        pass
    finally:
        ser.close()
        print("Porta serial fechada.")


if __name__ == "__main__":
    main()
