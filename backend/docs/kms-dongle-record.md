# Registro de dongle (`tccelta.dongle.record.v1`)

Contrato por **arquivo** entre o KMS e o backend no provisionamento. O KMS importa as chaves no Vault e grava
o `record.json`; o backend lê o arquivo e cadastra o dongle no banco. Nada de segredo vai no arquivo.

```
tccelta-kms provision <new|import|import-record> --out DIR   # grava DIR/<devid>/record.json
tccelta provision register --file DIR/<devid>/record.json        # insere dongle + dongle_state
tccelta provision register --file DIR                              # todos os DIR/*/record.json
```

No perfil `asym` o `record.json` leva a `public_key` (derivada da `k_sig` gerada por `provision new --profile asym`,
lida do CSV de teste ou vinda do `k_sig_public` do `kms_record.json` do firmware); o backend a guarda em
`dongle.public_key` e verifica os blocos localmente, sem o KMS.

## Ordem e garantias

1. **KMS primeiro, banco depois.** Se o segundo passo não acontecer, sobra no Vault uma chave órfã
   (inofensiva, sem uso); nunca existe dongle no banco sem chave no Vault. (Antes, banco e Vault compartilhavam
   uma transação; agora são dois processos.)
2. **Idempotência dos dois lados.** No KMS, chave que já existe no Transit é pulada e só o registro é reemitido
   (inclusive quando só uma das duas, K_auth ou K_rec, tinha sido importada). No backend, DevId já cadastrado é
   pulado (`já estava cadastrado`).
3. `provision new` escolhe um DevId livre consultando só o Vault (`<mount-auth>/keys/<devid hex>`): todo
   dongle cadastrado tem chave lá.

## Schema

```json
{
  "schema": "tccelta.dongle.record.v1",
  "dev_id": "0011223344556677",
  "profile": "hmac",
  "kms_auth_key": "0011223344556677",
  "kms_rec_key": "0011223344556677",
  "description": "dongle de teste"
}
```

| Campo | Tipo | Regra |
|---|---|---|
| `schema` | string | exatamente `tccelta.dongle.record.v1` |
| `dev_id` | string | 16 dígitos hexadecimais (8 bytes) |
| `profile` | string | `hmac`, `asym` ou `none` |
| `kms_auth_key` | string | nome da chave K_auth no Vault (hoje, o `dev_id` em hex); obrigatório |
| `kms_rec_key` | string | nome da chave K_rec no Vault; obrigatório |
| `public_key` | string | hex dos 65 B não comprimidos (`04…`) da chave de assinatura do dongle; **obrigatória** no perfil `asym`, omitida nos demais |
| `description` | string | opcional |

O backend recusa campos desconhecidos, schema diferente e valores inválidos **antes** de tocar no banco (com um
diretório, valida todos os registros primeiro).

## Testes do contrato

- `backend/internal/kms/provisioning` confere que o KMS grava exatamente o texto acima.
- `backend/internal/tools/provisioning` confere que o backend aceita exatamente esse texto e recusa as variações
  inválidas.
