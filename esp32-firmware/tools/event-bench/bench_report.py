#!/usr/bin/env python3
"""Relatório de benchmark com e sem segurança a partir dos logs do `capture.py`.

    bench_report.py logs/*.log [--out-dir .]

Gera `bench.csv` (uma linha por evento relevante) e `bench.md` com as métricas
B1 a B12 da spec fase-1/06 §7.3 que dependem só do log: média, p50, p95, máx. e n.
Só biblioteca padrão; percentis por interpolação linear.
"""
import argparse
import csv
import pathlib
import re
import statistics
import sys

EVENT = re.compile(r"^(?:[\d.]+\t)?EV (\d+) (\d+) (\w+)(.*)$")
FIELD = re.compile(r"(\w+)=(\"(?:[^\"\\]|\\.)*\"|\S+)")
RELEVANT = {"FLUSH", "SYNC_BLOCK", "SYNC_TX", "HS_CHALLENGE", "HS_RESPONSE", "HS_ACK", "SYNC_NOTICE",
            "SYNC_BATCH_END", "SYNC_FREE", "SYNC_OVERFLOW", "SEG_DROP", "STACK", "HEAP", "SYNC_MTU"}


def parse(path):
    events = []
    for raw in pathlib.Path(path).read_text(encoding="utf-8", errors="replace").splitlines():
        m = EVENT.match(raw)
        if not m:
            continue
        fields = {k: v for k, v in FIELD.findall(m.group(4))}
        events.append({"n": int(m.group(1)), "t": int(m.group(2)), "name": m.group(3), **fields})
    return events


def num(e, key):
    try:
        return float(e[key])
    except (KeyError, ValueError):
        return None


def percentile(values, p):
    v = sorted(values)
    if not v:
        return None
    k = (len(v) - 1) * p / 100
    lo, hi = int(k), min(int(k) + 1, len(v) - 1)
    return v[lo] + (v[hi] - v[lo]) * (k - lo)


def summary(values):
    values = [x for x in values if x is not None]
    if not values:
        return "-"
    return (f"média {statistics.fmean(values):.1f} · p50 {percentile(values, 50):.1f} · "
            f"p95 {percentile(values, 95):.1f} · máx {max(values):.1f} · n {len(values)}")


def metrics(events):
    out = {}
    flush = [e for e in events if e["name"] == "FLUSH" and e.get("kind") == "data" and e.get("ok") == "1"]
    out["B1 sign_us por bloco"] = summary([num(e, "sign_us") for e in flush])
    out["B1 sign_us/B"] = summary([num(e, "sign_us") / num(e, "len") for e in flush if num(e, "len")])
    out["B2 crc_us (gravação)"] = summary([num(e, "crc_us") for e in flush])
    out["B2 crc_us (leitura)"] = summary([num(e, "crc_us") for e in events if e["name"] == "SYNC_BLOCK"])
    out["B3 dur (ms) do flush"] = summary([num(e, "dur") for e in flush])
    out["B3 reserve+sign+crc+write (us)"] = summary(
        [sum(num(e, k) or 0 for k in ("reserve_us", "sign_us", "crc_us", "write_us")) for e in flush])
    out["B4 HS_RESPONSE total_us"] = summary([num(e, "total_us") for e in events if e["name"] == "HS_RESPONSE"])
    out["B4 HS_CHALLENGE verify_us"] = summary([num(e, "verify_us") for e in events if e["name"] == "HS_CHALLENGE"])
    out["B4 HS_ACK verify_us"] = summary([num(e, "verify_us") for e in events if e["name"] == "HS_ACK"])
    out["B4 nvs_us"] = summary([num(e, "nvs_us") for e in events if e["name"] == "HS_RESPONSE"])
    ends = [e for e in events if e["name"] == "SYNC_BATCH_END" and num(e, "dur_us")]
    out["B6 vazão do lote (kB/s)"] = summary([num(e, "bytes") / num(e, "dur_us") * 1000 for e in ends])
    out["B8 read_us + crc_us"] = summary(
        [(num(e, "read_us") or 0) + (num(e, "crc_us") or 0) for e in events if e["name"] == "SYNC_BLOCK"])
    out["B9 custo por bloco (us)"] = summary(
        [(num(e, "sign_us") or 0) + (num(e, "crc_us") or 0) for e in flush])
    # B10: tempo do FLUSH ao primeiro HS_ACK com ack >= seq
    acks = sorted((e["t"], num(e, "ack")) for e in events if e["name"] == "HS_ACK" and e.get("result") == "ok")
    lat = []
    for e in flush:
        seq = num(e, "seq")
        t_ack = next((t for t, a in acks if t >= e["t"] and a is not None and a >= seq), None)
        if t_ack is not None:
            lat.append(t_ack - e["t"])
    out["B10 latência de confirmação (ms)"] = summary(lat)
    out["B11 SEG_DROP / SYNC_OVERFLOW"] = (f"{sum(e['name'] == 'SEG_DROP' for e in events)} / "
                                           f"{sum(e['name'] == 'SYNC_OVERFLOW' for e in events)}")
    stacks = {}
    for e in events:
        if e["name"] == "STACK" and num(e, "free") is not None:
            stacks[e.get("task", "?")] = min(stacks.get(e.get("task", "?"), 1e9), num(e, "free"))
    out["B12 pilha livre mínima (B)"] = ", ".join(f"{k} {v:.0f}" for k, v in sorted(stacks.items())) or "-"
    heap = [num(e, "min") for e in events if e["name"] == "HEAP"]
    out["B12 heap mínima (B)"] = f"{min(heap):.0f}" if heap else "-"
    return out


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("logs", nargs="+", type=pathlib.Path)
    ap.add_argument("--out-dir", type=pathlib.Path, default=pathlib.Path("."))
    args = ap.parse_args()
    args.out_dir.mkdir(parents=True, exist_ok=True)

    rows, md = [], ["# Benchmark com e sem segurança\n"]
    for log in args.logs:
        events = parse(log)
        if not events:
            print(f"{log}: sem eventos", file=sys.stderr)
            continue
        for e in events:
            if e["name"] in RELEVANT:
                rows.append({"log": log.name, **e})
        md.append(f"## {log.name}\n\n| Métrica | Valor |\n|---|---|")
        md += [f"| {k} | {v} |" for k, v in metrics(events).items()]
        md.append("")

    columns = sorted({k for r in rows for k in r}, key=lambda k: (k not in ("log", "name", "n", "t"), k))
    with open(args.out_dir / "bench.csv", "w", newline="", encoding="utf-8") as f:
        w = csv.DictWriter(f, fieldnames=columns)
        w.writeheader()
        w.writerows(rows)
    (args.out_dir / "bench.md").write_text("\n".join(md), encoding="utf-8")
    print(f"{len(rows)} eventos -> {args.out_dir / 'bench.csv'}, {args.out_dir / 'bench.md'}")


if __name__ == "__main__":
    main()
