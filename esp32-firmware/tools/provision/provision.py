#!/usr/bin/env python3
"""Provisiona um dongle: gera DevId e chaves, a imagem da partição `secnvs` e o
registro para o KMS (spec fase-1/01 §11).

    provision.py --profile {dev,devsec,prod} [--security {hmac,asym,none}]
                 [--dev-id HEX16] [--out DIR] [--kms-wrapping FILE]
                 [--nvs-key {auto,new,board}] [--port COMx] [--flash] [--yes]

Saída em `<out>/<DevId>/`: `secnvs.csv`, `secnvs.bin`, `kms_record.json`,
`bench_keys.json` (dev/devsec) e `keys/nvs_keys.bin` (devsec/prod).
"""
import argparse
import base64
import csv
import datetime
import json
import pathlib
import secrets
import subprocess
import sys

HERE = pathlib.Path(__file__).resolve().parent
PARTITIONS = HERE.parent.parent / "partitions_collector.csv"
SECURITY_CODE = {"none": 0, "hmac": 1, "asym": 2}


def partition(name):
    """(offset, size) da partição `name` em partitions_collector.csv."""
    for line in PARTITIONS.read_text(encoding="utf-8").splitlines():
        cells = [c.strip() for c in line.split(",")]
        if cells and cells[0] == name:
            return int(cells[3], 16), int(cells[4], 16)
    raise SystemExit(f"partição {name} ausente em {PARTITIONS}")


def new_dev_id():
    while True:
        dev_id = secrets.token_bytes(8)
        if dev_id not in (b"\x00" * 8, b"\xff" * 8):
            return dev_id


def new_k_sig():
    """Escalar P-256 de 32 B e chave pública não comprimida (04 || X || Y)."""
    from cryptography.hazmat.primitives import serialization
    from cryptography.hazmat.primitives.asymmetric import ec

    key = ec.generate_private_key(ec.SECP256R1())
    scalar = key.private_numbers().private_value.to_bytes(32, "big")
    public = key.public_key().public_bytes(
        serialization.Encoding.X962, serialization.PublicFormat.UncompressedPoint
    )
    return scalar, public


WRAPPING_SCHEMA = "tccelta.kms.wrapping.v1"


def load_wrapping(path):
    """kms_wrapping.json do `tccelta-kms provision export-wrapping`: {auth, rec}: {kid, pem}."""
    wrapping = json.loads(path.read_text(encoding="utf-8"))
    if wrapping.get("schema") != WRAPPING_SCHEMA:
        raise SystemExit(f"{path}: schema {wrapping.get('schema')!r}, esperado {WRAPPING_SCHEMA}")
    for mount in ("auth", "rec"):
        entry = wrapping.get(mount)
        if not isinstance(entry, dict) or not entry.get("kid") or not entry.get("pem"):
            raise SystemExit(f"{path}: falta {mount}.kid ou {mount}.pem")
    return wrapping


def byok_wrap(key_material, wrapping):
    """Formato BYOK do Vault Transit: chave AES-256 efêmera cifrada com RSA-OAEP-SHA256
    pela wrapping key, seguida do material embrulhado com AES-KWP (RFC 5649). O kid é o
    que o KMS exportou, e o import-record o confere com o da wrapping key atual."""
    from cryptography.hazmat.primitives import hashes, serialization
    from cryptography.hazmat.primitives.asymmetric import padding, rsa
    from cryptography.hazmat.primitives.keywrap import aes_key_wrap_with_padding

    public = serialization.load_pem_public_key(wrapping["pem"].encode())
    if not isinstance(public, rsa.RSAPublicKey):
        raise SystemExit("a wrapping key do KMS não é RSA")
    ephemeral = secrets.token_bytes(32)
    wrapped_ephemeral = public.encrypt(
        ephemeral,
        padding.OAEP(mgf=padding.MGF1(hashes.SHA256()), algorithm=hashes.SHA256(), label=None),
    )
    wrapped_key = aes_key_wrap_with_padding(ephemeral, key_material)
    return base64.b64encode(wrapped_ephemeral + wrapped_key).decode(), wrapping["kid"]


def kms_field(key_material, wrapping, mount):
    if wrapping is None:
        return {"alg": "none", "kid": "", "ciphertext_b64": base64.b64encode(key_material).decode()}
    ciphertext, kid = byok_wrap(key_material, wrapping[mount])
    return {"alg": "vault-byok-rsa-oaep-sha256-aes-kwp", "kid": kid, "ciphertext_b64": ciphertext}


def write_csv(path, dev_id, k_auth, k_rec, k_sig, security):
    rows = [
        ["key", "type", "encoding", "value"],
        ["sec", "namespace", "", ""],
        ["prov_ver", "data", "u8", "1"],
        ["dev_id", "data", "hex2bin", dev_id.hex().upper()],
        ["k_auth", "data", "hex2bin", k_auth.hex().upper()],
        ["k_rec", "data", "hex2bin", k_rec.hex().upper()],
    ]
    if k_sig is not None:
        rows.append(["k_sig", "data", "hex2bin", k_sig.hex().upper()])
    rows.append(["prof", "data", "u8", str(SECURITY_CODE[security])])
    with open(path, "w", newline="", encoding="utf-8") as f:
        csv.writer(f, lineterminator="\n").writerows(rows)


def nvs_image(csv_path, bin_path, size, nvs_keys):
    """Gera a imagem da secnvs (cifrada com a nvs_key em devsec/prod, em claro no dev)."""
    cmd = [sys.executable, "-m", "esp_idf_nvs_partition_gen"]
    if nvs_keys:
        cmd += ["encrypt", str(csv_path), str(bin_path), hex(size), "--inputkey", str(nvs_keys)]
    else:
        cmd += ["generate", str(csv_path), str(bin_path), hex(size)]
    subprocess.run(cmd, check=True)


def main():
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--profile", choices=["dev", "devsec", "prod"], required=True)
    ap.add_argument("--security", choices=list(SECURITY_CODE), default="hmac")
    ap.add_argument("--dev-id", help="16 dígitos hexa (reprovisionar a mesma placa)")
    ap.add_argument("--out", type=pathlib.Path, default=HERE / "out")
    ap.add_argument("--kms-wrapping", type=pathlib.Path, help="kms_wrapping.json do tccelta-kms provision export-wrapping")
    ap.add_argument("--nvs-key", choices=["auto", "new", "board"], default="auto")
    ap.add_argument("--fe-key", type=pathlib.Path, default=pathlib.Path("keys/devsec/fe_key_devsec.bin"))
    ap.add_argument("--port")
    ap.add_argument("--flash", action="store_true")
    ap.add_argument("--yes", action="store_true")
    ap.add_argument("--import", dest="import_csv", type=pathlib.Path, help="secnvs.csv de outra origem")
    args = ap.parse_args()

    if args.flash and args.profile == "prod":
        raise SystemExit("prod não tem --flash: a gravação é o passo 5 do procedimento (spec 01 §10.3)")
    if args.profile == "prod" and args.security == "none":
        raise SystemExit("prod não aceita o perfil none")

    dev_id = bytes.fromhex(args.dev_id) if args.dev_id else new_dev_id()
    if len(dev_id) != 8:
        raise SystemExit("--dev-id precisa de 16 dígitos hexa")
    k_auth, k_rec = secrets.token_bytes(32), secrets.token_bytes(32)
    k_sig, k_sig_public = new_k_sig() if args.security == "asym" else (None, None)

    out = args.out / dev_id.hex().upper()
    (out / "keys").mkdir(parents=True, exist_ok=True)
    csv_path = args.import_csv or out / "secnvs.csv"
    if not args.import_csv:
        write_csv(csv_path, dev_id, k_auth, k_rec, k_sig, args.security)

    # Chave XTS da NVS: só devsec/prod (plano B1: o dev fica em claro).
    nvs_keys = None
    if args.profile == "dev":
        if args.nvs_key != "auto":
            print("aviso: --nvs-key é ignorado no perfil dev (NVS em claro)", file=sys.stderr)
    else:
        if args.nvs_key == "board":
            raise SystemExit("--nvs-key board: leia a nvs_key da placa com esptool e decifre com a chave FE (spec 01 §11)")
        nvs_keys = out / "keys" / "nvs_keys.bin"
        subprocess.run(
            [sys.executable, "-m", "esp_idf_nvs_partition_gen", "generate-key", "--keyfile", nvs_keys.name,
             "--outdir", str(nvs_keys.parent.parent)],
            check=True,
        )
        generated = out / "keys" / nvs_keys.name
        if not generated.exists():
            raise SystemExit("esp_idf_nvs_partition_gen não gerou nvs_keys.bin")

    secnvs_offset, secnvs_size = partition("secnvs")
    secnvs_bin = out / "secnvs.bin"
    nvs_image(csv_path, secnvs_bin, secnvs_size, nvs_keys)

    wrapping = load_wrapping(args.kms_wrapping) if args.kms_wrapping else None
    if wrapping is None:
        print("aviso: registro KMS com alg=none (sem --kms-wrapping); só para dev/devsec", file=sys.stderr)
    record = {
        "schema": "tccelta.kms.v1",
        "dev_id": dev_id.hex().upper(),
        "security_profile": args.security,
        "created_at": datetime.datetime.now(datetime.timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "k_auth": kms_field(k_auth, wrapping, "auth"),
        "k_rec": kms_field(k_rec, wrapping, "rec"),
    }
    if k_sig_public:
        record["k_sig_public"] = k_sig_public.hex().upper()
        print("chave pública de k_sig:", record["k_sig_public"])
    (out / "kms_record.json").write_text(json.dumps(record, indent=2))
    if args.profile in ("dev", "devsec"):
        (out / "bench_keys.json").write_text(
            json.dumps({"dev_id": dev_id.hex().upper(), "k_auth": k_auth.hex().upper(), "k_rec": k_rec.hex().upper()}, indent=2)
        )

    if args.flash:
        if not args.yes and args.profile == "devsec":
            raise SystemExit("devsec: confirme com --yes (a nvs existente pode ficar ilegível)")
        cmd = [sys.executable, "-m", "esptool"] + (["--port", args.port] if args.port else [])
        subprocess.run(cmd + ["write_flash", hex(secnvs_offset), str(secnvs_bin)], check=True)
    print(f"DevId {dev_id.hex().upper()} -> {out}")


if __name__ == "__main__":
    main()
