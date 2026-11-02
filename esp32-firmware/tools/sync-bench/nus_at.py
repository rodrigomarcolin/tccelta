#!/usr/bin/env python3
"""Envia um comando AT# ao dongle pelo NUS (BLE) e imprime a resposta.

    nus_at.py 'AT#SYNC?' [--name TCCeltaDongle] [--address AA:BB:..] [--espera 3]

Os comandos de bancada (AT#SYNC?, AT#ACK n, AT#WIPE, AT#LIST, AT#FLUSH n,
AT#CORRUPT n ...) só entram pelo NUS, nunca pela serial, e só nas builds
`*_debug`. Sem pareamento, só funciona no env `collector_mock_debug_open`;
nos envs com LESC, use um terminal BLE no celular já pareado (nRF Connect).
Ponha o comando entre aspas no shell, por causa do `#` e do `?`.
Dependência: bleak. Usado pelo roteiro `docs/roteiro-teste-limite-blocos.md`.
"""
import argparse
import asyncio
import sys

from bleak import BleakClient, BleakScanner

NUS_RX = "6E400002-B5A3-F393-E0A9-E50E24DCCA9E"  # escrita: comando + \r
NUS_TX = "6E400003-B5A3-F393-E0A9-E50E24DCCA9E"  # notificação: resposta


async def run(args):
    if args.address:
        target = args.address
    else:
        target = await BleakScanner.find_device_by_filter(lambda d, _: d.name == args.name, timeout=15)
        if target is None:
            raise SystemExit(f"dongle {args.name!r} não encontrado")
    async with BleakClient(target) as client:
        await client.start_notify(NUS_TX, lambda _, data: print(data.decode(errors="replace"), end="", flush=True))
        await client.write_gatt_char(NUS_RX, (args.comando + "\r").encode(), response=False)
        await asyncio.sleep(args.espera)
    print()


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("comando", help="ex.: 'AT#SYNC?'")
    ap.add_argument("--name", default="TCCeltaDongle")
    ap.add_argument("--address")
    ap.add_argument("--espera", type=float, default=3, help="segundos aguardando a resposta")
    args = ap.parse_args()
    if not args.comando.upper().startswith("AT"):
        raise SystemExit("o comando precisa começar com AT")
    asyncio.run(run(args))


if __name__ == "__main__":
    sys.exit(main())
