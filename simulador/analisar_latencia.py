#!/usr/bin/env python3
"""Estatística da latência de resposta da ECU vista pelo dongle.

    analisar_latencia.py logs/lat.log [--pid 0C] [--base 20] [--desde 30]

Lê um log do esp32-firmware/tools/event-bench/capture.py (build *_debug) e, por PID,
resume o campo `dur` dos eventos `READ ok=1` (tempo entre o pedido e a resposta, em
ms) e dos `TMAX_SAMPLE ok=1` (`lat`). Mostra também as leituras sem resposta
(`READ ok=0`) e os `CAN_STATUS`. O `dur` inclui a latência configurada no ECUSim
mais o custo fixo do barramento e do ISO-TP (alguns ms): compare sempre contra uma
rodada de referência com `LAT.CLEAR=ALL`, ou passe --base para ver o excesso.

--desde S ignora os eventos com t < S segundos desde o boot (útil para descartar o
início da sessão). Só biblioteca padrão.
"""
import argparse
import math
import re
import statistics
from collections import defaultdict

EVENT = re.compile(r"EV (\d+) (\d+) (\w+)(.*)")
FIELD = re.compile(r"(\w+)=(\S+)")


def percentil(valores, p):
    """Posto mais próximo, a mesma regra do TmaxMeter do firmware."""
    ordenados = sorted(valores)
    return ordenados[max(1, math.ceil(p * len(ordenados))) - 1]


def resumo(valores):
    if not valores:
        return "sem amostras"
    dp = statistics.pstdev(valores) if len(valores) > 1 else 0.0
    return (f"n={len(valores):<5} min={min(valores):<4} p50={percentil(valores, .50):<4} "
            f"p95={percentil(valores, .95):<4} p99={percentil(valores, .99):<4} "
            f"max={max(valores):<4} média={statistics.fmean(valores):7.2f} dp={dp:6.2f}")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("log")
    ap.add_argument("--pid", help="só este PID (hex, ex.: 0C)")
    ap.add_argument("--base", type=float, help="baseline esperado (ms): mostra o excesso sobre ele")
    ap.add_argument("--desde", type=float, default=0.0, help="ignora t < S segundos")
    args = ap.parse_args()

    ok, falhas, tmax = defaultdict(list), defaultdict(int), defaultdict(list)
    can_status = 0
    with open(args.log, encoding="utf-8", errors="replace") as f:
        for linha in f:
            i = linha.find("EV ")
            m = EVENT.match(linha[i:]) if i >= 0 else None
            if not m:
                continue
            _, t, nome, resto = m.groups()
            if int(t) < args.desde * 1000:
                continue
            c = dict(FIELD.findall(resto))
            pid = c.get("pid", "").upper()
            if args.pid and pid and pid != args.pid.upper():
                continue
            if nome == "READ":
                if c.get("ok") == "1":
                    ok[pid].append(int(c["dur"]))
                else:
                    falhas[pid] += 1
            elif nome == "TMAX_SAMPLE" and c.get("ok") == "1":
                tmax[pid].append(int(c["lat"]))
            elif nome == "CAN_STATUS":
                can_status += 1

    print("READ ok=1 (dur, ms)")
    for pid in sorted(set(ok) | set(falhas)):
        total = len(ok[pid]) + falhas[pid]
        print(f"  {pid}  {resumo(ok[pid])}  sem resposta={falhas[pid]}/{total}")
        if args.base is not None and ok[pid]:
            print(f"      excesso sobre a base de {args.base:g} ms: média {statistics.fmean(ok[pid]) - args.base:+.2f}, "
                  f"mínimo {min(ok[pid]) - args.base:+g}")
    if tmax:
        print("TMAX_SAMPLE ok=1 (lat, ms)")
        for pid in sorted(tmax):
            print(f"  {pid}  {resumo(tmax[pid])}")
    print(f"CAN_STATUS emitidos: {can_status}")


if __name__ == "__main__":
    main()
