# Implantação em VPS (Ansible em contêiner)

Os playbooks ficam em `deploy/ansible/` e rodam de um contêiner no seu PC, por SSH. Todos os comandos abaixo
são executados no Git Bash, na pasta `backend/`.

## O que o Ansible assume

- A VPS é Debian/Ubuntu, com **SSH só por chave** e **sem login de root** já configurados por você (o playbook
  não mexe no `sshd`).
- Existe um usuário com `sudo` (aqui, `davi`), que entra com a chave `~/.ssh/id_ed25519_contabo` (com passphrase).
- O Docker precisa estar rodando no PC.

## Antes do primeiro `site.yml`

1. Crie `deploy/ansible/group_vars/all/local.yml` (não versionado) a partir do modelo e preencha `vps_ip`,
   `acme_email` e `dbeaver_ips`. Os domínios (`dominio_api`, `dominio_db`) ficam em `group_vars/all/main.yml`; na etapa 1 (só IP) deixe-os `""`:

   ```bash
   cp deploy/ansible/group_vars/all/local.yml.example deploy/ansible/group_vars/all/local.yml
   ```

   O playbook para com uma mensagem se esses valores faltarem ou ainda forem os do modelo.
2. Aponte a chave (o `run.sh` monta a privada e a `.pub` do mesmo caminho). Para valer na sessão toda:

   ```bash
   export SSH_KEY=~/.ssh/id_ed25519_contabo
   ```

## Primeiro `site.yml`

```bash
bash deploy/ansible/run.sh site.yml -e ansible_user=davi --ask-become-pass --tags base,pki
```

O que cada parte faz:

| Parte | Efeito |
|---|---|
| `-e ansible_user=davi` | Conecta como `davi` em vez do `deploy` do inventário, que ainda não existe na VPS |
| `--ask-become-pass` | Pergunta a senha de `sudo` do `davi` (só na memória da execução) |
| `--tags base,pki` | Roda só os plays marcados `base` (servidor) e `pki` (certificados, no seu PC). Sem isso, o `site.yml` também faria a aplicação, que depende de passos manuais antes |

Durante a execução o terminal pede, nesta ordem, a **passphrase da chave** (o `ssh-add` do contêiner) e a
**senha de sudo**.

O play `base` cria o usuário `deploy` com a sua chave e `sudo` sem senha, instala pacotes e o Docker, desliga o
swap e configura UFW (22, 80, 443/tcp, 443/udp) e fail2ban. Se o seu SSH não estiver na porta 22, ajuste a
lista do UFW em `site.yml` antes de rodar, ou você perde o acesso.

Confirme ao fim:

```bash
ssh -i ~/.ssh/id_ed25519_contabo deploy@IP_DA_VPS 'docker --version && sudo -n true && echo ok'
```

## Depois do primeiro

1. Guarde `deploy/ansible/.pki-local/` em backup cifrado: é a única cópia das chaves das CAs.
2. A partir daqui o usuário é o `deploy`, só com a chave:

   ```bash
   bash deploy/ansible/run.sh site.yml      # aplicação, segredos, certificados, firewall, Vault selado
   ```

3. Init, unseal e bootstrap do Vault, à mão (as chaves de unseal e o token raiz nunca passam pelo Ansible).
4. `bash deploy/ansible/run.sh stack.yml` sobe o resto.

Os passos completos, a etapa 2 (domínio) e o provisionamento de dongles estão no runbook de implantação.

## Vault em produção: init, unseal e bootstrap

Fica fora do Ansible de propósito: as 5 chaves de unseal e o token raiz só passam pelo seu terminal. O
`init-unseal.sh` do repositório é de laboratório (1 chave) e **recusa** `AMBIENTE=producao`. O procedimento de
produção está em `deploy/vault-prod.sh`, que roda **na VPS** (usuário `deploy`):

```bash
ssh -i ~/.ssh/id_ed25519_contabo deploy@IP_DA_VPS
sh /opt/tccelta/deploy/vault-prod.sh status      # esperado antes do init: Initialized false, Sealed true
sh /opt/tccelta/deploy/vault-prod.sh init        # UMA vez: 5 chaves, limiar 3
sh /opt/tccelta/deploy/vault-prod.sh unseal      # pede 3 chaves (sem eco)
sh /opt/tccelta/deploy/vault-prod.sh bootstrap   # UMA vez: Transit, políticas, AppRoles, auditoria
```

| Subcomando | O que faz | Quando |
|---|---|---|
| `status` | `vault status` | Sempre |
| `init` | `vault operator init -key-shares=5 -key-threshold=3`. Imprime as 5 chaves e o token raiz **uma única vez** | Uma vez, na implantação |
| `unseal` | Pede chaves até o Vault abrir (3) | Após o `init` e **após todo reinício** do Vault ou da VPS |
| `bootstrap` | Grava o token raiz em `/vault/init/init.json` (600), roda o `vault-bootstrap`, apaga o arquivo e revoga o token raiz | Uma vez, com o Vault aberto |
| `provisionador` | Gera um token raiz temporário com 3 das 5 chaves, emite um token de provisionamento de 24 h (órfão), grava no volume e revoga o raiz temporário | Antes de provisionar dongles, se `dc run kms-ferramentas ...` der `invalid token` (expira em 24 h) |

Regras:

- **Guarde as 5 chaves e o token raiz fora da VPS**, em lugares diferentes (gerenciadores de senhas, mídia
  offline). O script nunca as grava. Perder 3 ou mais chaves torna o Vault irrecuperável, e com ele as chaves dos
  dongles.
- O Vault precisa ficar **aberto** depois do bootstrap: o KMS o consulta a cada operação. Ele volta selado a cada
  reinício do contêiner; rode `unseal` de novo.
- Se o `bootstrap` falhar no meio, o `init.json` pode ficar no volume. Apague-o antes de seguir:
  `dc exec vault rm -f /vault/init/init.json` (com o alias `dc` do runbook) e revogue o token raiz.
- Um token raiz novo, para uma mudança administrativa: `vault operator generate-root` (3 das 5 chaves).
- Sem `-T`, os comandos pedem um terminal: rode por SSH interativo, não por script sem TTY.

O procedimento acima ainda não foi executado contra um Vault de verdade (nem os comandos equivalentes do runbook).

## Outras tags de `site.yml`

| Tag | Uso |
|---|---|
| `base` | Servidor: usuário, pacotes, Docker, firewall |
| `pki` | Certificados (roda no PC) |
| `app` | Aplicação: código, `.env`, segredos, certificados, `pg_hba`, firewall do banco |
| `codigo` | Cria a pasta da aplicação e sincroniza o código (rsync) |
| `dbeaver` | Só o acesso ao banco: `pg_hba.conf` e firewall da 5432. Use para trocar IPs permitidos |

Para liberar ou trocar um IP do DBeaver: edite `dbeaver_ips` em `group_vars/all/local.yml` e rode
`bash deploy/ansible/run.sh site.yml --tags dbeaver`. Sem `rsync`, sem `stack.yml` e sem recriar o banco. No fim, o
playbook confere o firewall (um `RETURN` por IP e `DROP` no fim) e o `pg_hba.conf` visto pelo contêiner. Cada
item de `dbeaver_ips` precisa ser IPv4 em CIDR (ex.: `203.0.113.5/32`).

Sem `--tags`, roda tudo.
