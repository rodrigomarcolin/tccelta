"""Lê contorno, furos e envelopes de componentes de um .kicad_pcb (sem dependências)."""
import math
import re


def _blocos(txt, inicio_re):
    """Gera (início, fim) de cada s-expression de nível 1 cujo cabeçalho casa com inicio_re."""
    for m in re.finditer(inicio_re, txt):
        i = m.start()
        nivel = 0
        for j in range(i, len(txt)):
            c = txt[j]
            if c == "(":
                nivel += 1
            elif c == ")":
                nivel -= 1
                if nivel == 0:
                    yield i, j + 1
                    break


def ler_pcb(caminho):
    txt = open(caminho, encoding="utf-8").read()
    xs, ys = [], []
    for i, j in _blocos(txt, r"\t\((?:gr_line|gr_arc|gr_rect)\n"):
        b = txt[i:j]
        if '"Edge.Cuts"' not in b:
            continue
        for m in re.finditer(r"\((?:start|end|mid) ([-\d.]+) ([-\d.]+)\)", b):
            xs.append(float(m.group(1)))
            ys.append(float(m.group(2)))
    contorno = (min(xs), min(ys), max(xs), max(ys))
    furos, comps = {}, {}
    for i, j in _blocos(txt, r'\t\(footprint "'):
        b = txt[i:j]
        ref = re.search(r'\(property "Reference" "([^"]+)"', b).group(1)
        m = re.search(r"\n\t\t\(at ([-\d.]+) ([-\d.]+)(?: ([-\d.]+))?\)", b)
        fx, fy, rot = float(m.group(1)), float(m.group(2)), float(m.group(3) or 0)
        if b.startswith('\t(footprint "MountingHole'):
            furos[ref] = (fx, fy)
            continue
        pts = []
        c, s = math.cos(math.radians(rot)), math.sin(math.radians(rot))
        for pi, pj in _blocos(b, r"\t\t\(pad "):
            pb = b[pi:pj]
            pm = re.search(r"\(at ([-\d.]+) ([-\d.]+)(?: ([-\d.]+))?\)", pb)
            sm = re.search(r"\(size ([-\d.]+) ([-\d.]+)\)", pb)
            px, py = float(pm.group(1)), float(pm.group(2))
            r = max(float(sm.group(1)), float(sm.group(2))) / 2
            # KiCad: y para baixo; rotação positiva é anti-horária na tela
            ax = fx + px * c + py * s
            ay = fy - px * s + py * c
            pts += [(ax - r, ay - r), (ax + r, ay + r)]
        for pi, pj in _blocos(b, r"\t\t\(fp_(?:line|rect)\n"):
            pb = b[pi:pj]
            if '"F.CrtYd"' not in pb:
                continue
            for pm in re.finditer(r"\((?:start|end) ([-\d.]+) ([-\d.]+)\)", pb):
                px, py = float(pm.group(1)), float(pm.group(2))
                pts.append((fx + px * c + py * s, fy - px * s + py * c))
        for pi, pj in _blocos(b, r"\t\t\(fp_circle\n"):   # corpo redondo (ex.: C1 eletrolítico)
            pb = b[pi:pj]
            if '"F.CrtYd"' not in pb:
                continue
            cx, cy = map(float, re.search(r"\(center ([-\d.]+) ([-\d.]+)\)", pb).groups())
            ex, ey = map(float, re.search(r"\(end ([-\d.]+) ([-\d.]+)\)", pb).groups())
            r = math.hypot(ex - cx, ey - cy)
            ax, ay = fx + cx * c + cy * s, fy - cx * s + cy * c
            pts += [(ax - r, ay - r), (ax + r, ay + r)]
        if pts:
            comps[ref] = (min(p[0] for p in pts), min(p[1] for p in pts),
                          max(p[0] for p in pts), max(p[1] for p in pts))
    return contorno, furos, comps


if __name__ == "__main__":
    import sys
    c, f, k = ler_pcb(sys.argv[1])
    print(c, f)
    for r, v in sorted(k.items()):
        print(r, [round(x, 1) for x in v])
