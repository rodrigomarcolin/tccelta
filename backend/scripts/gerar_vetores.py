#!/usr/bin/env python3
"""Implementacao independente (so stdlib) do protocolo, para gerar vetores de teste.

Escrita a partir da spec 03, sem olhar o codigo Go. Saida: internal/domain/protocol/testdata/vetores.json
"""
import hashlib, hmac, json, os, struct

DEV = bytes.fromhex("0011223344556677")
K_AUTH = bytes(range(0x10, 0x30))
K_REC = bytes(range(0x50, 0x70))
NS = bytes(range(0xA0, 0xB0))


def entrada(tipo, payload):
    return b"MALCONG" + bytes([tipo]) + DEV + payload


def tag(key, tipo, payload, n):
    return hmac.new(key, entrada(tipo, payload), hashlib.sha256).digest()[:n]


def tlv(t, v):
    return bytes([t]) + struct.pack("<H", len(v)) + v


def caso_hs(nome, tipo, campos, payload):
    t = tag(K_AUTH, tipo, payload, 16)
    full = hmac.new(K_AUTH, entrada(tipo, payload), hashlib.sha256).hexdigest()
    return {"nome": nome, "tipo": tipo, "campos": campos, "entrada_hmac": entrada(tipo, payload).hex(),
            "hmac_completo": full, "tlv": tlv(tipo, payload + t).hex()}


def desafio(cs, ack, conf, flags, diag=0):
    p = struct.pack("<I", cs) + NS + struct.pack("<II", ack, conf) + bytes([flags])
    if flags & 1:
        p += struct.pack("<H", diag)
    return p


def bloco(nome, seq, boot, t0, regs, campos):
    body = b""
    for r in regs:
        body += r
    dados = struct.pack("<IHIH", seq, boot, t0, 12 + len(body)) + body
    t = hmac.new(K_REC, entrada(3, dados), hashlib.sha256).digest()
    return {"nome": nome, "tipo": 3, "campos": campos, "entrada_hmac": entrada(3, dados).hex(),
            "dados": dados.hex(), "tag": t.hex(), "tlv": tlv(3, dados + t).hex()}


def pid(p, dt, a, b):
    return bytes([p]) + struct.pack("<H", dt) + bytes([a, b])


def gps(dt, lat, lon, alt):
    return bytes([0xFF]) + struct.pack("<Hiii", dt, lat, lon, alt)


def marc(tipo, dt, f, t):
    return bytes([tipo]) + struct.pack("<HII", dt, f, t)


casos = []
casos.append(caso_hs("desafio_sem_diag", 1, {"c_s": 7, "n_s": NS.hex(), "ack_seq": 120, "c_conf": 6, "flags": 0}, desafio(7, 120, 6, 0)))
casos.append(caso_hs("desafio_com_diag", 1, {"c_s": 8, "n_s": NS.hex(), "ack_seq": 120, "c_conf": 7, "flags": 1, "diag_min": 30}, desafio(8, 120, 7, 1, 30)))
resp = struct.pack("<I", 7) + NS + struct.pack("<IIHI", 100, 130, 3, 600000)
casos.append(caso_hs("resposta", 2, {"c_s": 7, "n_s": NS.hex(), "tail_seq": 100, "head_seq": 130, "boot_id": 3, "t_ms": 600000}, resp))
casos.append(caso_hs("ack_flag_ancora", 4, {"ack_seq": 130, "c_conf": 7, "flags": 1}, struct.pack("<IIB", 130, 7, 1)))
casos.append(caso_hs("ack_ok", 4, {"ack_seq": 130, "c_conf": 7, "flags": 0}, struct.pack("<IIB", 130, 7, 0)))
casos.append(bloco("bloco_dados_2pids_1gps", 121, 3, 600000,
                   [pid(12, 0, 23, 112), gps(150, -235505200, -466333100, 760000), pid(13, 40, 62, 0)],
                   {"seq": 121, "boot_id": 3, "t0_ms": 600000,
                    "registros": [{"pid": 12, "dt": 0, "a": 23, "b": 112},
                                  {"gps": True, "dt": 150, "lat": -235505200, "lon": -466333100, "alt": 760000},
                                  {"pid": 13, "dt": 40, "a": 62, "b": 0}]}))
casos.append(bloco("bloco_dt_limites", 122, 3, 0, [pid(5, 0, 90, 0), pid(5, 65535, 91, 0)],
                   {"seq": 122, "boot_id": 3, "t0_ms": 0,
                    "registros": [{"pid": 5, "dt": 0, "a": 90, "b": 0}, {"pid": 5, "dt": 65535, "a": 91, "b": 0}]}))
casos.append(bloco("bloco_boot", 192, 4, 0, [marc(0xFE, 0, 130, 191)],
                   {"seq": 192, "boot_id": 4, "t0_ms": 0, "marcador": {"tipo": 254, "dt": 0, "from_seq": 130, "to_seq": 191}}))
casos.append(bloco("bloco_loss", 200, 0, 0, [marc(0xFD, 0, 195, 199)],
                   {"seq": 200, "boot_id": 0, "t0_ms": 0, "marcador": {"tipo": 253, "dt": 0, "from_seq": 195, "to_seq": 199}}))

# bloco no limite de 1024 bytes: 12 + 201*5 = 1017 + 0 ... (202 PIDs = 1022)
regs = [pid(4, i, i & 0xFF, 0) for i in range(202)]
casos.append(bloco("bloco_1022", 300, 5, 1000, regs, {"seq": 300, "boot_id": 5, "t0_ms": 1000, "n_pids": 202}))

saida = {"versao": 1, "dev_id": DEV.hex(), "k_auth": K_AUTH.hex(), "k_rec": K_REC.hex(), "casos": casos}
dest = os.path.join(os.path.dirname(__file__), "..", "internal", "domain", "protocol", "testdata", "vetores.json")
with open(dest, "w", newline="\n") as f:
    json.dump(saida, f, indent=1)
    f.write("\n")
print("ok", len(casos), "casos")
