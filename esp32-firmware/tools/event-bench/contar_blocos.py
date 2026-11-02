#!/usr/bin/env python3
"""Reconstrói, a partir de um log do capture.py, quantos blocos o log da flash guarda.

    contar_blocos.py logs/limite.log [--a-cada 10]

O firmware não emite a contagem de blocos nem o `usagePermille`: este script
refaz os segmentos pelos eventos `BOOT` (lê `log_max` = N), `SEG_NEW`,
`FLUSH ok=1`, `SEG_DROP` e `SYNC_FREE`, e imprime blocos, N e usage
(blocos × 1000 / N, a mesma conta do `LittleFsBlockStore`). A captura precisa
começar logo depois de um wipe (`BOOT wiped=1`); fora disso o estado inicial é
desconhecido e a conferência do `SEG_DROP` marca `DIVERGE`. Só biblioteca padrão.
Usado pelo roteiro `docs/roteiro-teste-limite-blocos.md`.
"""
import argparse
import re
import sys

EVENT = re.compile(r"EV \d+ \d+ (\w+)(.*)")
FIELD = re.compile(r"(\w+)=(\S+)")


def fields(text):
    return dict(FIELD.findall(text))


class Log:
    def __init__(self):
        self.segs = []      # [[first, count], ...], do mais antigo ao mais novo
        self.n = None       # log_max do último BOOT
        self.flushes = 0
        self.diverge = 0
        self.this_boot = []  # segmentos abertos desde o último FORMAT

    def open_segment(self, first):
        seg = [first, 0]
        self.segs.append(seg)
        self.this_boot.append(seg)

    def blocks(self):
        return sum(c for _, c in self.segs)

    def show(self, tag):
        b = self.blocks()
        usage = b * 1000 // self.n if self.n else "?"
        print(f"{tag:<16} blocos={b:<5} N={self.n} usage={usage} segs={[tuple(s) for s in self.segs]}")

    def feed(self, name, f, every):
        if name == "BOOT":
            self.n = int(f["log_max"]) if "log_max" in f else None
            if f.get("wiped") == "1":
                # O log foi apagado neste boot: só valem os segmentos criados depois do FORMAT.
                self.segs = [s for s in self.segs if any(s is x for x in self.this_boot)]
            self.show("BOOT wiped=" + f.get("wiped", "?"))
        elif name == "FORMAT":  # primeiro evento de cada boot
            self.this_boot = []
        elif name == "SEG_NEW":
            self.open_segment(int(f["first"]))
        elif name == "FLUSH" and f.get("ok") == "1":
            if not self.segs:  # captura começou no meio de um segmento
                self.open_segment(int(f["seq"]))
            self.segs[-1][1] += 1
            self.flushes += 1
            if every and self.flushes % every == 0:
                self.show(f"FLUSH#{self.flushes}")
        elif name == "SEG_DROP":
            expected = [int(f["first"]), int(f["count"])]
            dropped = self.segs.pop(0) if self.segs else None
            ok = dropped == expected
            self.diverge += not ok
            self.show("SEG_DROP" + ("" if ok else f" DIVERGE (log diz {expected}, script tinha {dropped})"))
        elif name == "SYNC_FREE":
            for _ in range(int(f["segs"])):
                if self.segs:
                    self.segs.pop(0)
            self.show("SYNC_FREE")


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("log", help="arquivo gerado pelo capture.py")
    ap.add_argument("--a-cada", type=int, default=10, help="imprime a cada N FLUSH (0 = só nos eventos de segmento)")
    args = ap.parse_args()

    log = Log()
    with open(args.log, encoding="utf-8") as fh:
        for raw in fh:
            line = raw.split("\t", 1)[-1].strip()
            m = EVENT.match(line)
            if m:
                log.feed(m.group(1), fields(m.group(2)), args.a_cada)
    log.show("FIM")
    return 1 if log.diverge else 0


if __name__ == "__main__":
    sys.exit(main())
