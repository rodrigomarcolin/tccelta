# -*- coding: utf-8 -*-
"""
Gerador do dongle OBD-II (TCC) para KiCad 10.

Rode com o Python que acompanha o KiCad (tem o modulo pcbnew):

    C:/Users/davif/AppData/Local/Programs/KiCad/10.0/bin/python.exe gerar_placa.py

O script gera do zero, dentro de montagem-kicad/tccelta/:
  - tccelta.kicad_sym, tccelta.pretty/, sym-lib-table, fp-lib-table
  - tccelta.kicad_pro, tccelta.kicad_dru (regras para corrosao caseira)
  - tccelta.kicad_sch (esquematico) e tccelta.kicad_pcb (PCB roteada, face simples)
  - fabricacao/ (PDFs 1:1, furacao, renders, esquematico)
Depois roda ERC e DRC e imprime o resumo.

Placa de FACE SIMPLES: cobre so em B.Cu, componentes por cima, nenhuma via.
"""
import os
import re
import sys
import json
import uuid
import math
import shutil
import subprocess

# =====================================================================
#  MEDIDAS FISICAS  (mm)  -  CONFERIR COM PAQUIMETRO e rodar o script de novo
# =====================================================================

# --- DOIT ESP32 DevKit V1 de 30 pinos (2 x 15) ----------------------------
# CONFERIR COM PAQUIMETRO: pinos por fileira (a placa do usuario tem 15).
ESP32_PINOS_POR_FILEIRA = 15
# CONFERIR COM PAQUIMETRO: distancia centro a centro entre as duas fileiras de pinos
# (0,9" = 22,86 mm e o tipico do DOIT V1; ainda nao medido).
ESP32_ESPACAMENTO_FILEIRAS = 22.86
# CONFERIR COM PAQUIMETRO: comprimento total da placa do ESP32 (USB ate a ponta da antena).
ESP32_COMPRIMENTO = 51.5
# CONFERIR COM PAQUIMETRO: largura da placa do ESP32 (usada so no contorno/keepout).
ESP32_LARGURA = 28.3
# CONFERIR COM PAQUIMETRO: quanto a placa do ESP32 avanca alem do pino 1 (lado da antena).
# No DOIT V1 o devkit tem ~8 mm alem do pino 1.
ESP32_ANTENA_AVANCO = 8.0
# CONFERIR COM PAQUIMETRO: onde comeca a antena do WROOM, medido do centro do pino 1.
ESP32_ANTENA_INICIO_APOS_PINO1 = 1.5
ESP32_PASSO = 2.54            # passo dos pinos (padrao)
# Regra: nenhum cobre (trilha ou plano) alem de "centro do pino 1 + 1,0 mm" na direcao da antena.
ESP32_KEEPOUT_INICIO = 1.0
# Pads de soquete que NAO existem na PCB (pino sem uso na ponta da antena): o D23 (pad 16).
# Sem esse pad, o plano de GND passa entre o D22 e a borda e liga o GND do transceptor.
# O pino D23 do ESP32 fica sem contato (basta nao montar esse contato no soquete).
ESP32_PADS_OMITIDOS = ("16",)

# --- Modulo buck LM2596 (placa azul) ------------------------------------
# CONFERIR COM PAQUIMETRO: corpo do modulo (comprimento x largura).
LM2596_COMPRIMENTO = 43.0
LM2596_LARGURA = 21.0
# CONFERIR COM PAQUIMETRO: distancia dos furos ate a borda MENOR do modulo.
LM2596_FURO_BORDA_CURTA = 2.0
# CONFERIR COM PAQUIMETRO: separacao entre os dois furos de uma mesma ponta (na largura).
LM2596_FUROS_LARGURA = 17.0
# CONFERIR COM PAQUIMETRO: separacao entre os furos de entrada e os de saida (no comprimento).
LM2596_FUROS_COMPRIMENTO = 39.0
# Posicao (x, y) de cada furo medida a partir do canto superior esquerdo do modulo visto de
# cima (x ao longo do comprimento, y ao longo da largura) e a rede de cada um.
# IN+ e OUT+ ficam na mesma lateral (a de baixo, mais perto do ESP32); IN- e OUT- na outra.
# CONFERIR no modulo real se os "+" estao do mesmo lado; se forem opostos, troque aqui.
_y_alto = (LM2596_LARGURA - LM2596_FUROS_LARGURA) / 2.0
_y_baixo = _y_alto + LM2596_FUROS_LARGURA
_x_in = LM2596_FURO_BORDA_CURTA
_x_out = LM2596_FURO_BORDA_CURTA + LM2596_FUROS_COMPRIMENTO
LM2596_FUROS = {
    "IN+":  {"num": 1, "xy": (_x_in,  _y_baixo), "rede": "+12V_PROT"},
    "IN-":  {"num": 2, "xy": (_x_in,  _y_alto),  "rede": "GND"},
    "OUT+": {"num": 3, "xy": (_x_out, _y_baixo), "rede": "+5V"},
    "OUT-": {"num": 4, "xy": (_x_out, _y_alto),  "rede": "GND"},
}

# --- Modulo transceptor "SN65HVD230 CAN Board" (Waveshare), montado EM PE ----------
# O modulo (placa azul estreita) encaixa num soquete femea vertical 1x4 e fica perpendicular
# a PCB. Visto de cima ele e uma linha fina (a largura do modulo ao longo da fileira de pinos).
# CONFERIR COM PAQUIMETRO: largura do modulo (lado da barra de 4 pinos).
SN65_MODULO_LARGURA = 14.0
# CONFERIR COM PAQUIMETRO: comprimento do modulo (altura acima da PCB; so informativo).
SN65_MODULO_ALTURA = 37.0
# CONFERIR COM PAQUIMETRO: espessura da placa do modulo + folga, vista de cima.
SN65_MODULO_ESPESSURA = 2.0
# CONFERIR COM PAQUIMETRO: saliencia do borne CAN do modulo para o lado dos componentes.
SN65_BORNE_SALIENCIA = 10.0
SN65_BORNE_LARGURA = 10.0
# CONFERIR no modulo: ordem dos pinos da esquerda para a direita, olhando para o lado dos
# componentes (o borne vem na sua direcao) com a barra de pinos embaixo.
# Com o lado dos componentes e o borne voltados para DENTRO da placa (norte), a esquerda desse
# observador e o leste, entao na PCB a ordem oeste -> leste e CAN_TX, CAN_RX, GND, 3V3.
# Outra ordem exige rever o roteamento do CAN (o script avisa).
SN65_ORDEM_PINOS = ("CAN_TX", "CAN_RX", "GND", "3V3")

# =====================================================================
#  PROJETO DA PLACA / REGRAS DE CORROSAO CASEIRA
# =====================================================================
LARG_SINAL = 1.0              # CAN_TX, CAN_RX, CAN_H, CAN_L, TERM, +3V3
LARG_POTENCIA = 1.5           # +12V_IN, +12V_FUS, +12V_PROT, +5V
ISOLAMENTO = 0.6              # clearance minimo
FOLGA_BORDA = 1.0             # cobre-borda
ZONA_FOLGA = 0.8
ZONA_LARG_MIN = 0.8
ZONA_ALIVIO = 0.8             # folga e largura do alivio termico
PAD_SOQUETE = (1.8, 2.6)      # oval (perpendicular a fileira), furo 1,0
FURO_SOQUETE = 1.0
PAD_LM2596 = 2.6              # redondo, furo 1,0
RAIO_CANTO = 2.0              # cantos arredondados da placa (0 = quadrados)
FOLGA_MODULOS = 1.0           # folga entre contornos de modulos

# Origem do desenho no PCB (so desloca tudo para o meio da folha).
ORIGEM_PCB = (60.0, 40.0)

# Posicoes no sistema da placa (mm): origem = centro do pino 1 da fileira de cima do ESP32.
BORDA_DIREITA = 2.0           # borda direita depois do pino 1 (antena para fora)
X_CORREDOR_12V = -45.5        # trilha +12V_PROT sobe pela esquerda ate o IN+ do buck
Y_5V = -6.0                   # linha de furos de baixo do LM2596 (IN+/OUT+)
Y_BORNE = 40.0                # linha dos pads do borne
X_BORNE_PIN5 = -20.5          # x do pino 5 (CAN_L) do borne; os outros vem a cada 5,08
C1_POS = (-49.0, 0.0)          # pino + do C1, colado no IN+ do buck (o pino - fica 2,5 mm a leste, GND)
Y_C1_DESCE = 4.5                # onde a trilha do corredor vira para oeste e chega ao + do C1 por baixo
Y_D1K = 25.98                 # catodo do D1 (anodo 7,62 mm abaixo)
F1_POS = (-45.5, 42.3)        # pino 1 do F1 (+12V_IN, vem do borne); pino 2 fica 5,08 mm acima
FURO_M3 = 3.2
PTC_PASSO = 5.08              # distancia entre os terminais do PTC
PTC_LARGURA = 7.6             # disco do PTC (ao longo dos terminais)
PTC_ESPESSURA = 3.4           # disco do PTC (espessura)

# =====================================================================
#  CAMINHOS
# =====================================================================
KICAD = "C:/Users/davif/AppData/Local/Programs/KiCad/10.0"
KICAD_CLI = KICAD + "/bin/kicad-cli.exe"
LIB_FP = KICAD + "/share/kicad/footprints"
LIB_SYM = KICAD + "/share/kicad/symbols"

AQUI = os.path.dirname(os.path.abspath(__file__))
PROJ = os.path.normpath(os.path.join(AQUI, ".."))
RAIZ = os.path.normpath(os.path.join(PROJ, ".."))
NOME = "tccelta"
F_SCH = os.path.join(PROJ, NOME + ".kicad_sch")
F_PCB = os.path.join(PROJ, NOME + ".kicad_pcb")
F_PRO = os.path.join(PROJ, NOME + ".kicad_pro")
F_DRU = os.path.join(PROJ, NOME + ".kicad_dru")
F_SYM = os.path.join(PROJ, NOME + ".kicad_sym")
D_PRETTY = os.path.join(PROJ, NOME + ".pretty")
D_FAB = os.path.join(PROJ, "fabricacao")
D_TMP = os.path.join(PROJ, "scripts", "_tmp")

NS = uuid.UUID("6f1b6b0e-3f0e-4a57-9d8e-0c5d2f6a7b11")


def uid(chave):
    return str(uuid.uuid5(NS, chave))


def f(v):
    """Formata numero para s-expression."""
    s = ("%.4f" % v).rstrip("0").rstrip(".")
    return "0" if s in ("-0", "") else s


# =====================================================================
#  MODELO DO CIRCUITO (fonte unica da verdade para esquematico e PCB)
# =====================================================================
FP_BORNE2 = "TerminalBlock_Phoenix:TerminalBlock_Phoenix_MKDS-1,5-2-5.08_1x02_P5.08mm_Horizontal"
FP_BORNE3 = "TerminalBlock_Phoenix:TerminalBlock_Phoenix_MKDS-1,5-3-5.08_1x03_P5.08mm_Horizontal"
FP_D1 = "Diode_THT:D_DO-41_SOD81_P7.62mm_Horizontal"
FP_C1 = "Capacitor_THT:CP_Radial_D6.3mm_P2.50mm"
FP_FURO = "MountingHole:MountingHole_3.2mm_M3"
FP_ESP = "tccelta:ESP32_DevKitV1_30_Soquete"
FP_LM = "tccelta:LM2596_Modulo_Soquete"
FP_SN = "tccelta:SN65HVD230_Waveshare_EmPe"
FP_PINH2 = "Connector_PinHeader_2.54mm:PinHeader_1x02_P2.54mm_Vertical"
FP_R1 = "Resistor_THT:R_Axial_DIN0207_L6.3mm_D2.5mm_P2.54mm_Vertical"
FP_F1 = "tccelta:PTC_Radial_P5.08_Vertical"

# ref -> dados
COMPONENTES = {
    "J1": dict(lib_id="Connector:Screw_Terminal_01x02", valor="Borne_2_vias_5.08", fp=FP_BORNE2),
    "J2": dict(lib_id="Connector:Screw_Terminal_01x03", valor="Borne_3_vias_5.08", fp=FP_BORNE3),
    "F1": dict(lib_id="Device:Polyfuse", valor="PTC_0.5A_60V", fp=FP_F1),
    "D1": dict(lib_id="Device:D_Schottky", valor="1N5819", fp=FP_D1),
    "C1": dict(lib_id="Device:C_Polarized", valor="100uF/25V", fp=FP_C1),
    "U1": dict(lib_id="tccelta:LM2596_Modulo", valor="LM2596_5V", fp=FP_LM),
    "U2": dict(lib_id="tccelta:ESP32_DevKitV1_30", valor="ESP32-DevKit-V1", fp=FP_ESP),
    "U3": dict(lib_id="tccelta:SN65HVD230_Waveshare", valor="SN65HVD230_CAN_Board", fp=FP_SN),
    "J3": dict(lib_id="Connector_Generic:Conn_01x02", valor="CAN_p/_modulo", fp=FP_PINH2),
    "R1": dict(lib_id="Device:R", valor="120R_1/4W", fp=FP_R1),
    "JP1": dict(lib_id="Jumper:Jumper_2_Open", valor="TERM_120R", fp=FP_PINH2),
    "J4": dict(lib_id="Connector:Screw_Terminal_01x02", valor="Borne_CAN_do_modulo_SN65", fp="",
               no_bom=True, no_board=True),
    "H1": dict(lib_id="Mechanical:MountingHole", valor="M3", fp=FP_FURO),
    "H2": dict(lib_id="Mechanical:MountingHole", valor="M3", fp=FP_FURO),
    "H3": dict(lib_id="Mechanical:MountingHole", valor="M3", fp=FP_FURO),
}

# (ref, pino) -> rede.  Pinos nao listados ficam sem ligacao (no-connect).
LIGACOES = {
    ("J1", "1"): "+12V_IN", ("J1", "2"): "GND",
    ("J2", "1"): "GND", ("J2", "2"): "CAN_H", ("J2", "3"): "CAN_L",
    ("F1", "1"): "+12V_IN", ("F1", "2"): "+12V_FUS",
    ("D1", "2"): "+12V_FUS", ("D1", "1"): "+12V_PROT",      # 2 = anodo, 1 = catodo
    ("C1", "1"): "+12V_PROT", ("C1", "2"): "GND",
    ("U1", "1"): "+12V_PROT", ("U1", "2"): "GND", ("U1", "3"): "+5V", ("U1", "4"): "GND",
    ("U2", "15"): "+5V", ("U2", "14"): "GND", ("U2", "29"): "GND",
    ("U2", "30"): "+3V3", ("U2", "20"): "CAN_TX", ("U2", "17"): "CAN_RX",
    ("U3", "1"): "+3V3", ("U3", "2"): "GND", ("U3", "3"): "CAN_RX", ("U3", "4"): "CAN_TX",
    ("J3", "1"): "CAN_H", ("J3", "2"): "CAN_L", ("J4", "1"): "CAN_H", ("J4", "2"): "CAN_L",
    ("R1", "1"): "TERM", ("R1", "2"): "CAN_L", ("JP1", "1"): "TERM", ("JP1", "2"): "CAN_H",
}
REDES = ["+12V_IN", "+12V_FUS", "+12V_PROT", "+5V", "+3V3", "GND", "CAN_TX", "CAN_RX", "CAN_H", "CAN_L", "TERM"]
REDES_SINAL = ["CAN_TX", "CAN_RX", "CAN_H", "CAN_L", "TERM", "+3V3"]
REDES_POTENCIA = ["+12V_IN", "+12V_FUS", "+12V_PROT", "+5V"]

# Pinos do DOIT ESP32 DevKit V1 (vista de cima, antena para cima, numeracao de cima para baixo)
ESP_ESQ = ["EN", "VP(IO36)", "VN(IO39)", "D34", "D35", "D32", "D33", "D25", "D26", "D27", "D14",
           "D12", "D13", "GND", "VIN"]
ESP_DIR = ["D23", "D22", "TX0", "RX0", "D21", "D19", "D18", "D5", "TX2(IO17)", "RX2(IO16)", "D4",
           "D2", "D15", "GND", "3V3"]
NP = ESP32_PINOS_POR_FILEIRA
# numero do pino: esquerda 1..15, direita 16..30
ESP_PINOS = [(str(i + 1), n) for i, n in enumerate(ESP_ESQ)] + \
            [(str(NP + 1 + i), n) for i, n in enumerate(ESP_DIR)]

# =====================================================================
#  GEOMETRIA DERIVADA
# =====================================================================
P = ESP32_PASSO
ESP_X1 = 0.0                                   # pino 1 (EN)
ESP_Y_CIMA = 0.0
ESP_Y_BAIXO = ESP32_ESPACAMENTO_FILEIRAS


def esp_pad(num):
    """Posicao (x, y) do pad numero 'num' do ESP32 no sistema da placa."""
    n = int(num)
    if n <= NP:
        return (-(n - 1) * P, ESP_Y_CIMA)
    return (-(n - NP - 1) * P, ESP_Y_BAIXO)


# Corpo do ESP32 (relativo ao pino 1 da fileira de cima)
ESP_CORPO_X1 = ESP32_ANTENA_AVANCO
ESP_CORPO_X0 = ESP32_ANTENA_AVANCO - ESP32_COMPRIMENTO
ESP_CORPO_YC = ESP32_ESPACAMENTO_FILEIRAS / 2.0
ESP_CORPO_Y0 = ESP_CORPO_YC - ESP32_LARGURA / 2.0
ESP_CORPO_Y1 = ESP_CORPO_YC + ESP32_LARGURA / 2.0

# LM2596: o canto sup. esq. e definido pelo furo IN+ colocado em (X_CORREDOR_12V, Y_5V)
LM_IN_X, LM_IN_Y = X_CORREDOR_12V, Y_5V
LM_TL = (LM_IN_X - LM2596_FUROS["IN+"]["xy"][0], LM_IN_Y - LM2596_FUROS["IN+"]["xy"][1])

# SN65 em pe, na borda de baixo, lado dos componentes para fora (sul). Origem = pino 3V3 (oeste).
Y_SN = 40.5                                     # linha dos pads do soquete do transceptor
SN_P1 = (-8.89, Y_SN)                           # pino mais a oeste (CAN_TX)
J3_POS = (-14.5, 37.46)                         # J3 (macho 1x2 CAN_H/CAN_L): pino 2 na linha do borne
# Terminacao CAN selecionavel: CAN_H -- JP1 -- TERM -- R1 (120 ohms, em pe) -- CAN_L.
R1_POS = (-12.4, 31.2)                          # pino 1 do R1 (TERM); pino 2 (CAN_L) 2,54 mm abaixo
JP1_POS = (-16.0, 31.2)                         # pino 1 do JP1 (TERM); pino 2 (CAN_H) 2,54 mm abaixo
X_CAN_L_R1 = -11.9                              # corredor do CAN_L ate o R1 (a leste dos pads do J3)
PAD_TERM = 1.9                                  # pads redondos do R1/JP1, furo 1,0
J3_REF_DX, J3_REF_DY = 0.0, 4.8
R1_REF_DX, R1_REF_DY = 2.6, 1.3
JP1_REF_DX, JP1_REF_DY = -2.6, -2.4
TXT_TERM = (-25.5, 29.2)
TXT_TERM2 = (-25.5, 30.4)
TXT_TERM3 = (-25.5, 31.6)
Y_FAIXA_3V3 = 43.0                              # faixa abaixo do soquete por onde o +3V3 vai a oeste
Y_DIAG = 32.0                                   # fim das diagonais CAN_TX/CAN_RX (abaixo dos pads)
X_FURO_BL = -49.5                               # furo M3 inferior esquerdo

# Borne: 5 vias, pinos a cada 5,08
BORNE_X = [X_BORNE_PIN5 - 4 * 5.08 + k * 5.08 for k in range(5)]   # via 1..5
# via 1 = J1.1 (+12V), 2 = J1.2 (GND), 3 = J2.1 (GND), 4 = J2.2 (CAN_H), 5 = J2.3 (CAN_L)

# =====================================================================
#  UTILIDADES S-EXPRESSION
# =====================================================================


def extrai_bloco(texto, inicio):
    """Devolve o bloco balanceado que comeca em texto[inicio] == '('."""
    prof = 0
    i = inicio
    em_str = False
    while i < len(texto):
        c = texto[i]
        if em_str:
            if c == "\\":
                i += 1
            elif c == '"':
                em_str = False
        else:
            if c == '"':
                em_str = True
            elif c == "(":
                prof += 1
            elif c == ")":
                prof -= 1
                if prof == 0:
                    return texto[inicio:i + 1]
        i += 1
    raise ValueError("bloco nao balanceado")


def simbolo_padrao(arquivo, nome):
    txt = open(os.path.join(LIB_SYM, arquivo), encoding="utf-8").read().replace("\r", "")
    chave = '\n\t(symbol "%s"' % nome
    pos = txt.index(chave) + 1
    return extrai_bloco(txt, pos)


def pinos_do_simbolo(bloco):
    """Lista de dicts (num, nome, x, y, ang) do simbolo."""
    pinos = []
    for m in re.finditer(r"\(pin\s+\w+\s+\w+\s*\(at\s+([-\d.]+)\s+([-\d.]+)\s+(\d+)\)", bloco):
        sub = extrai_bloco(bloco, m.start())
        num = re.search(r'\(number\s+"([^"]*)"', sub).group(1)
        nome = re.search(r'\(name\s+"([^"]*)"', sub).group(1)
        pinos.append(dict(num=num, nome=nome, x=float(m.group(1)), y=float(m.group(2)),
                          ang=int(m.group(3))))
    return pinos


def rota(dx, dy, graus):
    """Rotaciona (dx, dy) em coordenadas de tela (y para baixo) por 'graus' anti-horario."""
    a = math.radians(graus)
    c, s = round(math.cos(a), 6), round(math.sin(a), 6)
    return (dx * c + dy * s, -dx * s + dy * c)


# =====================================================================
#  SIMBOLOS PROPRIOS (tccelta.kicad_sym)
# =====================================================================


def prop(nome, valor, x, y, oculto=False, tam=1.27):
    return ('(property "%s" "%s" (at %s %s 0) %s(effects (font (size %s %s))%s))'
            % (nome, valor, f(x), f(y), "(show_name no) (do_not_autoplace no) ",
               f(tam), f(tam), " (hide yes)" if oculto else ""))


def simbolo_modulo(nome, descricao, esq, dir_, ref="U", largura=15.24, passo=2.54, folga=2.54,
                   fp=""):
    """Gera um simbolo retangular. esq/dir_ = listas [(numero, nome)] de cima para baixo."""
    n = max(len(esq), len(dir_))
    altura = (n - 1) * passo + 2 * folga
    meio = (n - 1) * passo / 2.0
    y_topo = altura / 2.0
    x_corpo = largura / 2.0
    comp = 3.81
    s = []
    s.append('\t(symbol "%s"' % nome)
    s.append("\t\t(pin_names (offset 1.016))")
    s.append("\t\t(exclude_from_sim no) (in_bom yes) (on_board yes) (in_pos_files yes)")
    s.append("\t\t(duplicate_pin_numbers_are_jumpers no)")
    s.append("\t\t" + prop("Reference", ref, 0, y_topo + 1.27))
    s.append("\t\t" + prop("Value", nome, 0, -y_topo - 1.27))
    s.append("\t\t" + prop("Footprint", fp, 0, 0, True))
    s.append("\t\t" + prop("Datasheet", "", 0, 0, True))
    s.append("\t\t" + prop("Description", descricao, 0, 0, True))
    s.append('\t\t(symbol "%s_1_1"' % nome)
    s.append("\t\t\t(rectangle (start %s %s) (end %s %s) (stroke (width 0.254) (type default)) "
             "(fill (type background)))" % (f(-x_corpo), f(y_topo), f(x_corpo), f(-y_topo)))
    for i, (num, pn) in enumerate(esq):
        y = meio - i * passo
        s.append('\t\t\t(pin passive line (at %s %s 0) (length %s) (name "%s" (effects (font '
                 '(size 1.27 1.27)))) (number "%s" (effects (font (size 1.27 1.27)))))'
                 % (f(-x_corpo - comp), f(y), f(comp), pn, num))
    for i, item in enumerate(dir_):
        if item is None:
            continue
        num, pn = item
        y = meio - i * passo
        s.append('\t\t\t(pin passive line (at %s %s 180) (length %s) (name "%s" (effects (font '
                 '(size 1.27 1.27)))) (number "%s" (effects (font (size 1.27 1.27)))))'
                 % (f(x_corpo + comp), f(y), f(comp), pn, num))
    s.append("\t\t)")
    s.append("\t)")
    return "\n".join(s)


def simbolos_proprios():
    esp = simbolo_modulo(
        "ESP32_DevKitV1_30", "DOIT ESP32 DevKit V1 de 30 pinos (2x15), vista de cima",
        [(str(i + 1), n) for i, n in enumerate(ESP_ESQ)],
        [(str(NP + 1 + i), n) if str(NP + 1 + i) not in ESP32_PADS_OMITIDOS else None
         for i, n in enumerate(ESP_DIR)], largura=20.32, fp=FP_ESP)
    lm = simbolo_modulo(
        "LM2596_Modulo", "Modulo buck LM2596 (placa azul), encaixado em 4 soquetes",
        [("1", "IN+"), ("2", "IN-")], [("3", "OUT+"), ("4", "OUT-")], largura=12.7, fp=FP_LM)
    sn = simbolo_modulo(
        "SN65HVD230_Waveshare", "SN65HVD230 CAN Board (Waveshare), em pe no soquete 1x4",
        [("1", "3V3"), ("2", "GND"), ("3", "CRX"), ("4", "CTX")],
        [], largura=12.7, fp=FP_SN)
    return {"ESP32_DevKitV1_30": esp, "LM2596_Modulo": lm, "SN65HVD230_Waveshare": sn}


def escreve_biblioteca_simbolos(simbolos):
    corpo = "\n".join(simbolos.values())
    txt = ('(kicad_symbol_lib\n\t(version 20251024)\n\t(generator "gerar_placa")\n'
           '\t(generator_version "1.0")\n%s\n)\n' % corpo)
    with open(F_SYM, "w", encoding="utf-8", newline="\n") as fh:
        fh.write(txt)


# =====================================================================
#  FOOTPRINTS PROPRIOS (tccelta.pretty)
# =====================================================================


class FP:
    def __init__(self, nome, descr):
        self.nome = nome
        self.l = []
        self.descr = descr
        self.n = 0

    def _u(self):
        self.n += 1
        return uid("fp/%s/%d" % (self.nome, self.n))

    def prop(self, nome, valor, x, y, camada, oculto=False, tam=1.0, rot=0):
        self.l.append('\t(property "%s" "%s" (at %s %s %s) (layer "%s") %s(effects (font (size %s %s) '
                      '(thickness %s))))' % (nome, valor, f(x), f(y), f(rot), camada,
                                             "(hide yes) " if oculto else "", f(tam), f(tam),
                                             f(tam * 0.15)))

    def linha(self, x0, y0, x1, y1, camada, larg=0.12):
        self.l.append('\t(fp_line (start %s %s) (end %s %s) (stroke (width %s) (type solid)) '
                      '(layer "%s") (uuid "%s"))' % (f(x0), f(y0), f(x1), f(y1), f(larg), camada,
                                                     self._u()))

    def ret(self, x0, y0, x1, y1, camada, larg=0.12):
        self.l.append('\t(fp_rect (start %s %s) (end %s %s) (stroke (width %s) (type solid)) '
                      '(fill no) (layer "%s") (uuid "%s"))' % (f(x0), f(y0), f(x1), f(y1), f(larg),
                                                                camada, self._u()))

    def poli(self, pts, camada, larg=0.12):
        for a, b in zip(pts[:-1], pts[1:]):
            self.linha(a[0], a[1], b[0], b[1], camada, larg)

    def texto(self, txt, x, y, camada="F.SilkS", tam=1.0, rot=0):
        self.l.append('\t(fp_text user "%s" (at %s %s %s) (layer "%s") (effects (font (size %s %s) '
                      '(thickness %s))) (uuid "%s"))' % (txt, f(x), f(y), f(rot), camada, f(tam),
                                                          f(tam), f(tam * 0.15), self._u()))

    def pad(self, num, x, y, w, h, furo, forma="oval"):
        self.l.append('\t(pad "%s" thru_hole %s (at %s %s) (size %s %s) (drill %s) '
                      '(layers "*.Cu" "*.Mask") (remove_unused_layers no) (uuid "%s"))'
                      % (num, forma, f(x), f(y), f(w), f(h), f(furo), self._u()))

    def keepout(self, pts):
        xy = " ".join("(xy %s %s)" % (f(x), f(y)) for x, y in pts)
        self.l.append(
            '\t(zone (net 0) (net_name "") (layers "F&B.Cu") (uuid "%s") (name "keepout_antena") '
            '(hatch edge 0.5) (connect_pads (clearance 0)) (min_thickness 0.25) '
            '(filled_areas_thickness no) (keepout (tracks not_allowed) (vias not_allowed) '
            '(pads allowed) (copperpour not_allowed) (footprints allowed)) '
            '(placement (enabled no) (sheetname "")) (fill (thermal_gap 0.5) '
            '(thermal_bridge_width 0.5)) (polygon (pts %s)))' % (self._u(), xy))

    def salva(self):
        cab = ['(footprint "%s"' % self.nome, "\t(version 20241229)", '\t(generator "gerar_placa")',
               '\t(generator_version "1.0")', '\t(layer "F.Cu")',
               '\t(descr "%s")' % self.descr, '\t(tags "soquete modulo tccelta")']
        corpo = cab + self.l + ["\t(attr through_hole)", "\t(embedded_fonts no)", ")"]
        with open(os.path.join(D_PRETTY, self.nome + ".kicad_mod"), "w", encoding="utf-8",
                  newline="\n") as fh:
            fh.write("\n".join(corpo) + "\n")


def gera_fp_esp32():
    fp = FP("ESP32_DevKitV1_30_Soquete",
            "DOIT ESP32 DevKit V1 de 30 pinos deitado, antena para a direita (+X). Origem = pino 1 (EN).")
    x0, x1, y0, y1 = ESP_CORPO_X0, ESP_CORPO_X1, ESP_CORPO_Y0, ESP_CORPO_Y1
    fp.prop("Reference", "REF**", -30.0, ESP_CORPO_YC, "F.SilkS", tam=1.5)
    fp.prop("Value", "ESP32_DevKitV1_30_Soquete", ESP_CORPO_YC * 0 - 22, ESP_CORPO_YC, "F.Fab",
            tam=1.0)
    fp.prop("Datasheet", "", 0, 0, "F.Fab", True)
    fp.prop("Description", "", 0, 0, "F.Fab", True)
    # contorno do corpo
    fp.ret(x0, y0, x1, y1, "F.SilkS", 0.15)
    fp.ret(x0, y0, x1, y1, "F.Fab", 0.1)
    c = 0.25
    fp.ret(x0 - c, y0 - c, x1 + c, y1 + c, "F.CrtYd", 0.05)
    # antena (area alem do pino 1): contorno + meandro
    ax0 = ESP_X1 + ESP32_ANTENA_INICIO_APOS_PINO1
    fp.linha(ax0, y0, ax0, y1, "F.SilkS", 0.15)
    pts = []
    n = 7
    ya, yb = y0 + 2.0, y1 - 2.0
    for k in range(n):
        yy = ya + (yb - ya) * k / (n - 1)
        xx = x1 - 1.2 if k % 2 == 0 else ax0 + 1.4
        pts.append((xx, yy))
    # meandro simples (zigue-zague)
    zig = []
    for k in range(n):
        yy = ya + (yb - ya) * k / (n - 1)
        zig.append((ax0 + 1.4, yy))
        zig.append((x1 - 1.2, yy))
    fp.poli(zig, "F.SilkS", 0.2)
    fp.poli(zig, "F.Fab", 0.15)
    fp.texto("ANTENA", 3.0, ESP_CORPO_YC, "F.SilkS", 0.9, 90)
    # USB (lado esquerdo)
    fp.ret(x0 - 1.0, ESP_CORPO_YC - 4.5, x0 + 4.5, ESP_CORPO_YC + 4.5, "F.SilkS", 0.15)
    fp.texto("USB", x0 + 2.4, ESP_CORPO_YC, "F.SilkS", 0.9, 90)
    # marca do pino 1 e legenda
    fp.texto("1", ESP_X1, -2.2, "F.SilkS", 1.0)
    fp.texto("ESP32 DevKit V1 (visto de cima)", ESP_X1 - 18.0, ESP_CORPO_YC - 2.4, "F.SilkS", 0.9)
    # pads e rotulos
    for (num, nome) in ESP_PINOS:
        x, y = esp_pad(num)
        if num not in ESP32_PADS_OMITIDOS:
            fp.pad(num, x, y, PAD_SOQUETE[0], PAD_SOQUETE[1], FURO_SOQUETE)
        rot_txt = nome.split("(")[0]
        dy = 3.9 if int(num) <= NP else -3.9
        fp.texto(rot_txt, x, y + dy, "F.SilkS", 0.8, 90)
    # keepout sem cobre sob a antena
    kx0 = ESP_X1 + ESP32_KEEPOUT_INICIO if False else ESP_X1 + ESP32_KEEPOUT_INICIO
    fp.keepout([(kx0, y0), (x1, y0), (x1, y1), (kx0, y1)])
    fp.salva()


def gera_fp_lm2596():
    fp = FP("LM2596_Modulo_Soquete",
            "Modulo buck LM2596 43x21 mm. Origem = canto superior esquerdo do corpo; "
            "4 soquetes de 1 pino nos furos (IN a esquerda, OUT a direita).")
    L, W = LM2596_COMPRIMENTO, LM2596_LARGURA
    fp.prop("Reference", "REF**", L / 2.0, W / 2.0 - 3.0, "F.SilkS", tam=1.5)
    fp.prop("Value", "LM2596_Modulo_Soquete", L / 2.0, W / 2.0 + 3.0, "F.Fab")
    fp.prop("Datasheet", "", 0, 0, "F.Fab", True)
    fp.prop("Description", "", 0, 0, "F.Fab", True)
    fp.ret(0, 0, L, W, "F.SilkS", 0.15)
    fp.ret(0, 0, L, W, "F.Fab", 0.1)
    fp.ret(-0.25, -0.25, L + 0.25, W + 0.25, "F.CrtYd", 0.05)
    for nome, d in LM2596_FUROS.items():
        x, y = d["xy"]
        fp.pad(d["num"], x, y, PAD_LM2596, PAD_LM2596, FURO_SOQUETE, "circle")
        dx = 4.2 if x < L / 2 else -4.2
        fp.texto(nome, x + dx, y, "F.SilkS", 1.0)
    fp.texto("LM2596 (5,0 V)  IN+/IN- a esquerda", L / 2.0, W / 2.0 + 0.0, "F.SilkS", 0.9)
    fp.salva()


_SN_NUM = {"3V3": 1, "GND": 2, "CAN_RX": 3, "CAN_TX": 4}
_SN_ROTULO = {"3V3": "3V3", "GND": "GND", "CAN_RX": "CRX", "CAN_TX": "CTX"}


def gera_fp_sn65():
    fp = FP("SN65HVD230_Waveshare_EmPe",
            "SN65HVD230 CAN Board (Waveshare) EM PE: soquete 1x4. Lado dos componentes e do borne "
            "CAN para -Y (para DENTRO da placa). Origem = pino 1 (oeste). Visto de cima o modulo e "
            "uma linha fina.")
    c = 1.5 * P
    L, E = SN65_MODULO_LARGURA, SN65_MODULO_ESPESSURA
    fp.prop("Reference", "REF**", c, -13.8, "F.SilkS", tam=1.0)
    fp.prop("Value", "SN65HVD230_Waveshare_EmPe", c, 3.5, "F.Fab")
    fp.prop("Datasheet", "", 0, 0, "F.Fab", True)
    fp.prop("Description", "", 0, 0, "F.Fab", True)
    # projecao do modulo em pe (linha fina)
    fp.ret(c - L / 2, -E / 2, c + L / 2, E / 2, "F.SilkS", 0.15)
    fp.ret(c - L / 2, -E / 2, c + L / 2, E / 2, "F.Fab", 0.1)
    fp.ret(c - L / 2 - 0.25, -E / 2 - 0.25, c + L / 2 + 0.25, E / 2 + 0.25, "F.CrtYd", 0.05)
    # saliencia do borne CAN do modulo (so no alto, ~27-37 mm acima da PCB), para o norte
    bl, bs = SN65_BORNE_LARGURA, SN65_BORNE_SALIENCIA
    fp.ret(c - bl / 2, -E / 2 - bs, c + bl / 2, -E / 2, "F.Fab", 0.1)
    fp.texto("BORNE CAN (no alto)", c, -E / 2 - bs / 2, "F.Fab", 0.8)
    # seta e texto: lado dos componentes / borne para DENTRO da placa (norte)
    fp.linha(c, -1.9, c, -8.6, "F.SilkS", 0.15)
    fp.linha(c, -8.6, c - 0.7, -7.5, "F.SilkS", 0.15)
    fp.linha(c, -8.6, c + 0.7, -7.5, "F.SilkS", 0.15)
    fp.texto("COMPONENTES E BORNE", c, -11.4, "F.SilkS", 0.7)
    fp.texto("PARA DENTRO", c, -12.5, "F.SilkS", 0.7)
    for i, nome in enumerate(SN65_ORDEM_PINOS):
        x = i * P
        fp.pad(_SN_NUM[nome], x, 0.0, PAD_SOQUETE[0], PAD_SOQUETE[1], FURO_SOQUETE)
        fp.texto(_SN_ROTULO[nome], x, -3.4, "F.SilkS", 0.8, 90)
    fp.texto("1", -1.9, 0.0, "F.SilkS", 0.9)
    fp.salva()


def gera_fp_ptc():
    """Fusivel rearmavel PTC radial (RXEF050 / MF-R050 e similares), terminais a 5,08 mm."""
    fp = FP("PTC_Radial_P5.08_Vertical",
            "Fusivel rearmavel PTC radial em pe (disco ~7,5 x 3 mm), terminais a 5,08 mm. "
            "Origem = pino 1; pino 2 em -Y.")
    yc = -PTC_PASSO / 2.0
    w, h = PTC_ESPESSURA, PTC_LARGURA
    fp.prop("Reference", "REF**", -2.6, yc, "F.SilkS", tam=1.0, rot=90)
    fp.prop("Value", "PTC_Radial_P5.08_Vertical", 2.6, yc, "F.Fab", rot=90)
    fp.prop("Datasheet", "", 0, 0, "F.Fab", True)
    fp.prop("Description", "", 0, 0, "F.Fab", True)
    fp.ret(-w / 2, yc - h / 2, w / 2, yc + h / 2, "F.SilkS", 0.15)
    fp.ret(-w / 2, yc - h / 2, w / 2, yc + h / 2, "F.Fab", 0.1)
    fp.ret(-w / 2 - 0.25, yc - h / 2 - 0.25, w / 2 + 0.25, yc + h / 2 + 0.25, "F.CrtYd", 0.05)
    fp.pad("1", 0, 0, PAD_LM2596, PAD_LM2596, FURO_SOQUETE, "circle")
    fp.pad("2", 0, -PTC_PASSO, PAD_LM2596, PAD_LM2596, FURO_SOQUETE, "circle")
    # modelo 3D so para o render: disco de 7,5 mm (mesmo formato de um PTC radial), girado para os pinos em -Y
    fp.l.append('	(model "${KICAD10_3DMODEL_DIR}/Capacitor_THT.3dshapes/C_Disc_D7.5mm_W2.5mm_P5.00mm.step" '
                '(offset (xyz 0 0 0)) (scale (xyz 1 1 1)) (rotate (xyz 0 0 -90)))')
    fp.salva()


def gera_footprints():
    if os.path.isdir(D_PRETTY):
        shutil.rmtree(D_PRETTY)
    os.makedirs(D_PRETTY)
    gera_fp_esp32()
    gera_fp_lm2596()
    gera_fp_sn65()
    gera_fp_ptc()
    with open(os.path.join(PROJ, "sym-lib-table"), "w", encoding="utf-8", newline="\n") as fh:
        fh.write('(sym_lib_table\n\t(version 7)\n\t(lib (name "tccelta")(type "KiCad")'
                 '(uri "${KIPRJMOD}/tccelta.kicad_sym")(options "")(descr "Simbolos do dongle OBD-II"))\n)\n')
    with open(os.path.join(PROJ, "fp-lib-table"), "w", encoding="utf-8", newline="\n") as fh:
        fh.write('(fp_lib_table\n\t(version 7)\n\t(lib (name "tccelta")(type "KiCad")'
                 '(uri "${KIPRJMOD}/tccelta.pretty")(options "")(descr "Footprints do dongle OBD-II"))\n)\n')


# =====================================================================
#  ESQUEMATICO
# =====================================================================


class Sch:
    def __init__(self):
        self.raiz = uid("sch/raiz")
        self.itens = []
        self.simbolos_lib = {}      # lib_id -> texto (ja com nome qualificado)
        self.n = 0

    def u(self):
        self.n += 1
        return uid("sch/%d" % self.n)

    def fio(self, x0, y0, x1, y1):
        self.itens.append('\t(wire (pts (xy %s %s) (xy %s %s)) (stroke (width 0) (type default)) '
                          '(uuid "%s"))' % (f(x0), f(y0), f(x1), f(y1), self.u()))

    def nc(self, x, y):
        self.itens.append('\t(no_connect (at %s %s) (uuid "%s"))' % (f(x), f(y), self.u()))

    def rotulo(self, nome, x, y, ang):
        just = "right" if ang == 180 else ("left" if ang in (0, 90) else "right")
        if ang == 270:
            just = "right"
        self.itens.append(
            '\t(global_label "%s" (shape bidirectional) (at %s %s %d) (fields_autoplaced yes) '
            '(effects (font (size 1.27 1.27)) (justify %s)) (uuid "%s")\n'
            '\t\t(property "Intersheetrefs" "${INTERSHEET_REFS}" (at %s %s 0) (effects (font '
            '(size 1.27 1.27)) (justify %s) (hide yes))))'
            % (nome, f(x), f(y), ang, just, self.u(), f(x), f(y), just))

    def texto(self, txt, x, y, tam=1.8, negrito=False, just="left"):
        self.itens.append('\t(text "%s" (exclude_from_sim no) (at %s %s 0) (effects (font (size %s %s)%s) '
                          '(justify %s bottom)) (uuid "%s"))'
                          % (txt, f(x), f(y), f(tam), f(tam), " (bold yes)" if negrito else "",
                             just, self.u()))

    def moldura(self, x0, y0, x1, y1):
        self.itens.append('\t(rectangle (start %s %s) (end %s %s) (stroke (width 0.3) (type dash)) '
                          '(fill (type none)) (uuid "%s"))' % (f(x0), f(y0), f(x1), f(y1), self.u()))

    def bloco(self, titulo, x0, y0, x1, y1):
        self.moldura(x0, y0, x1, y1)
        self.texto(titulo, x0 + 3, y0 + 8, 3.0, True)

    def componente(self, ref, x, y, rot=0, ref_at=None, val_at=None, pino_ang_stub=True):
        info = COMPONENTES[ref]
        lib_id = info["lib_id"]
        bloco = self.simbolos_lib[lib_id]
        pinos = pinos_do_simbolo(bloco)
        ref_at = ref_at or (x, y - 6.35)
        val_at = val_at or (x, y + 6.35)
        ang_txt = 90 if rot in (90, 270) else 0     # texto na horizontal mesmo com o simbolo girado
        linhas = ['\t(symbol (lib_id "%s") (at %s %s %d) (unit 1) (body_style 1) (exclude_from_sim no) '
                  '(in_bom %s) (on_board %s) (in_pos_files yes) (dnp no) (fields_autoplaced yes)'
                  % (lib_id, f(x), f(y), rot,
                     "no" if (ref.startswith("H") or info.get("no_bom")) else "yes",
                     "no" if info.get("no_board") else "yes"),
                  '\t\t(uuid "%s")' % uid("comp/" + ref),
                  '\t\t(property "Reference" "%s" (at %s %s %d) (effects (font (size 1.27 1.27))))'
                  % (ref, f(ref_at[0]), f(ref_at[1]), ang_txt),
                  '\t\t(property "Value" "%s" (at %s %s %d) (effects (font (size 1.27 1.27))))'
                  % (info["valor"], f(val_at[0]), f(val_at[1]), ang_txt),
                  '\t\t(property "Footprint" "%s" (at %s %s 0) (effects (font (size 1.27 1.27)) (hide yes)))'
                  % (info["fp"], f(x), f(y)),
                  '\t\t(property "Datasheet" "" (at %s %s 0) (effects (font (size 1.27 1.27)) (hide yes)))'
                  % (f(x), f(y)),
                  '\t\t(property "Description" "" (at %s %s 0) (effects (font (size 1.27 1.27)) (hide yes)))'
                  % (f(x), f(y))]
        for p in pinos:
            linhas.append('\t\t(pin "%s" (uuid "%s"))' % (p["num"], uid("pin/%s/%s" % (ref, p["num"]))))
        linhas.append('\t\t(instances (project "%s" (path "/%s" (reference "%s") (unit 1))))'
                      % (NOME, self.raiz, ref))
        linhas.append("\t)")
        self.itens.append("\n".join(linhas))
        # fios curtos + rotulos
        for p in pinos:
            dx, dy = rota(p["x"], -p["y"], rot)
            px, py = x + dx, y + dy
            # direcao para fora: oposta a do corpo (angulo do pino aponta para o corpo)
            ox, oy = rota(-math.cos(math.radians(p["ang"])), math.sin(math.radians(p["ang"])), rot)
            ox, oy = round(ox), round(oy)
            rede = LIGACOES.get((ref, p["num"]))
            if rede is None:
                if info["lib_id"] != "Mechanical:MountingHole":
                    self.nc(px, py)
                continue
            ex, ey = px + ox * 2.54, py + oy * 2.54
            self.fio(px, py, ex, ey)
            ang = {(1, 0): 0, (-1, 0): 180, (0, -1): 90, (0, 1): 270}[(ox, oy)]
            self.rotulo(rede, ex, ey, ang)

    def salva(self):
        lib = "\n".join(self.simbolos_lib.values())
        txt = ['(kicad_sch', '\t(version 20260306)', '\t(generator "eeschema")',
               '\t(generator_version "10.0")', '\t(uuid "%s")' % self.raiz, '\t(paper "A4")',
               '\t(title_block (title "Dongle OBD-II - TCC") (date "2026-11-07") (rev "1") '
               '(company "TCC ELTA") (comment 1 "Alimentacao 12 V -> 5 V (LM2596) -> ESP32 + SN65HVD230 (CAN)"))',
               '\t(lib_symbols', lib, '\t)']
        txt += self.itens
        txt += ['\t(sheet_instances (path "/" (page "1")))', "\t(embedded_fonts no)", ")"]
        with open(F_SCH, "w", encoding="utf-8", newline="\n") as fh:
            fh.write("\n".join(txt) + "\n")


def qualifica(bloco, lib_id):
    nome = lib_id.split(":")[1]
    return bloco.replace('(symbol "%s"' % nome, '(symbol "%s"' % lib_id, 1)


def gera_esquematico(proprios):
    s = Sch()
    # simbolos da biblioteca padrao
    for lib_id, arq in [("Connector:Screw_Terminal_01x02", "Connector.kicad_sym"),
                        ("Connector:Screw_Terminal_01x03", "Connector.kicad_sym"),
                        ("Connector_Generic:Conn_01x02", "Connector_Generic.kicad_sym"),
                        ("Device:D_Schottky", "Device.kicad_sym"),
                        ("Device:Polyfuse", "Device.kicad_sym"),
                        ("Device:C_Polarized", "Device.kicad_sym"),
                        ("Device:R", "Device.kicad_sym"),
                        ("Jumper:Jumper_2_Open", "Jumper.kicad_sym"),
                        ("Mechanical:MountingHole", "Mechanical.kicad_sym")]:
        s.simbolos_lib[lib_id] = qualifica(simbolo_padrao(arq, lib_id.split(":")[1]), lib_id)
    for nome, bloco in proprios.items():
        lid = "tccelta:" + nome
        s.simbolos_lib[lid] = qualifica(bloco, lid)

    # ---- bloco 1: entrada e protecao
    s.bloco("1. Entrada e protecao (cabo OBD-II)", 8, 12, 170, 78)
    s.componente("J1", 40.64, 38.1, 0, ref_at=(40.64, 31.75), val_at=(40.64, 45.72))
    s.componente("J2", 40.64, 58.42, 0, ref_at=(40.64, 52.07), val_at=(40.64, 66.04))
    s.componente("F1", 71.12, 55.88, 90, ref_at=(71.12, 51.82), val_at=(71.12, 60.2))
    s.componente("D1", 101.6, 38.1, 180, ref_at=(101.6, 33.02), val_at=(101.6, 43.18))
    s.componente("C1", 144.78, 50.8, 0, ref_at=(152.4, 48.26), val_at=(152.4, 53.34))
    s.texto("Cabo OBD-II: pino 16 = +12V, pinos 4 e 5 = GND, pino 6 = CAN_H, pino 14 = CAN_L",
            11, 72, 1.3)
    s.texto("F1 (PTC rearmavel 0,5 A / 60 V): abre num curto na placa. D1 (1N5819): inversao de polaridade.",
            11, 76, 1.3)
    s.texto("J1 + J2 formam um borne de 5 vias.", 11, 68, 1.3)

    # ---- bloco 2: regulacao
    s.bloco("2. Regulacao 5 V (modulo LM2596)", 176, 12, 286, 78)
    s.componente("U1", 233.68, 43.18, 0, ref_at=(233.68, 33.02), val_at=(233.68, 53.34))
    s.texto("Ajustar a saida para 5,0 V ANTES de encaixar o ESP32.", 179, 72, 1.3)
    s.texto("Modulo encaixado em 4 soquetes femea de 1 pino.", 179, 76, 1.3)

    # ---- bloco 3: ESP32
    s.bloco("3. Microcontrolador (DOIT ESP32 DevKit V1, 30 pinos)", 8, 84, 150, 206)
    s.componente("U2", 80.01, 146.05, 0, ref_at=(80.01, 124.46), val_at=(80.01, 168.0))
    s.texto("Alimentado pelo pino VIN (5 V). CAN: TX = GPIO21 (D21), RX = GPIO22 (D22).", 11, 198, 1.3)
    s.texto("Os demais pinos ficam sem ligacao (marcados com X).", 11, 202, 1.3)

    # ---- bloco 4: CAN
    s.bloco("4. Transceptor CAN (SN65HVD230 Waveshare, em pe)", 156, 84, 286, 144)
    s.componente("U3", 190.5, 111.76, 0, ref_at=(190.5, 102.0), val_at=(190.5, 121.0))
    s.componente("J3", 246.38, 101.6, 0, ref_at=(245.0, 95.0), val_at=(245.0, 108.0))
    s.componente("J4", 246.38, 124.46, 0, ref_at=(245.0, 118.0), val_at=(245.0, 131.0))
    # terminacao CAN selecionavel: CAN_H -- JP1 -- TERM -- R1 -- CAN_L
    s.componente("JP1", 266.7, 101.6, 0, ref_at=(266.7, 96.52), val_at=(266.7, 106.68))
    s.componente("R1", 266.7, 114.3, 90, ref_at=(266.7, 109.22), val_at=(266.7, 119.38))
    s.texto("J3 (macho 1x2) liga ao borne CAN do modulo (J4) por 2 fios trancados, ate 10 cm.", 159, 134, 1.3)
    s.texto("J4 e so o borne do modulo (nao vai na PCB). Remover SEMPRE o resistor SMD de 120 ohms do modulo.", 159, 142, 1.3)
    s.texto("JP1 colocado = terminacao de 120 ohms (bancada). Sem jumper = sem terminacao (carro).", 159, 138, 1.3)

    # ---- bloco 5: mecanica
    s.bloco("5. Mecanica", 156, 146, 286, 172)
    s.componente("H1", 185.0, 160.0, 0, ref_at=(185.0, 154.0), val_at=(185.0, 165.0))
    s.componente("H2", 215.0, 160.0, 0, ref_at=(215.0, 154.0), val_at=(215.0, 165.0))
    s.componente("H3", 245.0, 160.0, 0, ref_at=(245.0, 154.0), val_at=(245.0, 165.0))
    s.salva()


# =====================================================================
#  .kicad_pro e regras (.kicad_dru)
# =====================================================================


def escreve_projeto():
    classe_base = {
        "bus_width": 12, "diff_pair_gap": 0.25, "diff_pair_via_gap": 0.25, "diff_pair_width": 0.2,
        "line_style": 0, "microvia_diameter": 0.3, "microvia_drill": 0.1,
        "pcb_color": "rgba(0, 0, 0, 0.000)", "schematic_color": "rgba(0, 0, 0, 0.000)",
        "via_diameter": 0.6, "via_drill": 0.3, "wire_width": 6,
    }

    def classe(nome, larg, prio):
        d = dict(classe_base)
        d.update(name=nome, clearance=ISOLAMENTO, track_width=larg, priority=prio)
        return d

    padroes = [{"netclass": "Sinal", "pattern": r} for r in REDES_SINAL] + \
              [{"netclass": "Potencia", "pattern": r} for r in REDES_POTENCIA]
    pro = {
        "board": {
            "design_settings": {
                "defaults": {"copper_line_width": 0.2, "copper_text_size_h": 1.5,
                             "copper_text_size_v": 1.5, "copper_text_thickness": 0.3,
                             "other_line_width": 0.1, "silk_line_width": 0.12,
                             "silk_text_size_h": 1.0, "silk_text_size_v": 1.0,
                             "silk_text_thickness": 0.15},
                "rules": {
                    "min_clearance": ISOLAMENTO, "min_connection": 0.0,
                    "min_copper_edge_clearance": FOLGA_BORDA, "min_hole_clearance": 0.25,
                    "min_hole_to_hole": 0.25, "min_resolved_spokes": 2,
                    "min_silk_clearance": 0.0, "min_text_height": 0.8,
                    "min_text_thickness": 0.12, "min_through_hole_diameter": 0.8,
                    "min_track_width": ZONA_LARG_MIN, "min_via_annular_width": 0.1,
                    "min_via_diameter": 0.5, "solder_mask_to_copper_clearance": 0.0,
                    "use_height_for_length_calcs": True,
                },
                "rule_severities": {"silk_over_copper": "ignore", "silk_overlap": "ignore",
                                    "silk_edge_clearance": "ignore", "text_height": "ignore",
                                    "text_thickness": "ignore", "lib_footprint_issues": "ignore",
                                    "lib_footprint_mismatch": "ignore"},
                "track_widths": [0.0, LARG_SINAL, LARG_POTENCIA],
                "via_dimensions": [],
                "zones_allow_external_fillets": False,
            },
        },
        "meta": {"filename": NOME + ".kicad_pro", "version": 3},
        "net_settings": {
            "classes": [classe("Default", LARG_SINAL, 2147483647), classe("Sinal", LARG_SINAL, 1),
                        classe("Potencia", LARG_POTENCIA, 0)],
            "meta": {"version": 4},
            "netclass_assignments": None,
            "netclass_patterns": padroes,
        },
        "schematic": {"drawing": {"default_line_thickness": 6.0}},
        "sheets": [[uid("sch/raiz"), "Raiz"]],
        "text_variables": {},
    }
    with open(F_PRO, "w", encoding="utf-8", newline="\n") as fh:
        json.dump(pro, fh, indent=2)
    with open(F_DRU, "w", encoding="utf-8", newline="\n") as fh:
        fh.write('(version 1)\n\n'
                 '# Placa de face simples: nenhuma via permitida.\n'
                 '(rule "sem_vias"\n\t(constraint disallow via))\n\n'
                 '# Cobre so em B.Cu.\n'
                 '(rule "sem_trilhas_em_F_Cu"\n\t(layer "F.Cu")\n\t(constraint disallow track))\n')


# =====================================================================
#  PCB
# =====================================================================


def constroi_pcb():
    import pcbnew
    from pcbnew import FromMM, VECTOR2I

    ox, oy = ORIGEM_PCB

    def V(x, y):
        return VECTOR2I(FromMM(x + ox), FromMM(y + oy))

    b = pcbnew.BOARD()
    b.SetCopperLayerCount(2)
    nets = {}
    for nome in REDES:
        ni = pcbnew.NETINFO_ITEM(b, nome)
        b.Add(ni)
        nets[nome] = ni

    # pinos sem ligacao: o KiCad cria uma rede 'unconnected-(REF-PINO-PadN)' para cada um
    for ref, info in COMPONENTES.items():
        if info["lib_id"].startswith("tccelta:ESP32"):
            for num, nome in ESP_PINOS:
                if (ref, num) not in LIGACOES and num not in ESP32_PADS_OMITIDOS:
                    n = "unconnected-(%s-%s-Pad%s)" % (ref, nome, num)
                    ni = pcbnew.NETINFO_ITEM(b, n)
                    b.Add(ni)
                    nets[(ref, num)] = ni
    fps = {}

    def coloca(ref, lib_dir, nome_fp, lib_nick, x, y, rot=0):
        fp = pcbnew.FootprintLoad(lib_dir, nome_fp)
        fp.SetFPID(pcbnew.LIB_ID(lib_nick, nome_fp))
        fp.SetReference(ref)
        fp.SetValue(COMPONENTES[ref]["valor"])
        fp.SetPosition(V(x, y))
        if rot:
            fp.SetOrientationDegrees(rot)
        fp.SetPath(pcbnew.KIID_PATH("/" + uid("comp/" + ref)))
        for pad in fp.Pads():
            rede = LIGACOES.get((ref, pad.GetNumber()))
            if rede:
                pad.SetNet(nets[rede])
            elif (ref, pad.GetNumber()) in nets:
                pad.SetNet(nets[(ref, pad.GetNumber())])
        b.Add(fp)
        fps[ref] = fp
        return fp

    def std(ref, x, y, rot=0):
        nick, nome = COMPONENTES[ref]["fp"].split(":")
        return coloca(ref, os.path.join(LIB_FP, nick + ".pretty"), nome, nick, x, y, rot)

    # --- posicionamento ----------------------------------------------------
    coloca("U2", D_PRETTY, "ESP32_DevKitV1_30_Soquete", "tccelta", 0, 0)
    coloca("U1", D_PRETTY, "LM2596_Modulo_Soquete", "tccelta", LM_TL[0], LM_TL[1])
    coloca("U3", D_PRETTY, "SN65HVD230_Waveshare_EmPe", "tccelta", SN_P1[0], SN_P1[1])
    std("J3", J3_POS[0], J3_POS[1])
    std("R1", R1_POS[0], R1_POS[1], 270)      # em pe: pino 2 abaixo do pino 1
    std("JP1", JP1_POS[0], JP1_POS[1])
    std("J1", BORNE_X[0], Y_BORNE)
    std("J2", BORNE_X[2], Y_BORNE)
    std("D1", X_CORREDOR_12V, Y_D1K, 270)
    std("C1", C1_POS[0], C1_POS[1])
    coloca("F1", D_PRETTY, "PTC_Radial_P5.08_Vertical", "tccelta", F1_POS[0], F1_POS[1])

    # bornes colados: courtyard = corpo exato, para os dois blocos nao se sobreporem
    for ref, vias in (("J1", 2), ("J2", 3)):
        fp = fps[ref]
        for g in list(fp.GraphicalItems()):
            if g.GetLayer() == pcbnew.F_CrtYd:
                fp.Remove(g)
        r = pcbnew.PCB_SHAPE(fp)
        r.SetShape(pcbnew.SHAPE_T_RECT)
        r.SetFilled(False)
        r.SetLayer(pcbnew.F_CrtYd)
        r.SetWidth(FromMM(0.05))
        pp = fp.GetPosition()
        r.SetStart(pcbnew.VECTOR2I(pp.x - FromMM(2.52), pp.y - FromMM(5.2)))
        r.SetEnd(pcbnew.VECTOR2I(pp.x + FromMM(5.08 * (vias - 1) + 2.52), pp.y + FromMM(4.6)))
        fp.Add(r)

    # referencias legiveis (nao se sobrepoem entre si)
    for ref, (dx, dy) in {"J1": (2.54, 3.6), "J2": (5.08, 3.6), "C1": (1.25, 4.7),
                          "D1": (-2.6, 3.9), "J3": (J3_REF_DX, J3_REF_DY), "F1": (-2.6, -2.54),
                          "R1": (R1_REF_DX, R1_REF_DY), "JP1": (JP1_REF_DX, JP1_REF_DY)}.items():
        fps[ref].Reference().SetPosition(pcbnew.VECTOR2I(fps[ref].GetPosition().x + FromMM(dx),
                                                          fps[ref].GetPosition().y + FromMM(dy)))
        fps[ref].Reference().SetTextAngleDegrees(0)

    # alarga pads de D1/C1/J3 conforme as regras
    for pad in fps["J3"].Pads():
        pad.SetShape(pcbnew.PAD_SHAPE_OVAL)
        pad.SetSize(pcbnew.VECTOR2I(FromMM(2.6), FromMM(1.8)))
    for ref in ("R1", "JP1"):
        for pad in fps[ref].Pads():
            pad.SetShape(pcbnew.PAD_SHAPE_CIRCLE)
            pad.SetSize(pcbnew.VECTOR2I(FromMM(PAD_TERM), FromMM(PAD_TERM)))
            pad.SetDrillSize(pcbnew.VECTOR2I(FromMM(FURO_SOQUETE), FromMM(FURO_SOQUETE)))
    for ref in ("D1", "C1"):
        for pad in fps[ref].Pads():
            if ref == "C1":
                pad.SetShape(pcbnew.PAD_SHAPE_OVAL)
                pad.SetSize(pcbnew.VECTOR2I(FromMM(1.8), FromMM(2.6)))
            else:
                pad.SetShape(pcbnew.PAD_SHAPE_CIRCLE)
                pad.SetSize(pcbnew.VECTOR2I(FromMM(2.6), FromMM(2.6)))

    # Conexao solida ao plano (sem alivio termico) em 2 pads de GND: o GND do transceptor so tem
    # espaco para um raio (o outro lado e a faixa do +3V3 e a borda) e o C1 e o capacitor de
    # entrada (retorno de corrente alto, melhor solido).
    for ref, num in (("U3", "2"), ("C1", "2")):
        fps[ref].FindPadByNumber(num).SetLocalZoneConnection(pcbnew.ZONE_CONNECTION_FULL)

    def pos(ref, num):
        pad = fps[ref].FindPadByNumber(str(num))
        p = pad.GetPosition()
        return (round(pcbnew.ToMM(p.x) - ox, 4), round(pcbnew.ToMM(p.y) - oy, 4))

    # posicoes dos pads da terminacao: conferencia por codigo
    for ref, num, esperado in (("R1", 1, R1_POS), ("R1", 2, (R1_POS[0], R1_POS[1] + 2.54)),
                               ("JP1", 1, JP1_POS), ("JP1", 2, (JP1_POS[0], JP1_POS[1] + 2.54))):
        got = pos(ref, num)
        assert abs(got[0] - esperado[0]) < 0.01 and abs(got[1] - esperado[1]) < 0.01, \
            "pad %s.%s fora do lugar: %s != %s" % (ref, num, got, esperado)

    # limites da placa
    corpos_x = [LM_TL[0], ESP_CORPO_X0]
    x_min = min(min(corpos_x) - 0.8, X_FURO_BL - FURO_M3 / 2.0 - FOLGA_BORDA)
    x_max = ESP_X1 + BORDA_DIREITA
    y_min = LM_TL[1] - 1.0
    # fundo: calculado pelo bbox dos bornes
    bb = fps["J2"].GetBoundingBox(False, False)
    y_borne_fundo = pcbnew.ToMM(bb.GetBottom()) - oy
    y_max = y_borne_fundo + 1.0
    brd = dict(x0=x_min, y0=y_min, x1=x_max, y1=y_max)

    # furos de fixacao (lado direito, cantos)
    furos = {"H1": (x_max - 3.0, y_min + 4.0), "H2": (X_FURO_BL, y_max - 2.6),
             "H3": (x_max - 3.0, -9.0)}
    for ref, (mx, my) in furos.items():
        fh = std(ref, mx, my)
        fh.Reference().SetVisible(False)
        for g in list(fh.GraphicalItems()):
            if g.GetLayer() == pcbnew.F_CrtYd:
                fh.Remove(g)
        r = pcbnew.PCB_SHAPE(fh)
        r.SetShape(pcbnew.SHAPE_T_RECT)
        r.SetFilled(False)
        r.SetLayer(pcbnew.F_CrtYd)
        r.SetWidth(FromMM(0.05))
        r.SetStart(V(mx - 1.85, my - 1.85))
        r.SetEnd(V(mx + 1.85, my + 1.85))
        fh.Add(r)

    # --- contorno ----------------------------------------------------------
    def seg(x0, y0, x1, y1):
        s = pcbnew.PCB_SHAPE(b)
        s.SetShape(pcbnew.SHAPE_T_SEGMENT)
        s.SetStart(V(x0, y0))
        s.SetEnd(V(x1, y1))
        s.SetLayer(pcbnew.Edge_Cuts)
        s.SetWidth(FromMM(0.1))
        b.Add(s)

    def arco(cx, cy, r, a0):
        # quarto de circulo de a0 a a0+90 graus (a0 em graus, sentido horario na tela)
        s = pcbnew.PCB_SHAPE(b)
        s.SetShape(pcbnew.SHAPE_T_ARC)
        a1, am, a2 = [math.radians(a) for a in (a0, a0 + 45, a0 + 90)]
        s.SetArcGeometry(V(cx + r * math.cos(a1), cy + r * math.sin(a1)),
                         V(cx + r * math.cos(am), cy + r * math.sin(am)),
                         V(cx + r * math.cos(a2), cy + r * math.sin(a2)))
        s.SetLayer(pcbnew.Edge_Cuts)
        s.SetWidth(FromMM(0.1))
        b.Add(s)

    r = RAIO_CANTO
    X0, Y0, X1, Y1 = brd["x0"], brd["y0"], brd["x1"], brd["y1"]
    if r > 0:
        seg(X0 + r, Y0, X1 - r, Y0)
        seg(X1, Y0 + r, X1, Y1 - r)
        seg(X1 - r, Y1, X0 + r, Y1)
        seg(X0, Y1 - r, X0, Y0 + r)
        arco(X1 - r, Y0 + r, r, 270)
        arco(X1 - r, Y1 - r, r, 0)
        arco(X0 + r, Y1 - r, r, 90)
        arco(X0 + r, Y0 + r, r, 180)
    else:
        seg(X0, Y0, X1, Y0)
        seg(X1, Y0, X1, Y1)
        seg(X1, Y1, X0, Y1)
        seg(X0, Y1, X0, Y0)

    # --- trilhas ------------------------------------------------------------
    def chanfro(pts, c=1.0):
        """Troca cantos de 90 graus por cortes de 45 graus."""
        saida = [pts[0]]
        for i in range(1, len(pts) - 1):
            a, m, n = pts[i - 1], pts[i], pts[i + 1]
            v1 = (m[0] - a[0], m[1] - a[1])
            v2 = (n[0] - m[0], n[1] - m[1])
            l1, l2 = math.hypot(*v1), math.hypot(*v2)
            cruz = v1[0] * v2[1] - v1[1] * v2[0]
            dot = v1[0] * v2[0] + v1[1] * v2[1]
            if abs(dot) < 1e-6 and abs(cruz) > 1e-6:
                cc = min(c, l1 / 2.0 - 0.01, l2 / 2.0 - 0.01)
                saida.append((m[0] - v1[0] / l1 * cc, m[1] - v1[1] / l1 * cc))
                saida.append((m[0] + v2[0] / l2 * cc, m[1] + v2[1] / l2 * cc))
            else:
                saida.append(m)
        saida.append(pts[-1])
        return saida

    def trilha(rede, larg, pts, cham=1.0, so_45=True):
        pts = chanfro(pts, cham) if cham else pts
        for a, c2 in zip(pts[:-1], pts[1:]):
            dx, dy = c2[0] - a[0], c2[1] - a[1]
            ang = math.degrees(math.atan2(dy, dx)) % 45.0
            assert not so_45 or min(ang, 45.0 - ang) < 0.05, "segmento fora de 45 graus: %s -> %s" % (a, c2)
            if math.hypot(dx, dy) < 1e-4:
                continue
            t = pcbnew.PCB_TRACK(b)
            t.SetStart(V(*a))
            t.SetEnd(V(*c2))
            t.SetWidth(FromMM(larg))
            t.SetLayer(pcbnew.B_Cu)
            t.SetNet(nets[rede])
            b.Add(t)

    # +12V_IN: borne via 1 -> pino 1 do F1 (PTC)
    j1_1 = pos("J1", 1)
    f1_1, f1_2 = pos("F1", 1), pos("F1", 2)
    d1_a = pos("D1", 2)
    d1_k = pos("D1", 1)
    trilha("+12V_IN", LARG_POTENCIA, [j1_1, (f1_1[0] + (f1_1[1] - j1_1[1]), j1_1[1]), f1_1], 0)
    # +12V_FUS: pino 2 do F1 -> anodo do D1 (mesma coluna)
    trilha("+12V_FUS", LARG_POTENCIA, [f1_2, d1_a], 0)
    # +12V_PROT: catodo do D1 -> sobe o corredor -> diagonal para oeste -> pino + do C1 (por baixo)
    # -> do + do C1 direto ao IN+ do buck (D1 -> C1 -> IN+: o C1 nunca ve tensao reversa)
    c1_p = pos("C1", 1)
    u1_inp = pos("U1", 1)
    dd = d1_k[0] - c1_p[0]
    y_d = c1_p[1] + Y_C1_DESCE
    trilha("+12V_PROT", LARG_POTENCIA, [d1_k, (d1_k[0], y_d + dd), (c1_p[0], y_d), c1_p], 0)
    dn = u1_inp[0] - c1_p[0]
    trilha("+12V_PROT", LARG_POTENCIA, [c1_p, (c1_p[0], u1_inp[1] + dn), u1_inp], 0)
    # +5V: OUT+ -> pino VIN do ESP32 (por cima da fileira de cima)
    u1_outp = pos("U1", 3)
    p5v = pos("U2", 15)
    trilha("+5V", LARG_POTENCIA, [u1_outp, (p5v[0], u1_outp[1]), p5v])
    # +3V3: 3V3 do SN65 (pino mais a leste) -> desce para a faixa sul -> oeste -> sobe entre
    # os pads GND do borne -> pino 3V3 do ESP32 (fileira de baixo, ponta do USB)
    p3v = pos("U2", 30)
    s3v = pos("U3", 1)
    gnd_m, crx, ctx = pos("U3", 2), pos("U3", 3), pos("U3", 4)
    assert ctx[0] < crx[0] < gnd_m[0] < s3v[0], \
        "SN65_ORDEM_PINOS diferente da prevista: o roteamento do CAN precisa ser revisto"
    x_v = (pos("J1", 2)[0] + pos("J2", 1)[0]) / 2.0     # passa entre os pads GND e GND do borne
    y_j = p3v[1] + 5.8 + (x_v - p3v[0])
    trilha("+3V3", LARG_SINAL, [s3v, (s3v[0], Y_FAIXA_3V3), (x_v, Y_FAIXA_3V3), (x_v, y_j),
                                (p3v[0], y_j - (x_v - p3v[0])), p3v])
    # CAN_TX / CAN_RX: sobem direto (diagonais divergentes) para D21 / D22, sem cruzar
    d22 = pos("U2", 17)
    d21 = pos("U2", 20)
    trilha("CAN_TX", LARG_SINAL, [ctx, (ctx[0], Y_DIAG + abs(d21[0] - ctx[0])), (d21[0], Y_DIAG), d21], 0)
    trilha("CAN_RX", LARG_SINAL, [crx, (crx[0], Y_DIAG + abs(d22[0] - crx[0])), (d22[0], Y_DIAG), d22], 0)
    # CAN_H / CAN_L: borne (vias 4 e 5) -> J3, lado a lado
    j2_2, j2_3 = pos("J2", 2), pos("J2", 3)
    j3_1, j3_2 = pos("J3", 1), pos("J3", 2)
    trilha("CAN_H", LARG_SINAL, [j2_2, (j2_2[0], j3_1[1]), j3_1])
    trilha("CAN_L", LARG_SINAL, [j2_3, j3_2])
    # terminacao: CAN_H -> JP1.2 ; JP1.1 -> R1.1 (TERM) ; J3.2 (CAN_L) -> corredor leste -> R1.2
    jp1_1, jp1_2, r1_1, r1_2 = pos("JP1", 1), pos("JP1", 2), pos("R1", 1), pos("R1", 2)
    trilha("TERM", LARG_SINAL, [jp1_1, r1_1], 0)
    trilha("CAN_H", LARG_SINAL, [jp1_2, j3_1], 0, so_45=False)   # diagonal curta, fora dos 45 graus (validada no DRC)
    trilha("CAN_L", LARG_SINAL, [j3_2, (X_CAN_L_R1, j3_2[1]), (X_CAN_L_R1, r1_2[1] + 1.2), r1_2], 0, so_45=False)

    # --- keepout da antena (regra de area; o do footprint ja vem junto) ------
    # (o footprint ja traz a area proibida; nada a fazer aqui)

    # --- textos ---------------------------------------------------------------
    def texto_cobre(txt, x, y, tam=1.5, esp=0.3, rot=0):
        t = pcbnew.PCB_TEXT(b)
        t.SetText(txt)
        t.SetLayer(pcbnew.B_Cu)
        t.SetMirrored(True)
        t.SetTextSize(VECTOR2I(FromMM(tam), FromMM(tam)))
        t.SetTextThickness(FromMM(esp))
        t.SetPosition(V(x, y))
        t.SetHorizJustify(pcbnew.GR_TEXT_H_ALIGN_CENTER)
        t.SetTextAngleDegrees(rot)
        b.Add(t)

    def texto_serigrafia(txt, x, y, tam=1.0, rot=0):
        t = pcbnew.PCB_TEXT(b)
        t.SetText(txt)
        t.SetLayer(pcbnew.F_SilkS)
        t.SetTextSize(VECTOR2I(FromMM(tam), FromMM(tam)))
        t.SetTextThickness(FromMM(tam * 0.15))
        t.SetPosition(V(x, y))
        t.SetTextAngleDegrees(rot)
        b.Add(t)

    texto_cobre("TCCELTA DONGLE OBD-II v1", (X0 + X1) / 2.0 + 2.0, Y0 + 10.5, 1.8, 0.4)
    texto_serigrafia("TERM 120R", TXT_TERM[0], TXT_TERM[1], 0.8)
    texto_serigrafia("jumper: bancada", TXT_TERM2[0], TXT_TERM2[1], 0.8)
    texto_serigrafia("sem: carro", TXT_TERM3[0], TXT_TERM3[1], 0.8)
    texto_serigrafia("H", J3_POS[0] + 2.0, J3_POS[1], 1.0)
    texto_serigrafia("L", J3_POS[0] + 2.0, J3_POS[1] + 2.54, 1.0)
    rotulos = ["+12V", "GND", "GND", "CANH", "CANL"]
    for k, nome in enumerate(rotulos):
        texto_cobre(nome, BORNE_X[k], Y_BORNE - (5.6 if k == 0 else 6.5), 1.2, 0.3, 90)
        texto_serigrafia(nome, BORNE_X[k], Y_BORNE - 3.0, 1.0)

    # --- plano de GND -----------------------------------------------------------
    z = pcbnew.ZONE(b)
    z.SetLayer(pcbnew.B_Cu)
    z.SetNet(nets["GND"])
    z.SetLocalClearance(FromMM(ZONA_FOLGA))
    z.SetMinThickness(FromMM(ZONA_LARG_MIN))
    z.SetPadConnection(pcbnew.ZONE_CONNECTION_THERMAL)
    z.SetThermalReliefGap(FromMM(ZONA_ALIVIO))
    z.SetThermalReliefSpokeWidth(FromMM(ZONA_ALIVIO))
    z.SetIslandRemovalMode(pcbnew.ISLAND_REMOVAL_MODE_ALWAYS)
    z.SetZoneName("GND")
    ol = z.Outline()
    ol.NewOutline()
    m = FOLGA_BORDA
    rz = max(RAIO_CANTO - m, 0.0)
    cantos = [(X1 - RAIO_CANTO, Y0 + RAIO_CANTO, 270), (X1 - RAIO_CANTO, Y1 - RAIO_CANTO, 0),
              (X0 + RAIO_CANTO, Y1 - RAIO_CANTO, 90), (X0 + RAIO_CANTO, Y0 + RAIO_CANTO, 180)]
    if rz > 0:
        for (cx_, cy_, a0) in cantos:
            for k in range(0, 9):
                an = math.radians(a0 + 90.0 * k / 8.0)
                ol.Append(FromMM(cx_ + rz * math.cos(an) + ox), FromMM(cy_ + rz * math.sin(an) + oy))
    else:
        for (px_, py_) in [(X0 + m, Y0 + m), (X1 - m, Y0 + m), (X1 - m, Y1 - m), (X0 + m, Y1 - m)]:
            ol.Append(FromMM(px_ + ox), FromMM(py_ + oy))
    b.Add(z)

    pcbnew.SaveBoard(F_PCB, b)
    return brd


def preenche_zonas():
    import pcbnew
    b = pcbnew.LoadBoard(F_PCB)
    filler = pcbnew.ZONE_FILLER(b)
    filler.Fill(b.Zones())
    pcbnew.SaveBoard(F_PCB, b)
    return b


# =====================================================================
#  EXECUCAO DE FERRAMENTAS
# =====================================================================


def cli(*args):
    r = subprocess.run([KICAD_CLI] + list(args), capture_output=True, text=True)
    return r.returncode, r.stdout + r.stderr


def conta_json(caminho):
    """Conta violacoes por severidade em um relatorio JSON do kicad-cli."""
    d = json.load(open(caminho, encoding="utf-8"))
    cont = {}

    def varre(o):
        if isinstance(o, dict):
            if "severity" in o and "type" in o:
                cont[o["severity"]] = cont.get(o["severity"], 0) + 1
            for v in o.values():
                varre(v)
        elif isinstance(o, list):
            for v in o:
                varre(v)
    varre(d)
    return cont, d


def confere_netlist():
    """Exporta a netlist do esquematico e compara com o modelo (LIGACOES)."""
    import xml.etree.ElementTree as ET
    xml = os.path.join(D_TMP, "netlist.xml")
    cli("sch", "export", "netlist", "--format", "kicadxml", "-o", xml, F_SCH)
    raiz = ET.parse(xml).getroot()
    obtido = {}
    for net in raiz.iter("net"):
        nome = net.get("name")
        if nome.startswith("unconnected-"):
            continue
        for no in net.iter("node"):
            obtido[(no.get("ref"), no.get("pin"))] = nome
    esperado = dict(LIGACOES)
    return obtido == esperado, obtido, esperado


def verifica_escala_pdf(caminho, largura_mm, altura_mm):
    """Le o PDF e confere a escala 1:1 (caixa envolvente do desenho ~ tamanho da placa)."""
    import zlib
    d = open(caminho, "rb").read()
    xs, ys = [], []
    esc = 0.0072
    for m in re.finditer(rb"stream\r?\n(.*?)endstream", d, re.S):
        try:
            c = zlib.decompress(m.group(1)).decode("latin-1")
        except Exception:
            continue
        mm = re.match(r"\s*([\d.]+) 0 0 ([\d.]+) 0 0 cm", c)
        if mm:
            esc = float(mm.group(1))
        for t in re.finditer(r"(-?[\d.]+) (-?[\d.]+) (?:m|l)\n", c):
            xs.append(float(t.group(1)))
            ys.append(float(t.group(2)))
    k = esc * 25.4 / 72.0          # unidades do PDF (decimils) -> mm
    w, h = (max(xs) - min(xs)) * k, (max(ys) - min(ys)) * k
    mb = re.search(rb"/MediaBox\s*\[\s*0 0 ([\d.]+) ([\d.]+)", d)
    pagina = (float(mb.group(1)) * 25.4 / 72.0, float(mb.group(2)) * 25.4 / 72.0)
    ok = (w >= largura_mm - 0.5) and abs(h - altura_mm) < 0.5 and w < largura_mm + 8.0
    return ok, (round(w, 2), round(h, 2)), (round(pagina[0], 1), round(pagina[1], 1))


def furacao_txt(caminho):
    import pcbnew
    b = pcbnew.LoadBoard(F_PCB)
    cont = {}
    for fp in b.GetFootprints():
        for pad in fp.Pads():
            d = pcbnew.ToMM(pad.GetDrillSize().x)
            if d <= 0:
                continue
            npth = pad.GetAttribute() == pcbnew.PAD_ATTRIB_NPTH
            cont.setdefault((round(d, 2), npth), []).append(fp.GetReference())
    linhas = ["FURACAO - dongle OBD-II (tccelta)", "",
              "Diametro  Tipo       Qtd  Onde", "-" * 60]
    total = 0
    for (d, npth), refs in sorted(cont.items()):
        onde = {}
        for r in refs:
            onde[r] = onde.get(r, 0) + 1
        linhas.append("%5.2f mm  %-9s %3d  %s" % (d, "sem cobre" if npth else "com cobre", len(refs),
                                                 ", ".join("%s(%d)" % kv for kv in sorted(onde.items()))))
        total += len(refs)
    linhas += ["-" * 60, "Total de furos: %d" % total, "",
               "Sugestao: broca 1,0 mm para soquetes, LM2596 e D1/C1 (furo de 0,8-1,1 mm);",
               "1,3 mm para os bornes; 3,2 mm para os furos M3 (sem cobre)."]
    open(caminho, "w", encoding="utf-8", newline="\n").write("\n".join(linhas) + "\n")
    return linhas


def valida_e_exporta():
    relatorio = {}
    ok, obtido, esperado = confere_netlist()
    relatorio["netlist"] = ok
    print("Netlist do esquematico confere com o modelo:", ok)
    erc = os.path.join(D_TMP, "erc.json")
    cli("sch", "erc", "--severity-all", "--format", "json", "-o", erc, F_SCH)
    cont, _ = conta_json(erc)
    print("ERC:", cont or "0 violacoes")
    relatorio["erc"] = cont
    drc = os.path.join(D_TMP, "drc.json")
    cli("pcb", "drc", "--schematic-parity", "--severity-all", "--format", "json", "-o", drc, F_PCB)
    cont, d = conta_json(drc)
    print("DRC:", cont or "0 violacoes", "| desconectados:", len(d.get("unconnected_items", [])),
          "| paridade:", len(d.get("schematic_parity", [])))
    relatorio["drc"] = cont
    # --- fabricacao
    os.makedirs(D_FAB, exist_ok=True)
    base = ["--mode-single", "--black-and-white", "--drill-shape-opt", "1"]
    cli("pcb", "export", "pdf", "-l", "B.Cu,Edge.Cuts", "--exclude-refdes", "--exclude-value",
        *base, "-o", os.path.join(D_FAB, "cobre_B_Cu_toner.pdf"), F_PCB)
    cli("pcb", "export", "pdf", "-l", "B.Cu,F.Fab,Edge.Cuts", *base,
        "-o", os.path.join(D_FAB, "teste_papel_1x1.pdf"), F_PCB)
    cli("pcb", "export", "pdf", "-l", "F.SilkS,F.Fab,Edge.Cuts", *base,
        "-o", os.path.join(D_FAB, "serigrafia_F_Silk.pdf"), F_PCB)
    cli("pcb", "export", "pdf", "-l", "F.SilkS,F.Fab,Edge.Cuts", "--mirror", *base,
        "-o", os.path.join(D_FAB, "serigrafia_F_Silk_espelhada.pdf"), F_PCB)
    furacao_txt(os.path.join(D_FAB, "furacao.txt"))
    cli("pcb", "export", "drill", "--format", "excellon", "--excellon-units", "mm",
        "-o", D_FAB + "/", F_PCB)
    for lado in ("top", "bottom"):
        cli("pcb", "render", "--side", lado, "-w", "1800", "-h", "1500", "--quality", "basic",
            "-o", os.path.join(D_FAB, "render_%s.png" % lado), F_PCB)
    cli("sch", "export", "pdf", "-o", os.path.join(D_FAB, "esquematico.pdf"), F_SCH)
    cli("sch", "export", "svg", "-o", D_FAB, F_SCH)
    svg = os.path.join(D_FAB, NOME + ".svg")
    if os.path.exists(svg):
        os.replace(svg, os.path.join(D_FAB, "esquematico.svg"))
    # --- escala 1:1
    import pcbnew
    b = pcbnew.LoadBoard(F_PCB)
    bb = b.GetBoardEdgesBoundingBox()
    L, A = pcbnew.ToMM(bb.GetWidth()), pcbnew.ToMM(bb.GetHeight())
    for nome in ("cobre_B_Cu_toner.pdf", "teste_papel_1x1.pdf", "serigrafia_F_Silk.pdf"):
        ok, dim, pag = verifica_escala_pdf(os.path.join(D_FAB, nome), L, A)
        print("Escala 1:1 %-26s %s  desenho=%s mm (placa %.2f x %.2f) pagina=%s mm"
              % (nome, "OK" if ok else "FALHOU", dim, L, A, pag))
    return relatorio


def main():
    os.makedirs(D_FAB, exist_ok=True)
    os.makedirs(D_TMP, exist_ok=True)
    proprios = simbolos_proprios()
    escreve_biblioteca_simbolos(proprios)
    gera_footprints()
    escreve_projeto()
    gera_esquematico(proprios)
    brd = constroi_pcb()
    preenche_zonas()
    print("Placa: %.1f x %.1f mm" % (brd["x1"] - brd["x0"], brd["y1"] - brd["y0"]))
    valida_e_exporta()
    shutil.rmtree(D_TMP, ignore_errors=True)


if __name__ == "__main__":
    sys.path.insert(0, AQUI)
    main()
