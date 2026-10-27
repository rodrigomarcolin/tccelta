#!/usr/bin/env python3
"""Gerador único dos vetores de teste de segurança (spec fase-1/01 §12).

Só biblioteca padrão. Escreve `vectors.json` (ao lado deste arquivo) e, com
`--emit-header`, `esp32-firmware/test/support/vectors.h`; com `--emit-dart`,
`mobile-app/test/support/sync_vectors.dart`. `--check` falha se algum arquivo
gerado divergir do que o script produziria agora.

Consumidores: firmware (`test_crypto`), app (`bench_crypto_test.dart`) e o
backend (`testdata/vetores_firmware.json`, Fase 3).
"""
import argparse
import hashlib
import hmac
import json
import sys
import zlib
from pathlib import Path

HERE = Path(__file__).resolve().parent
FIRMWARE = HERE.parent.parent
REPO = FIRMWARE.parent

DEV_ID = bytes.fromhex("544343454C544101")  # "TCCELTA\x01"
K_AUTH = bytes(range(1, 33))
K_REC = bytes(range(0x21, 0x41))
K_SIG = bytes([0x11]) * 32
LABEL = b"MALCONG"
SPEC_PUB = ("040217E617F0B6443928278F96999E69A23A4F2C152BDF6D6CDF66E5B80282D4ED19"
            "4A7DEBCB97712D2DDA3CA85AA8765A56F45FC758599652F2897C65306E5794")

N_S = bytes(range(16))


def u32(v): return v.to_bytes(4, "little")
def u16(v): return v.to_bytes(2, "little")


# ── P-256 em Python puro (só para a chave pública de k_sig) ──────────────────
P = 0xFFFFFFFF00000001000000000000000000000000FFFFFFFFFFFFFFFFFFFFFFFF
A = P - 3
GX = 0x6B17D1F2E12C4247F8BCE6E563A440F277037D812DEB33A0F4A13945D898C296
GY = 0x4FE342E2FE1A7F9B8EE7EB4A7C0F9E162BCE33576B315ECECBB6406837BF51F5


def _add(p, q):
    if p is None: return q
    if q is None: return p
    if p[0] == q[0] and (p[1] + q[1]) % P == 0: return None
    if p == q:
        s = (3 * p[0] * p[0] + A) * pow(2 * p[1], -1, P) % P
    else:
        s = (q[1] - p[1]) * pow(q[0] - p[0], -1, P) % P
    x = (s * s - p[0] - q[0]) % P
    return x, (s * (p[0] - x) - p[1]) % P


def public_key(d: int) -> bytes:
    r, q = None, (GX, GY)
    while d:
        if d & 1: r = _add(r, q)
        q = _add(q, q)
        d >>= 1
    return b"\x04" + r[0].to_bytes(32, "big") + r[1].to_bytes(32, "big")


def mac(key, typ, payload, size):
    return hmac.new(key, LABEL + bytes([typ]) + DEV_ID + payload, hashlib.sha256).digest()[:size]


def build():
    desafio = u32(7) + N_S + u32(100) + u32(6) + b"\x00"
    desafio_diag = u32(8) + N_S + u32(100) + u32(7) + b"\x01" + u16(30)
    resposta = u32(7) + N_S + u32(65) + u32(130) + u16(3) + u32(123456)
    ack = u32(130) + u32(7) + b"\x00"
    boot = u32(192) + u16(4) + u32(812) + u16(23) + b"\xFE" + u16(0) + u32(131) + u32(191)
    loss = u32(140) + u16(0) + u32(0) + u16(23) + b"\xFD" + u16(0) + u32(140) + u32(141)
    assert len(desafio) == 29 and len(desafio_diag) == 31 and len(resposta) == 34
    assert len(ack) == 9 and len(boot) == 23 and len(loss) == 23

    handshake = [
        ("desafio", 0x01, desafio), ("desafio_diag", 0x01, desafio_diag),
        ("resposta", 0x02, resposta), ("ack", 0x04, ack)]
    blocks = [("boot", boot), ("loss", loss)]
    pub = public_key(int.from_bytes(K_SIG, "big"))
    return {
        "dev_id": DEV_ID.hex().upper(),
        "k_auth": K_AUTH.hex().upper(),
        "k_rec": K_REC.hex().upper(),
        "k_sig": K_SIG.hex().upper(),
        "k_sig_public": pub.hex().upper(),
        "handshake": [
            {"name": n, "type": t, "payload": p.hex().upper(),
             "tag": mac(K_AUTH, t, p, 16).hex().upper()} for n, t, p in handshake],
        "blocks": [
            {"name": n, "data": d.hex().upper(),
             "tag": mac(K_REC, 0x03, d, 32).hex().upper(),
             "ecdsa_hash": hashlib.sha256(LABEL + b"\x03" + DEV_ID + d).hexdigest().upper()}
            for n, d in blocks],
        "sha256": [
            {"name": "empty", "input_hex": "", "digest": hashlib.sha256(b"").hexdigest().upper()},
            {"name": "abc", "input_hex": b"abc".hex().upper(),
             "digest": hashlib.sha256(b"abc").hexdigest().upper()}],
        "hmac_rfc4231": [
            {"name": "case1", "key": ("0b" * 20).upper(), "data": b"Hi There".hex().upper(),
             "mac": hmac.new(bytes.fromhex("0b" * 20), b"Hi There", hashlib.sha256).hexdigest().upper()},
            {"name": "case2", "key": b"Jefe".hex().upper(),
             "data": b"what do ya want for nothing?".hex().upper(),
             "mac": hmac.new(b"Jefe", b"what do ya want for nothing?", hashlib.sha256).hexdigest().upper()}],
        "crc32": [{"name": "check", "input_hex": b"123456789".hex().upper(),
                   "crc": "%08X" % zlib.crc32(b"123456789")}],
    }


def self_check(v):
    exp = {
        "desafio": "5E08174F9D21CAA313D52A39DCD31943",
        "desafio_diag": "2689763B1FF2BAD8FC7955D4C7ECDEB3",
        "resposta": "3A93F9DC93EDA9E7DBAD035426EEB061",
        "ack": "84EB2C1203B2720E492DB89E04F1FAE7"}
    for h in v["handshake"]:
        assert h["tag"] == exp[h["name"]], f"vetor {h['name']} difere da spec: {h['tag']}"
    assert v["blocks"][0]["tag"] == "E55ECDF18FE15DDDF31661086B87DF51544C089829BD9DF2B44B278BAC333D8D"
    assert v["blocks"][1]["tag"] == "558FF3EEA60111C3ED541CD7BA3D1BB42CCB5A81CE6ABF71835DA7F747C0FF9C"
    assert v["blocks"][0]["ecdsa_hash"] == "35828A8792008036988697D32C62577927F779F84199216BF701CEB4226BC40D"
    assert v["crc32"][0]["crc"] == "CBF43926"
    assert v["hmac_rfc4231"][0]["mac"].lower() == \
        "b0344c61d8db38535ca8afceaf0bf12b881dc200c9833da726e9376c2e32cff7"
    assert v["hmac_rfc4231"][1]["mac"].lower() == \
        "5bdcc146bf60754e6a042426089575c75a003f089d2739839dec58b964ec3843"
    try:  # conferência obrigatória da chave pública (spec 01 §2)
        from cryptography.hazmat.primitives.asymmetric import ec
        k = ec.derive_private_key(int.from_bytes(K_SIG, "big"), ec.SECP256R1())
        pn = k.public_key().public_numbers()
        ref = b"\x04" + pn.x.to_bytes(32, "big") + pn.y.to_bytes(32, "big")
        assert ref.hex().upper() == v["k_sig_public"], "chave pública difere da biblioteca"
    except ImportError:
        print("aviso: 'cryptography' ausente; chave pública só conferida contra a constante da spec",
              file=sys.stderr)
    assert v["k_sig_public"] == SPEC_PUB, "chave pública difere da constante da spec"


def emit_header(v):
    def s(name, hexs): return f'static const char* const {name} = "{hexs}";\n'
    out = ["// GERADO por tools/provision/vectors.py — não editar.\n#pragma once\n\n"]
    for k in ("dev_id", "k_auth", "k_rec", "k_sig", "k_sig_public"):
        out.append(s("VEC_" + k.upper(), v[k]))
    for h in v["handshake"]:
        n = h["name"].upper()
        out.append(s(f"VEC_HS_{n}_PAYLOAD", h["payload"]))
        out.append(s(f"VEC_HS_{n}_TAG", h["tag"]))
        out.append(f"static const uint8_t VEC_HS_{n}_TYPE = {h['type']};\n")
    for b in v["blocks"]:
        n = b["name"].upper()
        out.append(s(f"VEC_BLK_{n}_DATA", b["data"]))
        out.append(s(f"VEC_BLK_{n}_TAG", b["tag"]))
        out.append(s(f"VEC_BLK_{n}_ECDSA_HASH", b["ecdsa_hash"]))
    for x in v["sha256"]:
        out.append(s(f"VEC_SHA256_{x['name'].upper()}_IN", x["input_hex"]))
        out.append(s(f"VEC_SHA256_{x['name'].upper()}_OUT", x["digest"]))
    for x in v["hmac_rfc4231"]:
        n = x["name"].upper()
        out.append(s(f"VEC_HMAC_{n}_KEY", x["key"]))
        out.append(s(f"VEC_HMAC_{n}_DATA", x["data"]))
        out.append(s(f"VEC_HMAC_{n}_MAC", x["mac"]))
    out.append(s("VEC_CRC32_CHECK_IN", v["crc32"][0]["input_hex"]))
    out.append(s("VEC_CRC32_CHECK_OUT", v["crc32"][0]["crc"]))
    return "".join(out)


def emit_dart(v):
    def c(name, hexs): return f"const String {name} = '{hexs}';\n"
    out = ["// GERADO por esp32-firmware/tools/provision/vectors.py — não editar.\n",
           "// ignore_for_file: constant_identifier_names\n\n"]
    for k in ("dev_id", "k_auth", "k_rec", "k_sig", "k_sig_public"):
        out.append(c("vec" + "".join(p.capitalize() for p in k.split("_")), v[k]))
    for h in v["handshake"]:
        n = "".join(p.capitalize() for p in h["name"].split("_"))
        out.append(c(f"vecHs{n}Payload", h["payload"]))
        out.append(c(f"vecHs{n}Tag", h["tag"]))
        out.append(f"const int vecHs{n}Type = {h['type']};\n")
    for b in v["blocks"]:
        n = b["name"].capitalize()
        out.append(c(f"vecBlk{n}Data", b["data"]))
        out.append(c(f"vecBlk{n}Tag", b["tag"]))
    for x in v["hmac_rfc4231"]:
        n = x["name"].capitalize()
        out.append(c(f"vecHmac{n}Key", x["key"]))
        out.append(c(f"vecHmac{n}Data", x["data"]))
        out.append(c(f"vecHmac{n}Mac", x["mac"]))
    out.append(c("vecCrc32CheckIn", v["crc32"][0]["input_hex"]))
    out.append(c("vecCrc32CheckOut", v["crc32"][0]["crc"]))
    return "".join(out)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--emit-header", action="store_true")
    ap.add_argument("--emit-dart", action="store_true")
    ap.add_argument("--check", action="store_true", help="falha se algo gerado divergir")
    a = ap.parse_args()
    v = build()
    self_check(v)
    targets = {HERE / "vectors.json": json.dumps(v, indent=2) + "\n"}
    header = FIRMWARE / "test" / "support" / "vectors.h"
    dart = REPO / "mobile-app" / "test" / "support" / "sync_vectors.dart"
    if a.emit_header or a.check:
        targets[header] = emit_header(v)
    if (a.emit_dart or a.check) and dart.parent.exists():
        targets[dart] = emit_dart(v)
    bad = 0
    for path, text in targets.items():
        if a.check:
            if not path.exists() or path.read_text(encoding="utf-8") != text:
                print(f"DIVERGE: {path}", file=sys.stderr)
                bad += 1
        else:
            path.parent.mkdir(parents=True, exist_ok=True)
            path.write_text(text, encoding="utf-8", newline="\n")
            print("escrito", path)
    sys.exit(1 if bad else 0)


if __name__ == "__main__":
    main()
