#!/usr/bin/env python
"""Copia os DADOS do schema `public` do banco tccelta remoto para o tccelta local.

O schema (partições hash, hypertables Timescale, triggers, funções, GRANTs) NÃO é
recriado aqui: ele vem das migrations. Pré-requisito no destino:

    cd backend && make up && make migrate     # local na MESMA versão do remoto

O script confere se goose_db_version é idêntica nos dois lados (aborta se não for),
esvazia as tabelas locais e copia tudo com COPY. A leitura da origem é uma única
transação REPEATABLE READ READ ONLY (snapshot consistente). No destino usa
session_replication_role=replica (precisa de superusuário): FKs e triggers não
disparam durante a carga, então a ordem das tabelas não importa.

Requisitos:  pip install "psycopg[binary]>=3.1"

    export SRC_PASSWORD=...  DST_PASSWORD=...
    python export_db.py --src-user dbeaver_ro --dst-user postgres

Variáveis alternativas às flags: {SRC,DST}_{HOST,PORT,USER,PASSWORD,DB,SSLMODE}.
"""
from __future__ import annotations

import argparse
import os
import sys

import psycopg

SCHEMA = "public"


def env(name: str, default: str | None = None) -> str | None:
    return os.environ.get(name, default)


def qid(name: str) -> str:
    return '"' + name.replace('"', '""') + '"'


def parse_args() -> argparse.Namespace:
    p = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    for side, host, user in (("src", "tccelta-db.malcong.com.br", None), ("dst", "localhost", "postgres")):
        up = side.upper()
        p.add_argument(f"--{side}-host", default=env(f"{up}_HOST", host))
        p.add_argument(f"--{side}-port", type=int, default=int(env(f"{up}_PORT", "5432")))
        p.add_argument(f"--{side}-user", default=env(f"{up}_USER", user))
        p.add_argument(f"--{side}-db", default=env(f"{up}_DB", "tccelta"))
        p.add_argument(f"--{side}-sslmode", default=env(f"{up}_SSLMODE", "require" if side == "src" else "prefer"))
    p.add_argument("--yes", action="store_true", help="não pedir confirmação antes de esvaziar as tabelas locais")
    return p.parse_args()


def connect(a: argparse.Namespace, side: str) -> psycopg.Connection:
    g = lambda k: getattr(a, f"{side}_{k}")
    return psycopg.connect(
        host=g("host"), port=g("port"), user=g("user"), dbname=g("db"),
        password=env(f"{side.upper()}_PASSWORD"), sslmode=g("sslmode"),
    )


def rows(conn: psycopg.Connection, query: str, *params):
    with conn.cursor() as cur:
        cur.execute(query, params)
        return cur.fetchall()


# Tabelas reais do schema: sem partições-filhas (o COPY na tabela-mãe roteia) e sem
# objetos de extensões (spatial_ref_sys é do PostGIS). Chunks do Timescale ficam em
# _timescaledb_internal, fora do schema public.
TABLES_SQL = """
    SELECT c.relname FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = %s AND c.relkind IN ('r','p') AND NOT c.relispartition
      AND NOT EXISTS (SELECT 1 FROM pg_depend x WHERE x.objid = c.oid AND x.deptype = 'e')
    ORDER BY c.relname"""

# Sequences ligadas a colunas de tabelas (serial/identity).
SEQ_SQL = """
    SELECT sc.relname FROM pg_class sc JOIN pg_namespace n ON n.oid = sc.relnamespace
    WHERE n.nspname = %s AND sc.relkind = 'S'
      AND NOT EXISTS (SELECT 1 FROM pg_depend x WHERE x.objid = sc.oid AND x.deptype = 'e')"""

OWNED_SQL = """
    SELECT sc.relname, tc.relname, a.attname
    FROM pg_depend d
    JOIN pg_class sc ON sc.oid = d.objid AND sc.relkind = 'S'
    JOIN pg_class tc ON tc.oid = d.refobjid
    JOIN pg_attribute a ON a.attrelid = tc.oid AND a.attnum = d.refobjsubid
    JOIN pg_namespace n ON n.oid = sc.relnamespace
    WHERE n.nspname = %s AND d.deptype IN ('a','i')"""

COLS_SQL = """
    SELECT a.attname FROM pg_attribute a
    JOIN pg_class c ON c.oid = a.attrelid JOIN pg_namespace n ON n.oid = c.relnamespace
    WHERE n.nspname = %s AND c.relname = %s AND a.attnum > 0
      AND NOT a.attisdropped AND a.attgenerated = ''
    ORDER BY a.attnum"""


def main() -> int:
    a = parse_args()
    if not a.src_user:
        print("Informe --src-user (ou SRC_USER).", file=sys.stderr)
        return 2
    if a.dst_host not in ("localhost", "127.0.0.1", "::1"):
        print("Por segurança o destino precisa ser localhost.", file=sys.stderr)
        return 2

    src = connect(a, "src")
    src.execute("SET statement_timeout = 0")  # dbeaver_ro tem 30s por padrão
    src.execute("SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY")
    dst = connect(a, "dst")

    src_tables = [r[0] for r in rows(src, TABLES_SQL, SCHEMA)]
    dst_tables = [r[0] for r in rows(dst, TABLES_SQL, SCHEMA)]
    if set(src_tables) != set(dst_tables):
        print("Schemas diferentes. Rode `make migrate` no local.\n"
              f"  só na origem: {sorted(set(src_tables) - set(dst_tables))}\n"
              f"  só no destino: {sorted(set(dst_tables) - set(src_tables))}", file=sys.stderr)
        return 1

    vq = f"SELECT version_id FROM {SCHEMA}.goose_db_version WHERE is_applied"
    sv, dv = {r[0] for r in rows(src, vq)}, {r[0] for r in rows(dst, vq)}
    dst_ahead = sv < dv
    if sv - dv:
        print(f"A origem tem migrations que o local não tem: {sorted(sv - dv)}. "
              "Atualize o código/`make migrate` no local.", file=sys.stderr)
        return 1
    if dst_ahead:
        print(f"Local à frente da origem (migrations {sorted(dv - sv)} só no local): "
              "copiando as colunas em comum e preservando o goose_db_version local.")

    if not a.yes:
        r = input(f"Isso ESVAZIA {len(dst_tables)} tabelas de {a.dst_db}@{a.dst_host} (e, por CASCADE de FK, "
                  f"também as do schema gold, que depende de public.dongle) e copia de {a.src_host}. Digite 'sim': ")
        if r.strip().lower() != "sim":
            print("Cancelado.")
            return 1

    with dst.cursor() as cur:
        cur.execute("SET session_replication_role = replica")  # sem FKs/triggers durante a carga
        names = ", ".join(f"{SCHEMA}.{qid(t)}" for t in dst_tables
                          if not (dst_ahead and t == "goose_db_version"))
        cur.execute(f"TRUNCATE {names} RESTART IDENTITY CASCADE")

        for t in src_tables:
            if dst_ahead and t == "goose_db_version":
                continue
            scols = [c[0] for c in rows(src, COLS_SQL, SCHEMA, t)]
            dcols = {c[0] for c in rows(dst, COLS_SQL, SCHEMA, t)}
            if not set(scols) <= dcols:
                print(f"{t}: colunas só na origem {sorted(set(scols) - dcols)}; ignoradas", file=sys.stderr)
            cols = ", ".join(qid(c) for c in scols if c in dcols)
            with src.cursor() as scur, scur.copy(f"COPY (SELECT {cols} FROM {SCHEMA}.{qid(t)}) TO STDOUT") as out, \
                    cur.copy(f"COPY {SCHEMA}.{qid(t)} ({cols}) FROM STDIN") as inp:
                for chunk in out:
                    inp.write(chunk)
            cur.execute(f"SELECT count(*) FROM {SCHEMA}.{qid(t)}")
            print(f"{t}: {cur.fetchone()[0]} linhas", flush=True)

        # last_value vem de pg_sequences (NULL, sem erro, se o papel não tem acesso à sequence).
        # Sem acesso: usa max(coluna dona) no destino, já com os dados copiados.
        svals = dict(rows(src, "SELECT sequencename, last_value FROM pg_sequences WHERE schemaname = %s", SCHEMA))
        owners = {r[0]: (r[1], r[2]) for r in rows(dst, OWNED_SQL, SCHEMA)}
        for seq in (r[0] for r in rows(dst, SEQ_SQL, SCHEMA)):
            last = svals.get(seq)
            if last is None and seq in owners:
                tbl, col = owners[seq]
                cur.execute(f"SELECT max({qid(col)}) FROM {SCHEMA}.{qid(tbl)}")
                last = cur.fetchone()[0]
            if last is not None:
                cur.execute("SELECT setval(%s, %s, true)", (f"{SCHEMA}.{qid(seq)}", last))

    dst.commit()
    src.rollback()
    print("Concluído.")
    return 0


if __name__ == "__main__":
    sys.exit(main())
