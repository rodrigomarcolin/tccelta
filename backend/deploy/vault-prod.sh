#!/bin/sh
# Vault em producao (AMBIENTE=producao): init, unseal e bootstrap, a mao e reproduziveis.
# Rodar NA VPS, como o usuario deploy, a partir de qualquer pasta:
#
#   sh /opt/tccelta/deploy/vault-prod.sh status      # estado atual
#   sh /opt/tccelta/deploy/vault-prod.sh init        # uma unica vez: 5 chaves, limiar 3
#   sh /opt/tccelta/deploy/vault-prod.sh unseal      # abre com 3 chaves (apos init e apos todo reinicio)
#   sh /opt/tccelta/deploy/vault-prod.sh bootstrap   # uma unica vez: Transit, politicas, AppRoles, auditoria
#
# As chaves de unseal e o token raiz nunca sao gravados por este script: sao digitados (sem eco) ou impressos
# no terminal. O unico arquivo temporario e o init.json do bootstrap (permissao 600), apagado ao fim.
set -eu
cd "$(dirname "$0")/.."

dc() { docker compose -f compose.yaml -f compose.prodlike.yaml -f compose.prod.yaml "$@"; }

# vault status: rc 0 = aberto, 2 = selado, outro = erro
aberto() { dc exec -T vault vault status >/dev/null 2>&1; }

case "${1:-}" in
  status)
    dc exec -T vault vault status || true
    ;;

  init)
    echo "Copie as 5 chaves e o token raiz DIRETO para o gerenciador de senhas, em lugares diferentes."
    echo "Nao redirecione para arquivo: esta e a unica vez que aparecem."
    printf "Continuar? [digite init] "
    read -r R
    [ "$R" = "init" ] || { echo "cancelado"; exit 1; }
    dc exec vault vault operator init -key-shares=5 -key-threshold=3
    ;;

  unseal)
    n=0
    while [ "$n" -lt 3 ]; do
      if aberto; then break; fi
      dc exec -it vault vault operator unseal
      n=$((n + 1))
    done
    dc exec -T vault vault status || true
    aberto && echo "Vault aberto." || { echo "Vault ainda selado: repita com mais chaves." >&2; exit 1; }
    ;;

  bootstrap)
    aberto || { echo "Vault selado ou fora do ar: rode 'unseal' antes." >&2; exit 1; }
    echo "Cole o token raiz (nao aparece na tela). Ele e gravado em /vault/init/init.json (600) so durante o bootstrap."
    dc exec -it vault sh -c 'umask 077; printf "Token raiz: "; read -rs T; echo;
      printf "{\"root_token\": \"%s\"}\n" "$T" > /vault/init/init.json'
    dc run --rm vault-bootstrap          # termina com: bootstrap do Vault concluido (perfil=producao)
    echo "Apagando o init.json e revogando o token raiz (digite o token de novo)."
    dc exec -it vault sh -c 'rm -f /vault/init/init.json; printf "Token raiz: "; read -rs T; echo;
      VAULT_TOKEN="$T" vault token revoke -self'
    echo "Pronto. Proximo passo: bash deploy/ansible/run.sh stack.yml (no PC)."
    ;;

  *)
    echo "uso: sh deploy/vault-prod.sh status|init|unseal|bootstrap" >&2
    exit 2
    ;;
esac
