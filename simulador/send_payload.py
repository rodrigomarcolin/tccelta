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
        nome amigável, ou mensagens do canal de DTC, que tem outro formato).
        Ex.: 0C2EE00000

    list
        Lista todos os PIDs suportados (código hex + nome amigável).

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


def resolve_payload(line):
    """Converte um comando digitado em payload hex a enviar, ou None se não
    reconhecido. Aceita `NOME=valor` (ex.: RPM=3000) ou um payload hex bruto
    já pronto (ex.: para comandos de DTC, que têm outro formato)."""
    if "=" in line:
        name, _, raw_value = line.partition("=")
        name = name.strip().upper()
        if name not in PIDS:
            print(
                f"PID desconhecido: '{name}'. Use /list para ver os nomes disponíveis."
            )
            return None
        try:
            value = float(raw_value.strip().replace(",", "."))
        except ValueError:
            print(f"Valor inválido: '{raw_value.strip()}'")
            return None
        pid, encode = PIDS[name]
        return build_pid_payload(pid, encode(value))

    # Payload hex já pronto (ex.: comando de DTC, ou "0C2EE00000" manual)
    return line


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
                print("  NOME=valor            ex.: RPM=3000, SPEED=80, COOLANT_TEMP=90")
                print("  NOME=valor;NOME=valor ex.: RPM=3000;SPEED=80;COOLANT_TEMP=90")
                print("  <10 hex>              payload bruto (PID + 4 bytes), ex.: 0C2EE00000")
                print("  list                  lista todos os PIDs e seus nomes")
                print("  close                 fecha a porta serial e encerra")
                continue

            if command == "list":
                for name, (pid, _) in sorted(PIDS.items(), key=lambda kv: kv[1][0]):
                    print(f"  0x{pid:02X}  {name}")
                continue

            for segment in line.split(";"):
                segment = segment.strip()
                if not segment:
                    continue

                payload = resolve_payload(segment)
                if payload is None:
                    continue

                ser.write(payload.encode() + b"\n")
                print(f"Enviado: {payload}")
    except KeyboardInterrupt:
        pass
    finally:
        ser.close()
        print("Porta serial fechada.")


if __name__ == "__main__":
    main()
