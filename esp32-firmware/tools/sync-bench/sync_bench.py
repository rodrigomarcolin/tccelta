#!/usr/bin/env python3
"""Faz o papel de app e de servidor de bancada para validar o SYNC do dongle sem o app.

    sync_bench.py --scenario handshake|stream|replay|badtag|nosession|badack|incomplete|corrupt
                  [--name TCCeltaDongle] [--address AA:BB:..] [--keys bench_keys.json]
                  [--tlat 10] [--duration 60]

Usa as chaves fixas de bancada (ou `bench_keys.json` do provision.py). Dependência: bleak.
Cenários e resultados esperados: spec fase-1/05 §13.
"""
import argparse
import asyncio
import hashlib
import hmac
import json
import struct
import time

SYNC_RX = "7A3C0002-9B1E-4F2A-8C5D-2E6F1A0B3C4D"
SYNC_TX = "7A3C0003-9B1E-4F2A-8C5D-2E6F1A0B3C4D"
NUS_RX = "6E400002-B5A3-F393-E0A9-E50E24DCCA9E"
LABEL = b"MALCONG"
ERRORS = {1: "tag inválida", 2: "c_S antigo", 3: "ACK_seq inválido", 4: "incompleta", 5: "sem sessão", 7: "reenviar"}
DEFAULT_KEYS = {
    "dev_id": "544343454C544101",
    "k_auth": "0102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F20",
    "k_rec": "2122232425262728292A2B2C2D2E2F303132333435363738393A3B3C3D3E3F40",
}


def tlv(t, v):
    return struct.pack("<BH", t, len(v)) + v


def tag16(k_auth, dev_id, t, payload):
    return hmac.new(k_auth, LABEL + bytes([t]) + dev_id + payload, hashlib.sha256).digest()[:16]


class Assembler:
    """Remonta as notificações `C | pedaço` (bit 7 = primeiro; contador mod 128)."""

    def __init__(self):
        self.buf, self.counter = b"", None

    def feed(self, data):
        c, piece = data[0], bytes(data[1:])
        if c & 0x80:
            self.buf, self.counter = piece, 0
        elif self.counter is None or (c & 0x7F) != (self.counter + 1) % 128:
            self.buf, self.counter = b"", None
            return None
        else:
            self.buf += piece
            self.counter = c & 0x7F
        if len(self.buf) >= 3 and len(self.buf) == 3 + struct.unpack_from("<H", self.buf, 1)[0]:
            msg, self.buf = self.buf, b""
            return msg[0], msg[3:]
        return None


class Bench:
    def __init__(self, client, keys):
        self.client = client
        self.dev_id = bytes.fromhex(keys["dev_id"])
        self.k_auth = bytes.fromhex(keys["k_auth"])
        self.k_rec = bytes.fromhex(keys["k_rec"])
        self.queue = asyncio.Queue()
        self.asm = Assembler()
        self.cs = int(time.time())
        self.head = self.tail = self.ack = 0
        self.last_read = 0

    def on_notify(self, _, data):
        msg = self.asm.feed(data)
        if msg:
            self.queue.put_nowait(msg)

    async def send(self, t, v):
        await self.client.write_gatt_char(SYNC_RX, tlv(t, v), response=True)

    async def recv(self, want, timeout=5.0):
        end = time.monotonic() + timeout
        while time.monotonic() < end:
            t, v = await asyncio.wait_for(self.queue.get(), max(0.01, end - time.monotonic()))
            if t == 0x07:
                self.head = struct.unpack_from("<I", v, 0)[0]
            if t == want:
                return v
            if t == 0x06:
                print("  erro do dongle:", ERRORS.get(v[0], v[0]))
                if want == 0x06:
                    return v
        raise TimeoutError(f"sem mensagem {want:#04x}")

    def challenge(self, ack, bad_tag=False, cs=None):
        cs = self.cs + 1 if cs is None else cs
        self.cs = max(self.cs, cs)
        payload = struct.pack("<I", cs) + bytes(range(16)) + struct.pack("<IIB", ack, cs - 1, 0)
        tag = bytearray(tag16(self.k_auth, self.dev_id, 1, payload))
        if bad_tag:
            tag[0] ^= 1
        return payload + bytes(tag), cs

    async def handshake(self):
        await self.recv(0x07)  # aviso com flags 0 e 1
        t0 = time.monotonic()
        v, cs = self.challenge(self.ack)
        await self.send(0x01, v)
        resp = await self.recv(0x02)
        assert resp[:4] == struct.pack("<I", cs) and resp[4:20] == bytes(range(16)), "eco do desafio"
        assert hmac.compare_digest(resp[34:], tag16(self.k_auth, self.dev_id, 2, resp[:34])), "tag da resposta"
        self.tail, self.head = struct.unpack_from("<II", resp, 20)
        self.ack = max(self.ack, self.tail - 1)
        payload = struct.pack("<IIB", self.ack, cs, 0)
        await self.send(0x04, payload + tag16(self.k_auth, self.dev_id, 4, payload))
        await self.recv(0x07)
        print(f"handshake ok em {1000 * (time.monotonic() - t0):.0f} ms (TAIL {self.tail}, HEAD {self.head}, ACK {self.ack})")

    async def stream(self, tlat, duration):
        end = time.monotonic() + duration
        pending, last_post = [], time.monotonic()
        self.last_read = self.ack
        while time.monotonic() < end:
            if self.head > self.last_read:
                await self.send(0x05, struct.pack("<H", min(self.head - self.last_read, 0xFFFF)))
            try:
                v = await self.recv(0x03, 2.0)
            except (TimeoutError, asyncio.TimeoutError):
                v = None
            if v:
                seq = struct.unpack_from("<I", v, 0)[0]
                assert seq == self.last_read + 1 or v[12] in (0xFD, 0xFE), f"salto sem marcador em {seq}"
                length = struct.unpack_from("<H", v, 10)[0]
                tag = hmac.new(self.k_rec, LABEL + b"\x03" + self.dev_id + v[:length], hashlib.sha256).digest()
                assert hmac.compare_digest(tag, v[length:length + 32]), f"tag do bloco {seq}"
                # LOSS cobre [from, to]: o próximo seq esperado é to + 1.
                self.last_read = struct.unpack_from("<I", v, 19)[0] if v[12] == 0xFD else seq
                pending.append(seq)
            if time.monotonic() - last_post >= tlat and pending:
                self.ack, pending, last_post = self.last_read, [], time.monotonic()
                payload = struct.pack("<IIB", self.ack, self.cs, 0)
                await self.send(0x04, payload + tag16(self.k_auth, self.dev_id, 4, payload))
                print(f"  ACK {self.ack}")


async def run(args):
    from bleak import BleakClient, BleakScanner

    keys = json.load(open(args.keys)) if args.keys else DEFAULT_KEYS
    device = args.address or await BleakScanner.find_device_by_filter(lambda d, a: d.name == args.name, timeout=15)
    if device is None:
        raise SystemExit("dongle não encontrado")
    async with BleakClient(device) as client:
        bench = Bench(client, keys)
        await client.start_notify(SYNC_TX, bench.on_notify)
        s = args.scenario
        if s in ("handshake", "stream", "replay", "incomplete", "badack", "corrupt"):
            await bench.handshake()
        if s == "stream":
            await bench.stream(args.tlat, args.duration)
        elif s == "replay":
            v, _ = bench.challenge(bench.ack, cs=bench.cs)
            await bench.send(0x01, v)
            await bench.recv(0x06)
        elif s == "badtag":
            await bench.recv(0x07)
            v, _ = bench.challenge(0, bad_tag=True)
            await bench.send(0x01, v)
            await bench.recv(0x06)
        elif s == "nosession":
            await bench.recv(0x07)
            await bench.send(0x05, struct.pack("<H", 10))
            await bench.recv(0x06)
        elif s == "badack":
            payload = struct.pack("<IIB", bench.head + 1, bench.cs, 0)
            await bench.send(0x04, payload + tag16(bench.k_auth, bench.dev_id, 4, payload))
            await bench.recv(0x06)
        elif s == "incomplete":
            await bench.send(0x05, struct.pack("<H", 20))
            await bench.send(0x06, bytes([4]))
            await bench.recv(0x07)
        elif s == "corrupt":
            await client.write_gatt_char(NUS_RX, f"AT#CORRUPT {bench.ack + 1}\r".encode(), response=False)
            await bench.send(0x05, struct.pack("<H", 1))
            v = await bench.recv(0x03)
            assert v[12] == 0xFD, "esperava um bloco LOSS"
            print("LOSS recebido:", struct.unpack_from("<II", v, 15))
        print("cenário", s, "concluído")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--scenario", required=True, choices=["handshake", "stream", "replay", "badtag", "nosession", "badack", "incomplete", "corrupt"])
    ap.add_argument("--name", default="TCCeltaDongle")
    ap.add_argument("--address")
    ap.add_argument("--keys")
    ap.add_argument("--tlat", type=float, default=10)
    ap.add_argument("--duration", type=float, default=60)
    asyncio.run(run(ap.parse_args()))


if __name__ == "__main__":
    main()
