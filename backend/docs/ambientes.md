# Ambientes: dev, prodlike e prod

São três níveis de segurança sobre o **mesmo código e as mesmas imagens**. Muda a configuração, não o programa.

- **dev** (`make up`): conveniência primeiro. Tudo em HTTP, Vault em modo de desenvolvimento, portas abertas em
  `127.0.0.1` e dongles de teste à mão.
- **prodlike** (`make up-prodlike`): a stack de desenvolvimento com os guardrails de **transporte e de segredos**
  ligados: HTTPS e mTLS em toda a cadeia, Vault de verdade, nada publicado no host. O nome quer dizer "com os
  guardrails de produção", não "idêntico à produção": a PKI é de laboratório e o Vault é de um nó só.
- **prod** (`AMBIENTE=producao`): além do transporte, o código passa a **proibir** os atalhos de teste. O repositório
  traz só o desenho do KMS em um host separado; **não há compose nem alvo do Makefile para a API em produção**.

Existe ainda a **stack de teste** (`compose.test.yaml`, projeto `tccelta-teste`), que não é um nível de segurança: é
uma stack isolada para o `make e2e`. Ela é descrita no [README](../README.md#stack-de-teste).

## Comparação

| Aspecto | dev | prodlike | prod |
|---|---|---|---|
| Como sobe | `make up` (`compose.yaml`) | `make up-prodlike` (`compose.yaml` + `compose.prodlike.yaml`, perfil `tls`) | Não há alvo. Só `deploy/kms/compose.yaml` (Vault + KMS num host, atrás de VPN); a API fica em outro host, sem definição aqui |
| `AMBIENTE` | `dev` | `prodlike` | `producao` |
| Vault | Modo dev: em memória (perde tudo ao reiniciar), token raiz fixo em arquivo, porta 8200 no host | Modo servidor: raft em volume, TLS 1.3, sem token raiz de desenvolvimento | Modo servidor com a mesma configuração (`vault.hcl`), com `IPC_LOCK`; só o KMS o alcança |
| Init e unseal do Vault | Já nasce aberto | Automático, **de laboratório**: 1 chave, limiar 1, guardada em `init.json` no volume `vault_init` | Manual: o `init-unseal.sh` **recusa** `producao`. O script registra a intenção: 5 chaves, limiar 3 |
| Auditoria do Vault | Desligada | Ligada (`auditoria.log`) | Ligada (o bootstrap liga em tudo que não é dev) |
| KMS → Vault | HTTP | HTTPS (CA de laboratório) | HTTPS |
| API → KMS | `http://kms:8300` | `https://kms:8300` com mTLS | O KMS exige mTLS. Pelo comentário do `deploy/kms/compose.yaml`, a API precisa de `KMS_URL` https, do certificado de cliente e da CA; a validação do lado da API não foi conferida |
| Identidade do cliente do KMS (mTLS) | Não confere (HTTP simples) | **Sim**: certificado de cliente assinado pela CA e com `CN` igual a `KMS_TLS_CLIENTE_CN` (padrão `tccelta-api`, o do `api-cliente.pem`); outro `CN` da mesma CA recebe `403 cliente_nao_autorizado` | **Sim**, mesma regra: sem `KMS_TLS_CERT`, `KMS_TLS_KEY` e `KMS_TLS_CLIENT_CA` o KMS nem inicia. O certificado de cliente da API precisa ter o `CN` configurado, ou o valor de `KMS_TLS_CLIENTE_CN` do KMS tem de ser ajustado ao `CN` emitido pela PKI |
| Portas no host | Banco 5432, Vault 8200, API 8080-8081 | Nenhuma dessas; só o Caddy (8443) | O KMS na interface da VPN (`KMS_BIND_IP:8300`); o Vault sem porta no host |
| Redes `dados`, `kms`, `cofre` | Comuns | `internal: true` (sem saída para fora) | No `deploy/kms/compose.yaml`, uma rede `kms` comum só entre o Vault e o KMS; as redes da API não são definidas aqui |
| Certificados | Nenhum | CA de laboratório (`make certs`, validade de 1 ano) | PKI própria; o script de laboratório não serve |
| Dongles de teste (`seed-dev`, `provisionar novo`, `cenario`) | Funcionam | O KMS os permite, mas o `kms-ferramentas` não foi ajustado ao Vault em TLS: **na prática não funcionam** | **Recusados** pelo KMS (`provisionar novo` e `importar` do CSV) |
| Registro de chave com `alg none` | Aceito | Aceito | **Recusado** |
| Perfil de bloco `none` (`PERMITIR_PERFIL_NONE`) | Pode ser ligado | Pode ser ligado | **Proibido**: configuração inválida |

Na coluna **prod**, as linhas "Identidade do cliente do KMS", "Dongles de teste", "`alg none`" e "Perfil de bloco
`none`" são regras **do código**, que valem com `AMBIENTE=producao`. As demais descrevem o desenho do deploy e
dependem de como ele é implantado.

O `AMBIENTE=prodlike` só significa "não é dev": o KMS e a API tratam qualquer valor diferente de `dev` como
"exigir mTLS e conferir o CN do cliente". O que separa `prodlike` de `producao` no código Go são três proibições
(dongles de teste, `alg none` e perfil de bloco `none`); no script de init do Vault, a recusa do init de laboratório.

## O que o prodlike aplica

`make up-prodlike` aplica o `compose.prodlike.yaml` sobre a stack:

- o Vault passa a rodar em modo servidor (raft, TLS 1.3, auditoria), sem porta publicada;
- o banco deixa de publicar a porta 5432;
- o KMS roda com `AMBIENTE=prodlike`: **TLS com certificado de cliente obrigatório** (mTLS), e a API se
  apresenta com o certificado `api-cliente.pem` gerado pelo `make certs`;
- o serviço `vault-unseal` inicializa e abre o Vault antes do `vault-bootstrap`.

Fora de `AMBIENTE=dev` o KMS recusa conexões sem certificado de cliente assinado pela CA configurada em
`KMS_TLS_CLIENT_CA` e com o CN igual a `KMS_TLS_CLIENTE_CN` (padrão `tccelta-api`, o do `api-cliente.pem`); outro
certificado da mesma CA recebe `403 cliente_nao_autorizado`.

## O que nem o prodlike nem o repositório entregam

Para uma produção de verdade faltam:

- uma CA e uma PKI de verdade (a de laboratório vale 1 ano);
- o unseal com várias chaves e o auto-unseal; o Vault roda em um nó (raft, `node_id = "kms1"`), sem alta
  disponibilidade;
- backup do banco e do Vault;
- um compose ou alvo para a API em produção.

## Trocar de um nível para outro

Os dois primeiros compartilham o projeto `tccelta` e o volume do banco. Ao ir de dev para prodlike (ou voltar),
rode `make limpar` primeiro: o Vault muda de modo (memória para raft). O bootstrap regenera um `secret_id` inválido
que sobre no volume, mas começar limpo evita surpresa. Depois: `make certs` e `make up-prodlike` (ou só `make up`).
O `make limpar` apaga os dados de desenvolvimento.

**Contêineres de perfis desligados.** O `docker compose down` não remove os serviços de perfis que não estão
ativos no comando (por exemplo o `caddy`, do perfil `tls`). Se a rede do projeto foi recriada, esse contêiner
antigo guarda o ID da rede velha e o próximo `up` falha com `network ... not found`. Para derrubar tudo, inclusive
o overlay prodlike e os perfis, use `make down-tudo` ou `make limpar-tudo`, que passam todos os perfis.
