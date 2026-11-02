#!/usr/bin/env bash
# Uso (a partir de backend/): sh deploy/ansible/run.sh site.yml [opcoes do ansible-playbook]
# SSH_KEY: chave privada que entra na VPS (padrao ~/.ssh/id_ed25519). A pasta backend/ inteira e montada em /work.
set -eu
export MSYS_NO_PATHCONV=1
cd "$(dirname "$0")/../.."
KEY="${SSH_KEY:-$HOME/.ssh/id_ed25519}"
docker build -q -t tccelta-ansible deploy/ansible >/dev/null
exec docker run --rm -it \
  -v "$KEY:/ssh/key:ro" -v "$KEY.pub:/ssh/key.pub:ro" \
  -v "$PWD:/work" -w /work/deploy/ansible \
  tccelta-ansible ansible-playbook "$@"
