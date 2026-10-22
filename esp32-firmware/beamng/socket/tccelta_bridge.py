#!/usr/bin/env python3
"""BeamNG -> ECUSim bridge.

Receives tccelta.lua UDP packets from BeamNG and feeds the ECUSim firmware
(rodrigomarcolin/tccelta, simulador/ECUSim) over serial, using the firmware's
own update format (PIDUpdateSerialControl.ino): one message per PID,
    <PID 2 hex><4 value bytes, 8 hex>\n      e.g. "0C2EE00000\n"  (11 bytes)

UDP packet in : "<4s8f" = b"OBD1", rpm, engineLoad, watertemp, fuel, throttle,
                speed(m/s), turboBoostPsi, oiltemp   (36 bytes)

Usage:
  py tccelta_brigde.py                      # print only
  py tccelta_brigde.py --port COM8          # print + feed the ECU sim
  py tccelta_brigde.py --port COM8 --hz 20 --gap-ms 3
Serial needs:  pip install pyserial   (only if you use --port)
"""
import argparse
import socket
import struct
import time

HOST, PORT = "127.0.0.1", 4446
FMT = "<4s8f"
SIZE = struct.calcsize(FMT)  # 36

# --- MAF estimate (BeamNG exposes no MAF/MAP; these are assumptions, tune them) ---
DISPLACEMENT_L = 2.0
VOLUMETRIC_EFF = 0.85
INTAKE_TEMP_C = 25.0
ATM_KPA = 101.325
PSI_TO_KPA = 6.89476
IDLE_MAP_KPA = 30.0  # naturally aspirated fudge: idle -> ~30 kPa, full load -> atmospheric

PID_ORDER = (0x0C, 0x0D, 0x04, 0x49, 0x05, 0x10, 0x2F)  # fast-changing first
FULL_REFRESH_S = 2.0  # resend everything this often, to heal any dropped/garbled message


def clamp(x, lo, hi):
    return max(lo, min(hi, x))


def estimate_map_kpa(load, boost_psi):
    if boost_psi > 0:
        return ATM_KPA + boost_psi * PSI_TO_KPA
    return IDLE_MAP_KPA + clamp(load, 0, 1) * (ATM_KPA - IDLE_MAP_KPA)


def estimate_maf_gps(rpm, map_kpa):
    # speed-density: air mass per intake stroke, 4-stroke -> rpm/120 cycles per second
    rho_g_per_l = (map_kpa * 1000.0) / (287.05 * (INTAKE_TEMP_C + 273.15))
    return (rpm / 120.0) * DISPLACEMENT_L * VOLUMETRIC_EFF * rho_g_per_l


def byte(x):
    return int(clamp(round(x), 0, 255))


def word(x):
    v = int(clamp(round(x), 0, 65535))
    return (v >> 8) & 0xFF, v & 0xFF


def encode_pids(rpm, load, coolant, fuel, throttle, speed_ms, boost_psi):
    map_kpa = estimate_map_kpa(load, boost_psi)
    maf = estimate_maf_gps(rpm, map_kpa)
    return {
        0x04: (byte(load * 255),),                 # A*100/255 %
        0x05: (byte(coolant + 40),),               # A-40 C
        0x0C: word(rpm * 4),                       # (256A+B)/4 rpm
        0x0D: (byte(speed_ms * 3.6),),             # A km/h
        0x10: word(maf * 100),                     # (256A+B)/100 g/s
        0x2F: (byte(fuel * 255),),                 # A*100/255 %
        0x49: (byte(throttle * 255),),             # A*100/255 %
    }, map_kpa, maf


def build_payload(pid, value_bytes):
    """Same layout as build_pid_payload() in the repo's send_payload.py."""
    padded = (list(value_bytes) + [0, 0, 0, 0])[:4]
    return "{:02X}{:02X}{:02X}{:02X}{:02X}\n".format(pid, *padded).encode()


def drain_udp(sock):
    """Read everything queued; return the newest valid unpacked packet or None."""
    latest = None
    while True:
        try:
            data, _ = sock.recvfrom(2048)
        except BlockingIOError:
            return latest
        if len(data) != SIZE:
            print(f"bad length {len(data)} (expected {SIZE}) - struct packing differs?")
            continue
        fields = struct.unpack(FMT, data)
        if fields[0] == b"OBD1":
            latest = fields[1:]


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--port", help="serial port of the Arduino, e.g. COM8 (omit to only print)")
    ap.add_argument("--baud", type=int, default=115200)
    ap.add_argument("--hz", type=float, default=20.0, help="ECU sim refresh rate (default 20)")
    ap.add_argument("--gap-ms", type=float, default=3.0,
                    help="pause between serial messages; keeps the Uno's 64-byte RX buffer from overflowing")
    args = ap.parse_args()

    ser = None
    if args.port:
        import serial  # pip install pyserial
        ser = serial.Serial(args.port, args.baud, timeout=0)
        time.sleep(2.0)  # the Uno resets when the port opens
        print(f"serial open: {args.port} @ {args.baud}")
        print("tip: start this after the sim printed 'CAN BUS Shield init ok!'")

    sock = socket.socket(socket.AF_INET, socket.SOCK_DGRAM)
    sock.bind((HOST, PORT))
    sock.setblocking(False)
    print(f"listening on {HOST}:{PORT}, expecting {SIZE}-byte OBD1 packets")

    period = 1.0 / args.hz
    gap = args.gap_ms / 1000.0
    last_sent = {}
    last_full = 0.0
    next_send = time.time()
    last_print = 0.0
    latest = None
    rx_buf = b""

    while True:
        pkt = drain_udp(sock)
        if pkt:
            latest = pkt

        now = time.time()

        # echo anything the ECU sim prints (its error messages) so they are not missed
        if ser and ser.in_waiting:
            rx_buf += ser.read(ser.in_waiting)
            while b"\n" in rx_buf:
                line, rx_buf = rx_buf.split(b"\n", 1)
                print("[ecu]", line.decode(errors="replace").strip())

        if latest and now >= next_send:
            next_send = now + period
            rpm, load, coolant, fuel, throttle, speed, boost, oiltemp = latest
            pids, map_kpa, maf = encode_pids(rpm, load, coolant, fuel, throttle, speed, boost)
            full = now - last_full >= FULL_REFRESH_S
            if full:
                last_full = now
            if ser:
                for pid in PID_ORDER:
                    if full or last_sent.get(pid) != pids[pid]:
                        ser.write(build_payload(pid, pids[pid]))
                        last_sent[pid] = pids[pid]
                        time.sleep(gap)

            if now - last_print >= 0.25:  # console output at 4 Hz
                last_print = now
                print(
                    f"rpm={rpm:7.0f} load={load:4.2f} coolant={coolant:5.1f}C fuel={fuel:4.2f} "
                    f"thr={throttle:4.2f} speed={speed * 3.6:6.1f}km/h boost={boost:5.2f}psi "
                    f"| MAP~{map_kpa:5.1f}kPa MAF~{maf:5.2f}g/s"
                )
                print("   " + "  ".join(f"{pid:02X}:{bytes(v).hex()}" for pid, v in sorted(pids.items())))

        time.sleep(0.002)


if __name__ == "__main__":
    main()