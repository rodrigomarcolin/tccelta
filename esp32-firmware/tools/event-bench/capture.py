#!/usr/bin/env python3
"""Captura da serial os eventos `EV ...` do dongle (build *_debug / *_bench_*) em um arquivo.

    capture.py --port COM5 [--baud 921600] --out logs/collector_mock_bench_hmac-regime-2026-10-27.log

Só grava as linhas que começam com `EV `, cada uma precedida do carimbo do PC
(segundos desde o início da captura) separado por TAB. Dependência: pyserial.
"""
import argparse
import pathlib
import time

import serial


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--port", required=True)
    ap.add_argument("--baud", type=int, default=921600)
    ap.add_argument("--out", type=pathlib.Path, required=True)
    args = ap.parse_args()
    args.out.parent.mkdir(parents=True, exist_ok=True)

    start = time.monotonic()
    with serial.Serial(args.port, args.baud, timeout=1) as port, open(args.out, "w", encoding="utf-8") as out:
        print(f"capturando {args.port} -> {args.out} (Ctrl+C encerra)")
        try:
            while True:
                line = port.readline().decode("utf-8", errors="replace").strip()
                if line.startswith("EV "):
                    out.write(f"{time.monotonic() - start:.3f}\t{line}\n")
                    out.flush()
        except KeyboardInterrupt:
            pass


if __name__ == "__main__":
    main()
