#!/usr/bin/env python3
"""Verifica no host a assinatura ECDSA P-256 de um bloco do perfil `asym`
(spec fase-4/01). Só biblioteca padrão.

    verify_block.py --pub HEX130 --dev-id HEX16 --data HEX --tag HEX128
    verify_block.py --record kms_record.json --event "EV 231 10849 FLUSH ... hex=... tag=..."
    <dump do monitor> | verify_block.py --record kms_record.json --event -

`--pub` é a chave pública de 65 B (`04 ‖ X ‖ Y`); com `--record` ela e o DevId
saem do `kms_record.json` do provision.py. `--event` aceita uma linha FLUSH (ou
`-` para ler as linhas da entrada padrão e conferir todas) de um build `*_debug`
com `hex=` e `tag=` (os builds `*_bench_*` os omitem). Sai com 0 se todas as
assinaturas verificam e 1 se alguma falha.
"""
import argparse
import hashlib
import json
import re
import sys

from vectors import LABEL, N, _add, _mul, _mul_point, _verify  # noqa: F401  (aritmética P-256 do gerador)


def block_hash(dev_id: bytes, data: bytes) -> bytes:
    return hashlib.sha256(LABEL + b"\x03" + dev_id + data).digest()


def verify_block(pub: bytes, dev_id: bytes, data: bytes, tag: bytes) -> bool:
    if len(pub) != 65 or pub[0] != 4 or len(tag) != 64:
        return False
    return _verify(pub, block_hash(dev_id, data), tag)


def parse_event(line: str):
    """(seq, data, tag) de uma linha FLUSH com hex= e tag=, ou None."""
    if " FLUSH " not in line:
        return None
    field = lambda k: (re.search(rf"\b{k}=([0-9A-Fa-f]+)\b", line) or [None, None])[1]
    seq, data, tag = re.search(r"\bseq=(\d+)", line), field("hex"), field("tag")
    if not (seq and data and tag):
        return None
    return int(seq[1]), bytes.fromhex(data), bytes.fromhex(tag)


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--record", help="kms_record.json (dev_id e k_sig_public)")
    ap.add_argument("--pub", help="chave pública, 65 B em hexa")
    ap.add_argument("--dev-id", help="DevId, 8 B em hexa")
    ap.add_argument("--data", help="área de dados do bloco, em hexa")
    ap.add_argument("--tag", help="tag r‖s, 64 B em hexa")
    ap.add_argument("--event", help="linha FLUSH do monitor, ou - para ler da entrada padrão")
    a = ap.parse_args()

    pub, dev = a.pub, a.dev_id
    if a.record:
        rec = json.load(open(a.record, encoding="utf-8"))
        pub, dev = pub or rec["k_sig_public"], dev or rec["dev_id"]
    if not (pub and dev):
        ap.error("informe --record ou --pub e --dev-id")
    pub_b, dev_b = bytes.fromhex(pub), bytes.fromhex(dev)

    items = []
    if a.data and a.tag:
        items.append((None, bytes.fromhex(a.data), bytes.fromhex(a.tag)))
    elif a.event:
        lines = sys.stdin if a.event == "-" else [a.event]
        items = [x for x in map(parse_event, lines) if x]
    if not items:
        ap.error("nenhum bloco para verificar (--data/--tag ou --event com hex= e tag=)")

    bad = 0
    for seq, data, tag in items:
        ok = verify_block(pub_b, dev_b, data, tag)
        bad += not ok
        print(f"seq={seq if seq is not None else '-'} len={len(data)} {'OK' if ok else 'INVÁLIDA'}")
    sys.exit(1 if bad else 0)


if __name__ == "__main__":
    main()
