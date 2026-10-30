#!/bin/sh
# Gera os arquivos de deploy/segredos/ se nao existirem (idempotente), via conteiner alpine.
set -eu
DIR="$(cd "$(dirname "$0")/.." && pwd)/deploy/segredos"
mkdir -p "$DIR"
for n in pg_senha_superusuario pg_senha_dono pg_senha_api pg_senha_worker pg_senha_provisionador vault_dev_root_token redis_senha; do
  if [ ! -s "$DIR/$n" ]; then
    MSYS_NO_PATHCONV=1 docker run --rm alpine:3.20 sh -c "apk add -q --no-cache openssl >/dev/null && openssl rand -hex 24" | tr -d '\r\n' > "$DIR/$n"
    chmod 600 "$DIR/$n" 2>/dev/null || true
  fi
done
[ -f "$DIR/cloudflared_token" ] || { : > "$DIR/cloudflared_token"; chmod 600 "$DIR/cloudflared_token" 2>/dev/null || true; }
echo "segredos em $DIR"
