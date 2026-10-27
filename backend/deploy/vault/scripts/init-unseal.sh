#!/bin/sh
# Init/unseal SOMENTE de laboratorio (1 share, threshold 1). Producao: 5 shares, threshold 3, unseal manual.
set -eu
if [ "${AMBIENTE:-}" = "producao" ]; then echo "recusado: AMBIENTE=producao" >&2; exit 1; fi
echo "############ AVISO: init com 1 share so para laboratorio ############"
i=0
while [ $i -lt 60 ]; do
  set +e; vault status >/dev/null 2>&1; rc=$?; set -e
  if [ $rc -eq 0 ] || [ $rc -eq 2 ]; then break; fi
  i=$((i+1)); sleep 1
done
set +e; vault status >/dev/null 2>&1; rc=$?; set -e
if [ $rc -eq 0 ]; then echo "vault ja deselado"; exit 0; fi
if [ ! -f /vault/init/init.json ]; then
  vault operator init -key-shares=1 -key-threshold=1 -format=json > /vault/init/init.json
  chmod 600 /vault/init/init.json
fi
KEY=$(grep -A1 unseal_keys_b64 /vault/init/init.json | tail -1 | sed 's/[^"]*"\([^"]*\)".*/\1/')
vault operator unseal "$KEY" >/dev/null
echo "vault deselado"
