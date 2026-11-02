# Implantação em VPS (Ansible em contêiner)

Os playbooks ficam em `deploy/ansible/` e rodam de um contêiner no seu PC, por SSH. Todos os comandos abaixo
são executados no Git Bash, na pasta `backend/`.

## O que o Ansible assume

- A VPS é Debian/Ubuntu, com **SSH só por chave** e **sem login de root** já configurados por você (o playbook
  não mexe no `sshd`).
- Existe um usuário com `sudo` (aqui, `davi`), que entra com a chave `~/.ssh/id_ed25519_contabo` (com passphrase).
- O Docker precisa estar rodando no PC.

## Antes do primeiro `site.yml`

1. Edite `deploy/ansible/group_vars/all.yml`: `vps_ip`, `acme_email`, `dbeaver_ips`. Na etapa 1 (só IP),
   deixe `dominio_api` e `dominio_db` vazios.
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

## Outras tags de `site.yml`

| Tag | Uso |
|---|---|
| `base` | Servidor: usuário, pacotes, Docker, firewall |
| `pki` | Certificados (roda no PC) |
| `app` | Aplicação: código, `.env`, segredos, certificados, `pg_hba`, firewall do banco |
| `codigo` | Só sincroniza o código (rsync) |

Sem `--tags`, roda tudo.
