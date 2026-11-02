#!/bin/sh
# Copia a chave montada (somente leitura, com permissao do Windows) para um lugar com 600 e a carrega num ssh-agent.
set -eu
if [ -f /ssh/key ]; then
  install -d -m 700 /root/.ssh
  install -m 600 /ssh/key /root/.ssh/id
  eval "$(ssh-agent -s)" >/dev/null
  ssh-add /root/.ssh/id          # pede a senha da chave, se houver (o contêiner roda com -it)
fi
exec "$@"
