"""Gerador da caixa da PCB do TCCelta (FreeCAD 1.1, Part).

Uso (a partir da raiz do repositório):
    CAIXA_VARIANTE=universal "~/AppData/Local/Programs/FreeCAD 1.1/bin/freecadcmd.exe" montagem-caixa/scripts/gerar_caixa.py

Variantes: universal (v1 e v2 cabem), v1, v2.  Plano: .spec/caixa-pcb/README.md.
O contorno e os furos são lidos dos .kicad_pcb.  Coordenadas internas = KiCad (x leste, y SUL);
no FreeCAD o eixo Y é -y do KiCad (norte = +Y), para o modelo ficar com a mesma vista da PCB.
"""
import math
import os
import sys

import FreeCAD as App
import Part

V = App.Vector

# ---------------------------------------------------------------- caminhos
try:
    AQUI = os.path.dirname(os.path.abspath(__file__))
except NameError:  # freecadcmd às vezes não define __file__
    AQUI = os.path.abspath(os.environ.get("CAIXA_SCRIPTS", "montagem-caixa/scripts"))
sys.path.insert(0, AQUI)
RAIZ = os.path.abspath(os.path.join(AQUI, "..", ".."))
import pcb_geo  # noqa: E402

PCB_V1 = os.path.join(RAIZ, "montagem-kicad", "tccelta", "tccelta.kicad_pcb")
PCB_V2 = os.path.join(RAIZ, "montagem-kicad", "tccelta_v2", "tccelta_v2.kicad_pcb")

# ---------------------------------------------------------------- parâmetros
VARIANTE = os.environ.get("CAIXA_VARIANTE", "universal")
assert VARIANTE in ("universal", "v1", "v2")

PAREDE = 2.0
FUNDO = 2.0
TETO = 2.0
FOLGA_PCB = 1.0            # placa -> parede (oeste, norte)
FOLGA_ANTENA = 1.5         # ponta da antena -> parede leste
ZONA_SUL = 15.0            # borda sul da placa -> parede da frente (conferir com os fios)
FOLGA_ENCAIXE = 0.2        # calibrar na prova
Z_DIVISAO = 19.0
Z_PCB_BAIXO = 8.0          # fundo da PCB (boss 2 + espaçador 4 + ...)
ESP_PCB = 1.6
Z_PCB_TOPO = Z_PCB_BAIXO + ESP_PCB
FOLGA_TOPO = 3.0
LIP_L, LIP_H = 1.0, 2.0
ALT_COMP = {"universal": 37.5, "v1": 37.5, "v2": 25.0}[VARIANTE]

BOSS_D, BOSS_H, FURO_M3 = 8.0, 2.0, 3.4
# porca M3 presa por baixo (entra sob pressão num alojamento sextavado); parafuso M3 x 10 desce por cima
PORCA_SF, PORCA_H = 5.5, 2.4               # sextavado: medida entre faces, altura
ALOJ_SF, ALOJ_H = PORCA_SF + 0.15, PORCA_H + 0.1
PARAFUSO_L = 10.0
ESPACADOR_D = 6.0
CENTRO_D, CENTRO_H = ESPACADOR_D + 0.4, 0.4  # rebaixo no topo do boss que centra o espaçador
Z_ESPACADOR0 = FUNDO + BOSS_H - CENTRO_H     # o espaçador apoia no fundo do rebaixo
ESPACADOR_H = Z_PCB_BAIXO - Z_ESPACADOR0
assert Z_ESPACADOR0 - ALOJ_H >= 1.0          # plástico entre a porca e o espaçador (só compressão)
assert Z_PCB_TOPO - PARAFUSO_L <= ALOJ_H - PORCA_H  # a ponta do parafuso atravessa a porca inteira

BOTAO_X, BOTAO_Z = 56.0, 11.0   # README dizia ~54; 56 afasta o berço do dupont do J5 (v2)
BOTAO_CORPO = (12.0, 12.0, 7.3)   # largura, altura, profundidade
BOTAO_CAPA_D = 11.0
BOTAO_CAPA_H = 1.5         # ASSUMIDO (falta medir); capinha 1 mm recuada
BOTAO_RECUO = 1.0
BOTAO_FURO = BOTAO_CAPA_D + 0.6

CABO_D = 8.0               # medido (2026-11-09)
CABO_X, CABO_Z = 29.0, 11.0
JANELA_USB = True
USB_Y, USB_Z, USB_W, USB_H = 52.1, Z_PCB_TOPO + 14.0, 12.4, 9.4   # USB no meio do ESP: v1 y 51,4; v2 y 52,7

ANG_ENTRADA, ANG_SAIDA = 30.0, 50.0
DEFLEXAO_TRAVA = 0.6       # = altura do ressalto (0,8) - ponta do dente (0,2)
TEXTO = "TCCELTA"
FONTE = "C:/Windows/Fonts/arialbd.ttf"

# ---------------------------------------------------------------- geometria das placas
c1, furos1, comps1 = pcb_geo.ler_pcb(PCB_V1)
c2, furos2, comps2 = pcb_geo.ler_pcb(PCB_V2)
PLACAS = {"v1": (c1, furos1, comps1), "v2": (c2, furos2, comps2)}
VERS = ["v1", "v2"] if VARIANTE == "universal" else [VARIANTE]

X_OESTE, X_LESTE_PCB = min(c1[0], c2[0]), max(c1[2], c2[2])
Y_NORTE = min(c1[1], c2[1])
Y_SUL = max(PLACAS[v][0][3] for v in VERS)
X_ANTENA = 68.3            # ponta da antena do ESP32 (README: x 16,3 a 68,3)
# corpo do ESP32 (união v1/v2): v1 fileiras a 22,86 (y 37,3 a 65,6); v2 fileiras a 25,4 (y 38,6 a 66,9)
ESP_BBOX = (16.3, 37.0, 68.3, 67.1)
Z_ESP_BAIXO = Z_PCB_TOPO + 11.0   # soquete 8,5 + plástico 2,5
DEDO_Y0, DEDO_Y1 = 44.0, 58.0     # faixa das pontas do ESP32 onde os dedos pegam (troca na feira)
assert Z_DIVISAO < Z_ESP_BAIXO

IX0, IX1 = X_OESTE - FOLGA_PCB, X_ANTENA + FOLGA_ANTENA
IY0, IY1 = Y_NORTE - FOLGA_PCB, Y_SUL + ZONA_SUL
OX0, OX1, OY0, OY1 = IX0 - PAREDE, IX1 + PAREDE, IY0 - PAREDE, IY1 + PAREDE
Z_TOPO_INT = Z_PCB_TOPO + ALT_COMP + FOLGA_TOPO
Z_TOPO = Z_TOPO_INT + TETO

# furos usados na variante (nome, x, y)
FUROS = {}
for v in VERS:
    for nome, (x, y) in PLACAS[v][1].items():
        FUROS[(round(x, 2), round(y, 2))] = f"{nome}{v}"
FUROS = [(k[0], k[1], n) for k, n in FUROS.items()]


# ---------------------------------------------------------------- helpers (coords KiCad -> FreeCAD)
def box(x0, x1, y0, y1, z0, z1):
    return Part.makeBox(x1 - x0, y1 - y0, z1 - z0, V(x0, -y1, z0))


def cil_z(x, y, z0, z1, d):
    return Part.makeCylinder(d / 2, z1 - z0, V(x, -y, z0), V(0, 0, 1))


def cil_y(x, z, y0, y1, d):
    """cilindro ao longo de y (de y0 a y1 do KiCad)."""
    return Part.makeCylinder(d / 2, y1 - y0, V(x, -y1, z), V(0, 1, 0))


def sextavado_z(x, y, z0, z1, sf):
    """prisma sextavado vertical, sf = medida entre faces."""
    r = sf / 2 / math.cos(math.radians(30))
    p = [V(x + r * math.cos(math.radians(60 * i)), -y + r * math.sin(math.radians(60 * i)), z0) for i in range(6)]
    p.append(p[0])
    return Part.Face(Part.makePolygon(p)).extrude(V(0, 0, z1 - z0))


def prisma_xz(pts, y0, y1):
    """polígono (x,z) extrudado de y0 a y1 (KiCad)."""
    p = [V(x, -y1, z) for x, z in pts]
    p.append(p[0])
    f = Part.Face(Part.makePolygon(p))
    return f.extrude(V(0, y1 - y0, 0))


def uniao(lista):
    r = lista[0]
    for s in lista[1:]:
        r = r.fuse(s)
    return r.removeSplitter()


def corte(base, lista):
    for s in lista:
        base = base.cut(s)
    return base.removeSplitter()


# ---------------------------------------------------------------- travas (perfis em d,z; d = para fora a partir do piso do rebaixo)
R_REBAIXO = 2.0
ARM_D0 = 0.9
ZB = Z_DIVISAO - 6.0
BUMP_H = 0.8
TRAVA_L = 8.0              # largura do braço
TRAVA_REBAIXO_L = TRAVA_L + 0.6
_run_ent = BUMP_H / math.tan(math.radians(ANG_ENTRADA))
_run_sai = BUMP_H / math.tan(math.radians(ANG_SAIDA))
PERFIL_RESSALTO = [(0, ZB), (BUMP_H, ZB + _run_sai), (0, ZB + _run_sai + _run_ent)]
_zt = ZB + 0.07
PERFIL_DENTE = [(ARM_D0 + 0.1, ZB + 0.65), (0.2, _zt), (0.2, _zt - 0.3),
                (ARM_D0 + 0.1, _zt - 0.3 - 0.7 / math.tan(math.radians(ANG_ENTRADA)))]
Z_BRACO_BAIXO = PERFIL_DENTE[-1][1]
Z_REBAIXO0 = Z_DIVISAO - 8.5
Z_PAD0 = Z_PCB_TOPO + 0.1   # o reforço interno nasce em cunha (45°+) acima do topo da PCB
FENDA = 1.0
Z_FENDA_TOPO = Z_DIVISAO + 5.0
TRAVAS_Y = [Y_NORTE + 6.0, Y_SUL + 4.0]   # centros (KiCad); fora da faixa da antena (y 37 a 66)
assert all(not (ESP_BBOX[1] - 3 < y < ESP_BBOX[3] + 3) for y in TRAVAS_Y)


def perfil_para(lado, perfil):
    """(d,z) -> (x,z); lado 'O' = parede oeste (fora = -x), 'L' = leste."""
    if lado == "O":
        return [(OX0 + R_REBAIXO - d, z) for d, z in perfil]
    return [(OX1 - R_REBAIXO + d, z) for d, z in perfil]


# ---------------------------------------------------------------- BASE
def fazer_base():
    s = box(OX0, OX1, OY0, OY1, 0, Z_DIVISAO)
    s = s.cut(box(IX0, IX1, IY0, IY1, FUNDO, Z_DIVISAO + 1))
    # rebordo (lip) de 1 mm na face interna, cortado nas zonas das travas, do USB e da porta
    lip = box(IX0 - LIP_L, IX1 + LIP_L, IY0 - LIP_L, IY1 + LIP_L, Z_DIVISAO, Z_DIVISAO + LIP_H)
    lip = lip.cut(box(IX0, IX1, IY0, IY1, Z_DIVISAO - 1, Z_DIVISAO + LIP_H + 1))
    cortes_lip = [box(IX0 - 3, IX1 + 3, y - TRAVA_REBAIXO_L / 2, y + TRAVA_REBAIXO_L / 2, 0, 99) for y in TRAVAS_Y]
    cortes_lip.append(box(CABO_X - 8.9, CABO_X + 8.9, IY1 - 1, IY1 + 3, 0, 99))
    if JANELA_USB:
        cortes_lip.append(box(IX0 - 3, IX0 + 1, USB_Y - USB_W / 2 - 0.5, USB_Y + USB_W / 2 + 0.5, 0, 99))
    # sem rebordo na frente da ponta da antena: a borda da base (Z_DIVISAO) fica abaixo do fundo do ESP32 e os dedos
    # alcançam a ponta dele para trocá-lo na feira
    cortes_lip.append(box(IX1 - 1, IX1 + 3, DEDO_Y0, DEDO_Y1, 0, 99))
    s = s.fuse(corte(lip, cortes_lip))
    # bosses (o furo, o alojamento da porca e o centrador são cortados no fim, depois das pontes de baixo)
    for x, y, _ in FUROS:
        s = s.fuse(cil_z(x, y, FUNDO, FUNDO + BOSS_H, BOSS_D))
    # travas: rebaixo externo + reforço interno + ressalto em rampa
    for y in TRAVAS_Y:
        for lado in ("O", "L"):
            if lado == "O":
                pad = prisma_xz([(IX0, Z_PAD0), (IX0 + R_REBAIXO, Z_PAD0 + R_REBAIXO * 1.4), (IX0 + R_REBAIXO, Z_DIVISAO), (IX0, Z_DIVISAO)],
                                y - TRAVA_REBAIXO_L / 2, y + TRAVA_REBAIXO_L / 2)
                reb = box(OX0 - 1, OX0 + R_REBAIXO, y - TRAVA_REBAIXO_L / 2, y + TRAVA_REBAIXO_L / 2, Z_REBAIXO0, Z_DIVISAO + 1)
            else:
                pad = prisma_xz([(IX1, Z_PAD0), (IX1 - R_REBAIXO, Z_PAD0 + R_REBAIXO * 1.4), (IX1 - R_REBAIXO, Z_DIVISAO), (IX1, Z_DIVISAO)],
                                y - TRAVA_REBAIXO_L / 2, y + TRAVA_REBAIXO_L / 2)
                reb = box(OX1 - R_REBAIXO, OX1 + 1, y - TRAVA_REBAIXO_L / 2, y + TRAVA_REBAIXO_L / 2, Z_REBAIXO0, Z_DIVISAO + 1)
            s = s.fuse(pad).cut(reb)
            s = s.fuse(prisma_xz(perfil_para(lado, PERFIL_RESSALTO), y - 3.5, y + 3.5))
    # porta do cabo (janela + canaletas dos trilhos), aberta para cima
    s = s.cut(box(CABO_X - 7.0, CABO_X + 7.0, IY1 - 0.1, IY1 + PAREDE + 0.1, FUNDO, Z_DIVISAO + 1))
    s = s.cut(box(CABO_X - 8.9, CABO_X + 8.9, IY1 + 0.5 - 0.1, IY1 + 1.5 + 0.1, FUNDO, Z_DIVISAO + 1))
    # furo do botão
    s = s.cut(cil_y(BOTAO_X, BOTAO_Z, IY1 - 0.5, IY1 + PAREDE + 0.5, BOTAO_FURO))
    s = s.fuse(berco_botao())
    # abraçadeiras por baixo: longe dos furos, para o alojamento da porca ficar aberto por baixo
    for yc in Y_PONTES:
        ponte = box(OX0 + 6, OX1 - 6, yc - 5, yc + 5, -4.0, 0.0)
        ponte = ponte.cut(box(OX0, OX1, yc - 2.7, yc + 2.7, -3.0, -1.0))
        s = s.fuse(ponte)
    # furo passante, alojamento sextavado da porca (aberto por baixo) e centrador do espaçador.
    # Sem removeSplitter na base: com as pontes novas ele a deixava inválida ("Unorientable shape").
    for x, y, _ in FUROS:
        s = s.cut(cil_z(x, y, -5, FUNDO + BOSS_H + 1, FURO_M3))
        s = s.cut(sextavado_z(x, y, -5, ALOJ_H, ALOJ_SF))
        s = s.cut(cil_z(x, y, FUNDO + BOSS_H - CENTRO_H, FUNDO + BOSS_H + 1, CENTRO_D))
    return s


Y_PONTES = (50.0, Y_SUL + 9.0)   # sob o meio do ESP32 e na zona da frente
assert all(abs(yc - y) > 5 + ALOJ_SF for yc in Y_PONTES for _, y, _ in FUROS)


def pos_botao():
    s_stand = BOTAO_CAPA_H - BOTAO_RECUO + (PAREDE - PAREDE)  # afastamento do corpo da face interna
    # ponta da capinha a BOTAO_RECUO da face externa: PAREDE + s - CAPA_H = RECUO
    s_stand = BOTAO_RECUO + BOTAO_CAPA_H - PAREDE
    s_stand = max(s_stand, 0.0)
    y_frente = IY1 - s_stand
    return y_frente, y_frente - BOTAO_CORPO[2]


def berco_botao():
    yf, yt = pos_botao()
    gl = 1.5
    meia = BOTAO_CORPO[0] / 2 + 0.2
    z0, z1 = BOTAO_Z - BOTAO_CORPO[1] / 2 - 0.1, BOTAO_Z + BOTAO_CORPO[1] / 2 + 0.1
    y_ini = yt - 1.5
    partes = [
        box(BOTAO_X - meia - gl, BOTAO_X - meia, y_ini, IY1, FUNDO, z1 + 0.4),
        box(BOTAO_X + meia, BOTAO_X + meia + gl, y_ini, IY1, FUNDO, z1 + 0.4),
        box(BOTAO_X - meia, BOTAO_X + meia, y_ini, IY1, FUNDO, z0),          # apoio embaixo
        box(BOTAO_X - meia, BOTAO_X - meia + 1.4, y_ini, yt, z0, z1),         # batentes por trás
        box(BOTAO_X + meia - 1.4, BOTAO_X + meia, y_ini, yt, z0, z1),
    ]
    return uniao(partes)


# ---------------------------------------------------------------- INSERTO
def fazer_inserto():
    y0 = IY1
    corpo = box(CABO_X - 6.8, CABO_X + 6.8, y0, y0 + PAREDE, FUNDO, Z_DIVISAO)
    flange = box(CABO_X - 8.8, CABO_X + 8.8, y0 + 0.6, y0 + 1.4, FUNDO, Z_DIVISAO)
    tampa = box(CABO_X - 8.8, CABO_X + 8.8, y0, y0 + 1.0, Z_DIVISAO, Z_DIVISAO + LIP_H)
    orelha = box(CABO_X - 7.0, CABO_X + 7.0, y0 - 8.0, y0, 3.4, 5.0)
    for sx in (-5.6, 5.6):
        orelha = orelha.cut(box(CABO_X + sx - 0.8, CABO_X + sx + 0.8, y0 - 6.0, y0 - 3.0, 2.5, 8))
    s = uniao([corpo, flange, tampa, orelha])
    s = s.cut(cil_y(CABO_X, CABO_Z, y0 - 1, y0 + PAREDE + 1, CABO_D + 0.4))
    return s.removeSplitter()


# ---------------------------------------------------------------- ESPAÇADORES
def fazer_espacadores():
    out = []
    for x, y, n in FUROS:
        t = cil_z(x, y, Z_ESPACADOR0, Z_PCB_BAIXO, ESPACADOR_D)
        t = t.cut(cil_z(x, y, 0, 20, FURO_M3))
        out.append((n, t))
    return out


# ---------------------------------------------------------------- TAMPA
def texto_baixo_relevo():
    comp_alvo = (OY1 - OY0) * 0.68
    alt_max = (OX1 - OX0) * 0.42
    h0 = 10.0
    ws = Part.makeWireString(TEXTO, FONTE, h0, 0)
    faces = []
    for ch in ws:
        if not ch:
            continue
        faces.append(Part.makeFace(ch, "Part::FaceMakerBullseye"))
    comp = Part.makeCompound(faces)
    bb = comp.BoundBox
    esc = min(comp_alvo / bb.XLength, alt_max / bb.YLength)
    m = App.Matrix()
    m.scale(esc, esc, 1)
    comp = comp.transformGeometry(m)
    bb = comp.BoundBox
    comp.translate(V(-(bb.XMin + bb.XMax) / 2, -(bb.YMin + bb.YMax) / 2, 0))
    comp.rotate(V(0, 0, 0), V(0, 0, 1), 90)  # linha de base ao longo do +Y (norte)
    # centro na tampa (entre norte e a parte da frente), um pouco para o lado da antena? -> centro geométrico
    cx = (OX0 + OX1) / 2
    cy = -(OY0 + OY1) / 2
    comp.translate(V(cx, cy, Z_TOPO - 0.8))
    solidos = [f.extrude(V(0, 0, 1.0)) for f in comp.Faces]
    return uniao(solidos), esc * h0


def fazer_tampa():
    s = box(OX0, OX1, OY0, OY1, Z_DIVISAO, Z_TOPO)
    s = s.cut(box(IX0, IX1, IY0, IY1, Z_DIVISAO - 1, Z_TOPO_INT))
    # rasgo do rebordo (por dentro da parede)
    g = box(IX0 - LIP_L - FOLGA_ENCAIXE, IX1 + LIP_L + FOLGA_ENCAIXE, IY0 - LIP_L - FOLGA_ENCAIXE,
            IY1 + LIP_L + FOLGA_ENCAIXE, Z_DIVISAO - 1, Z_DIVISAO + LIP_H + FOLGA_ENCAIXE)
    g = g.cut(box(IX0, IX1, IY0, IY1, 0, 99))
    s = s.cut(g)
    # texto
    txt, _ = texto_baixo_relevo()
    s = s.cut(txt)
    # ventilação: norte e oeste, perto do LM2596
    z0v, z1v = 26.5, min(36.0, Z_TOPO_INT - 3.0)
    for xc in (19.0, 26.0, 33.0, 40.0, 47.0):
        s = s.cut(box(xc - 1.25, xc + 1.25, OY0 - 1, IY0 + 1, z0v, z1v))
    for yc in (19.0, 24.0, 29.0, 34.0):
        s = s.cut(box(OX0 - 1, IX0 + 1, yc - 1.25, yc + 1.25, z0v, z1v))
    # recorte do USB (aberto na borda de baixo)
    if JANELA_USB:
        s = s.cut(box(OX0 - 1, IX0 + 1, USB_Y - USB_W / 2, USB_Y + USB_W / 2, Z_DIVISAO - 1, USB_Z + USB_H / 2))
    # braços das travas
    for y in TRAVAS_Y:
        for lado in ("O", "L"):
            xa, xb = (OX0 - 1, OX0 + PAREDE) if lado == "O" else (OX1 - PAREDE, OX1 + 1)
            for dy in (-TRAVA_L / 2 - FENDA, TRAVA_L / 2):
                s = s.cut(box(xa, xb, y + dy, y + dy + FENDA, Z_DIVISAO - 0.5, Z_FENDA_TOPO))
            # braço (parte abaixo da linha de divisão) + dente
            if lado == "O":
                braco = box(OX0, OX0 + R_REBAIXO - ARM_D0, y - TRAVA_L / 2, y + TRAVA_L / 2, Z_BRACO_BAIXO, Z_DIVISAO + 0.01)
            else:
                braco = box(OX1 - R_REBAIXO + ARM_D0, OX1, y - TRAVA_L / 2, y + TRAVA_L / 2, Z_BRACO_BAIXO, Z_DIVISAO + 0.01)
            dente = prisma_xz(perfil_para(lado, PERFIL_DENTE), y - TRAVA_L / 2, y + TRAVA_L / 2)
            s = s.fuse(braco).fuse(dente)
    return s.removeSplitter()


# ---------------------------------------------------------------- FANTASMAS
ALT_FANT = {"C1": 12.0, "D1": 3.5, "F1": 8.0, "J1": 10.0, "J2": 10.0, "J3": 10.0, "J5": 8.5,
            "JP1": 8.5, "R1": 3.0}


def fantasmas(v):
    c, furos, comps = PLACAS[v]
    out = {}
    placa = box(c[0], c[2], c[1], c[3], Z_PCB_BAIXO, Z_PCB_TOPO)
    for nome, (x, y) in furos.items():
        placa = placa.cut(cil_z(x, y, 0, 20, FURO_M3))
    out[f"placa_{v}"] = placa
    for ref, (x0, y0, x1, y1) in comps.items():
        if ref == "U2":
            x0, y0, x1, y1 = ESP_BBOX
            h = 16.0
        elif ref == "U1":
            h = 25.0
        elif ref == "U3":
            h = 37.5 if v == "v1" else 11.0
        else:
            h = ALT_FANT.get(ref, 8.0)
        out[f"{ref}_{v}"] = box(x0, x1, y0, y1, Z_PCB_TOPO, Z_PCB_TOPO + h)
    if v == "v2":   # barra de 90° do J5 + conector dupont apontando para a frente
        jx0, _, jx1, jy1 = comps["J5"]
        out["J5dupont_v2"] = box(jx0, jx1, jy1 + 0.5, c[3] + 8.0, 10.8, 13.4)
    return out


def fantasmas_dedos():
    """Espaço para os dedos pegarem o ESP32 pelas pontas (USB a oeste, antena a leste), com a tampa fora."""
    return {
        "dedo_oeste": box(IX0 + 0.5, ESP_BBOX[0] - 0.1, DEDO_Y0, DEDO_Y1, Z_ESP_BAIXO - 6.0, Z_ESP_BAIXO + 20.0),
        "dedo_leste": box(ESP_BBOX[2] + 0.1, OX1 + 10.0, DEDO_Y0, DEDO_Y1, Z_DIVISAO + 0.05, Z_ESP_BAIXO + 20.0),
    }


def fantasmas_externos():
    yf, yt = pos_botao()
    bw, bh, bd = BOTAO_CORPO
    out = {
        "botao_corpo": box(BOTAO_X - bw / 2, BOTAO_X + bw / 2, yt, yf, BOTAO_Z - bh / 2, BOTAO_Z + bh / 2),
        "botao_capa": cil_y(BOTAO_X, BOTAO_Z, yf, yf + BOTAO_CAPA_H, BOTAO_CAPA_D),
        "cabo": cil_y(CABO_X, CABO_Z, IY1 - 14, IY1 + 30, CABO_D),
    }
    return out


# ---------------------------------------------------------------- verificação
def vol_comum(a, b):
    try:
        return a.common(b).Volume
    except Exception as e:  # noqa
        return float("nan")


def main():
    saida = os.path.join(RAIZ, "montagem-caixa", VARIANTE)
    os.makedirs(saida, exist_ok=True)
    base = fazer_base()
    tampa = fazer_tampa()
    inserto = fazer_inserto()
    espac = fazer_espacadores()
    pecas = {"base": base, "tampa": tampa, "inserto": inserto}
    for n, e in espac:
        pecas[f"espacador_{n}"] = e

    print(f"=== caixa TCCelta, variante {VARIANTE} ===")
    print(f"externo: {OX1 - OX0:.1f} x {OY1 - OY0:.1f} x {Z_TOPO:.1f} mm  (Y_SUL={Y_SUL}, ALT_COMP={ALT_COMP})")
    print(f"furos: {[(n, x, y) for x, y, n in FUROS]}")
    falhas = 0
    for nome, s in pecas.items():
        ok = s.isValid() and len(s.Solids) == 1
        print(f"{nome:22s} vol {s.Volume / 1000:8.2f} cm3  valido={s.isValid()}  solidos={len(s.Solids)}")
        falhas += 0 if ok else 1

    for v in VERS:
        fan = fantasmas(v)
        fan.update(fantasmas_externos())
        print(f"--- interferência com fantasmas ({v}) ---")
        for pn in ("base", "tampa", "inserto"):
            for fn, f in fan.items():
                if fn.startswith("placa") and pn == "tampa":
                    continue
                vc = vol_comum(pecas[pn], f)
                if vc > 0.5 or vc != vc:
                    # a placa só pode tocar espaçadores; botão/cabo têm de passar pelos furos
                    print(f"  INTERFERE {pn} x {fn}: {vc:.1f} mm3")
                    falhas += 1
        # espaçadores x placa (fantasma) e componentes
        furos_v = {(round(x, 2), round(y, 2)) for x, y in PLACAS[v][1].values()}
        for n, e in espac:
            xy = [(x, y) for x, y, nn in FUROS if nn == n][0]
            if (round(xy[0], 2), round(xy[1], 2)) not in furos_v:
                continue
            for fn, f in fan.items():
                vc = vol_comum(e, f)
                if vc > 0.5:
                    print(f"  INTERFERE espacador {n} x {fn}: {vc:.1f} mm3")
                    falhas += 1
        # troca do ESP32 na feira: dedos nas duas pontas, sem a tampa
        for dn, d in fantasmas_dedos().items():
            alvos = {"base": base, "inserto": inserto}
            alvos.update({fn: f for fn, f in fan.items() if not fn.startswith(("U2_", "placa"))})
            ruins = [(an, vol_comum(d, a)) for an, a in alvos.items()]
            ruins = [(an, vc) for an, vc in ruins if vc > 0.5 or vc != vc]
            print(f"  {dn}: {'livre' if not ruins else 'BLOQUEADO por ' + str(ruins)}")
            falhas += len(ruins)
        # antena: distância mínima das peças à zona da antena
        ant = box(62.0, ESP_BBOX[2], ESP_BBOX[1], ESP_BBOX[3], Z_PCB_TOPO, Z_PCB_TOPO + 16)
        dmin = min(pecas[p].distToShape(ant)[0] for p in ("base", "tampa"))
        ok = dmin >= FOLGA_ANTENA - 0.01
        # (só ASCII nos prints: o console do freecadcmd descarta linhas com acento)
        print(f"  antena: distancia minima as paredes {dmin:.2f} mm  {'OK' if ok else 'FALHA'}")
        falhas += 0 if ok else 1
        # furos alinhados com os bosses
        for nome, (x, y) in PLACAS[v][1].items():
            alvo = cil_z(x, y, FUNDO, FUNDO + 1, 1.0)
            ok = vol_comum(base, alvo) < 1e-6 and base.distToShape(cil_z(x, y, FUNDO, FUNDO + 1, 1.0))[0] < 0.01
            # o furo é vazio: a distância do eixo ao material deve ser (FURO_M3 - 1)/2
            d = base.distToShape(Part.Vertex(V(x, -y, FUNDO + 0.5)))[0]
            ok = abs(d - FURO_M3 / 2) < 0.05
            print(f"  furo {nome}{v}: raio livre {d:.2f} (esperado {FURO_M3 / 2:.2f})  {'OK' if ok else 'FALHA'}")
            falhas += 0 if ok else 1
    print(f"TOTAL DE FALHAS: {falhas}")

    # --- documento
    doc = App.newDocument("caixa_tccelta")
    sh = doc.addObject("Spreadsheet::Sheet", "Parametros")
    linhas = [("VARIANTE", VARIANTE), ("Y_SUL", Y_SUL), ("ALT_COMP", ALT_COMP), ("ZONA_SUL", ZONA_SUL),
              ("PAREDE", PAREDE), ("FUNDO", FUNDO), ("TETO", TETO), ("FOLGA_ENCAIXE", FOLGA_ENCAIXE),
              ("Z_DIVISAO", Z_DIVISAO), ("CABO_D (medido)", CABO_D), ("BOTAO_X", BOTAO_X), ("BOTAO_Z", BOTAO_Z),
              ("ANG_SAIDA_TRAVA", ANG_SAIDA), ("DEFLEXAO_TRAVA", DEFLEXAO_TRAVA)]
    for i, (k, val) in enumerate(linhas, 1):
        sh.set(f"A{i}", k)
        sh.set(f"B{i}", str(val))
    doc.recompute()
    for nome, s in pecas.items():
        o = doc.addObject("Part::Feature", nome)
        o.Shape = s
    for v in VERS:
        fan = fantasmas(v)
        for fn, f in fan.items():
            o = doc.addObject("Part::Feature", "fantasma_" + fn)
            o.Shape = f
    for fn, f in list(fantasmas_externos().items()) + list(fantasmas_dedos().items()):
        o = doc.addObject("Part::Feature", "fantasma_" + fn)
        o.Shape = f
    doc.recompute()
    doc.saveAs(os.path.join(saida, "caixa_tccelta.FCStd"))

    # --- exportação
    import Mesh
    import MeshPart
    Part.export([doc.getObject(n) for n in pecas], os.path.join(saida, "caixa_tccelta_montagem.step"))

    def para_impressao(nome, s):
        s = s.copy()
        if nome == "tampa":
            s.rotate(V(0, 0, 0), V(1, 0, 0), 180)
        elif nome == "inserto":
            s.rotate(V(0, 0, 0), V(1, 0, 0), 90)
        bb = s.BoundBox
        s.translate(V(-bb.XMin, -bb.YMin, -bb.ZMin))
        return s

    for nome, s in pecas.items():
        p = para_impressao(nome, s)
        m = MeshPart.meshFromShape(Shape=p, LinearDeflection=0.05, AngularDeflection=0.3)
        m.write(os.path.join(saida, f"{nome}.stl"))
    print("arquivos em", saida)
    return falhas


main()
