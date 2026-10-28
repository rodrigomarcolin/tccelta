# Registro de dongle (`tccelta.dongle.registro.v1`)

Contrato por **arquivo** entre o KMS e o backend no provisionamento. O KMS importa as chaves no Vault e grava
o `registro.json`; o backend lê o arquivo e cadastra o dongle no banco. Nada de segredo vai no arquivo.

```
tccelta-kms provisionar <novo|importar|importar-registro> --saida DIR   # grava DIR/<devid>/registro.json
tccelta provision register --file DIR/<devid>/registro.json        # insere dongle + dongle_state
tccelta provision register --file DIR                              # todos os DIR/*/registro.json
```

## Ordem e garantias

1. **KMS primeiro, banco depois.** Se o segundo passo não acontecer, sobra no Vault uma chave órfã
   (inofensiva, sem uso); nunca existe dongle no banco sem chave no Vault. (Antes, banco e Vault compartilhavam
   uma transação; agora são dois processos.)
2. **Idempotência dos dois lados.** No KMS, chave que já existe no Transit é pulada e só o registro é reemitido
   (inclusive quando só uma das duas, K_auth ou K_rec, tinha sido importada). No backend, DevId já cadastrado é
   pulado (`já estava cadastrado`).
3. `provisionar novo` escolhe um DevId livre consultando só o Vault (`<mount-auth>/keys/<devid hex>`): todo
   dongle cadastrado tem chave lá.

## Schema

```json
{
  "schema": "tccelta.dongle.registro.v1",
  "dev_id": "0011223344556677",
  "perfil": "hmac",
  "kms_chave_auth": "0011223344556677",
  "kms_chave_rec": "0011223344556677",
  "descricao": "dongle de teste"
}
```

| Campo | Tipo | Regra |
|---|---|---|
| `schema` | string | exatamente `tccelta.dongle.registro.v1` |
| `dev_id` | string | 16 dígitos hexadecimais (8 bytes) |
| `perfil` | string | `hmac`, `asym` ou `none` |
| `kms_chave_auth` | string | nome da chave K_auth no Vault (hoje, o `dev_id` em hex); obrigatório |
| `kms_chave_rec` | string | nome da chave K_rec no Vault; obrigatório |
| `chave_publica` | string | hex dos 65 B não comprimidos (`04…`) da chave de assinatura do dongle; **obrigatória** no perfil `asym`, omitida nos demais |
| `descricao` | string | opcional |

O backend recusa campos desconhecidos, schema diferente e valores inválidos **antes** de tocar no banco (com um
diretório, valida todos os registros primeiro).

## Testes do contrato

- `kms/internal/provisionamento` confere que o KMS grava exatamente o texto acima.
- `backend/internal/provisioning` confere que o backend aceita exatamente esse texto e recusa as variações
  inválidas.
