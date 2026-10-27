# 03 — Arquitetura da API em Go e pacote `protocolo`

> Fase 3 · Servidor. Depende de: 01 (contêineres), 02 (schema para o sqlc). É dependência de: 04, 05, 06, 07, 08, 10.

## Objetivo

Definir o módulo Go do backend: layout de pacotes, dependências, configuração, banco (pgx + sqlc), HTTP (stdlib), observabilidade, limites de carga, desligamento gracioso e, principalmente, o pacote isolado **`protocolo`**. Esse pacote codifica e decodifica, byte a byte, as mensagens 0x01–0x04, os registros do bloco e a entrada do HMAC. É o mesmo pacote que o simulador usa (spec 10).

## Escopo

Dentro: estrutura de diretórios, `go.mod`, `sqlc.yaml`, pacote `protocolo` completo (formatos, assinaturas, testes com vetores), catálogo de erros HTTP, middlewares, métricas, configuração.

Fora: regras de negócio de cada endpoint (04, 05), worker (06), detalhes do KMS (07) e do JWT (08).

## Design

### Módulo e dependências

`backend/go.mod`:

```
module tccelta/backend

go 1.24

require (
    github.com/jackc/pgx/v5            v5.7.x   // driver, pool, COPY binário
    github.com/pressly/goose/v3        v3.22.x  // migrations (spec 02)
    github.com/hashicorp/vault/api     v1.15.x  // cliente Vault (spec 07)
    github.com/hashicorp/vault/api/auth/approle v0.8.x
    github.com/golang-jwt/jwt/v5       v5.2.x   // JWT (spec 08)
    github.com/google/uuid             v1.6.x   // jti
    github.com/prometheus/client_golang v1.20.x // métricas
    golang.org/x/time                  v0.8.x   // rate.Limiter
    golang.org/x/sync                  v0.10.x  // errgroup, semaphore
    github.com/hashicorp/golang-lru/v2 v2.0.x   // cache DevId→dongle e limitadores locais (expirable LRU)
    github.com/redis/go-redis/v9       v9.7.x   // estado compartilhado do rate limit (decisão do autor)
    github.com/testcontainers/testcontainers-go v0.34.x // só testes de integração
    github.com/alicebob/miniredis/v2   v2.34.x  // só testes unitários do limitador
)
```

**[Decisão] Roteador: `net/http.ServeMux` do Go ≥ 1.22** (padrões com método e curingas, como `POST /v1/dongles/{id}/desafio` e `r.PathValue("id")`). São quatro rotas: um roteador externo (chi) não acrescenta nada que justifique a dependência, e middlewares são funções `func(http.Handler) http.Handler` compostas à mão.

**[Decisão] SQL só em arquivos `.sql` com sqlc** (`internal/banco/queries/*.sql` ⇒ pacote gerado `internal/banco/consultas`). Nenhuma string SQL em código Go, com duas exceções explícitas e centralizadas em `internal/banco/tx.go` e `internal/banco/escuta.go`: `SET LOCAL lock_timeout/statement_timeout` e `LISTEN <canal>` (o sqlc não gera nenhum dos dois).

### Layout

```
backend/
├── cmd/
│   └── tccelta/                 # binário único
│       ├── main.go              # despacha subcomandos: api | worker | migrar | provisionar | simulador | saude
│       ├── api.go  worker.go  migrar.go  provisionar.go  simulador.go  saude.go
├── internal/
│   ├── config/                  # Config a partir de env; validação; *_ARQUIVO para segredos
│   ├── versao/                  # Versao (ldflags)
│   ├── protocolo/               # ★ formatos binários (sem dependências além da stdlib)
│   │   ├── tlv.go  devid.go  tag.go  perfil.go
│   │   ├── desafio.go  resposta.go  ack.go  bloco.go  registros.go  erros.go
│   │   ├── *_test.go  fuzz_test.go
│   │   └── testdata/vetores.json  testdata/vetores_firmware.json
│   ├── banco/
│   │   ├── pool.go              # pgxpool + AfterConnect (codec geography)
│   │   ├── tx.go                # EmTransacao(ctx, pool, opts, fn) + SET LOCAL
│   │   ├── escuta.go            # Escutar(ctx, pool, canal) <-chan string: conexão dedicada com LISTEN e reconexão
│   │   ├── geo.go               # tipo PontoZ + codec EWKB
│   │   ├── migracoes.go  migrations/*.sql
│   │   ├── queries/*.sql        # fonte do sqlc
│   │   └── consultas/           # GERADO pelo sqlc (não editar)
│   ├── dongles/                 # Cache DevId → Dongle (LRU com TTL)
│   ├── kms/                     # interfaces + implementação Vault (spec 07)
│   │   └── kmsteste/            # implementação em memória só para testes
│   ├── token/                   # emissão/validação de JWT (spec 08)
│   ├── pids/                    # catálogo de PIDs em memória + LISTEN pid_alterado
│   ├── handshake/               # regras do desafio e da resposta (spec 04)
│   ├── ingestao/                # regras do lote (spec 05)
│   ├── tempo/                   # algoritmo puro da âncora (spec 06)
│   ├── worker/                  # laço LISTEN/SKIP LOCKED (spec 06)
│   ├── httpapi/                 # rotas, handlers, middlewares, problem+json
│   ├── limites/                 # rate limit por chave, semáforo de concorrência
│   ├── metricas/                # registradores Prometheus
│   ├── provisionamento/         # geração de DevId/chaves + import no Vault (spec 07)
│   └── simulador/               # dongles virtuais (spec 10)
├── scripts/  deploy/  testdata/  saida/(gitignored)
├── sqlc.yaml  Makefile  Dockerfile  compose*.yaml
```

Regra de dependência: `protocolo` não importa nada do projeto. `handshake`/`ingestao` dependem de `protocolo`, `banco/consultas`, `kms` (interface), `token`, `pids` e `dongles`. `httpapi` só traduz HTTP ⇄ serviços. `cmd` monta tudo (injeção manual de dependências).

### Configuração (`internal/config`)

Todas as variáveis têm um padrão seguro. Segredos aceitam a forma `<NOME>_ARQUIVO` (Docker secrets). `config.Carregar()` falha no boot com uma lista de erros legível.

| Variável | Padrão | Uso |
|---|---|---|
| `AMBIENTE` | `dev` | `dev`/`prodlike`/`producao`; `producao` recusa `PERMITIR_PERFIL_NONE` e importação de chaves de teste |
| `HTTP_ENDERECO` | `:8080` | API |
| `METRICAS_ENDERECO` | `:9090` (api) / `:9091` (worker) | Prometheus, fora do Caddy |
| `SAUDE_ENDERECO_WORKER` | `:8090` | `/saude/viva` do worker |
| `BANCO_URL`, `BANCO_SENHA_ARQUIVO` | — | DSN pgx (senha injetada do arquivo) |
| `BANCO_POOL_MAX` / `BANCO_POOL_MIN` | `40` / `4` | pgxpool |
| `VAULT_ADDR`, `VAULT_CACERT`, `VAULT_CLIENT_CERT`, `VAULT_CLIENT_KEY` | — | spec 07 |
| `VAULT_APPROLE_ROLE_ID_ARQUIVO`, `VAULT_APPROLE_SECRET_ID_ARQUIVO` | — | spec 07 |
| `VAULT_TIMEOUT`, `VAULT_RETENTATIVAS` | `2s`, `1` | spec 07 |
| `VAULT_MOUNT_AUTH`, `VAULT_MOUNT_REC`, `VAULT_MOUNT_JWT`, `VAULT_CHAVE_JWT` | `transit-auth`, `transit-rec`, `transit-jwt`, `jwt` | spec 07 |
| `JWT_EMISSOR`, `JWT_AUDIENCIA`, `JWT_VALIDADE`, `JWT_FOLGA` | `tccelta-api`, `tccelta-ingestao`, `1h`, `30s` | spec 08 |
| `DESAFIO_VALIDADE` | `60s` | prazo do desafio |
| `DESAFIO_MAX_PENDENTES` | `3` | desafios não respondidos e não vencidos por dongle |
| `ANCORA_RTT_MAX` | `2s` | âncora só se S2 − S1 ≤ isto |
| `T_LAT_SUGERIDO` | `60s` (`0` = omitir) | devolvido ao app na resposta do handshake |
| `LOTE_MAX_BLOCOS` | `32` | acima disso, 429 |
| `INGESTAO_MAX_SIMULTANEAS` | `32` | semáforo por réplica (≤ `BANCO_POOL_MAX` − folga) |
| `INGESTAO_ESPERA_VAGA` | `100ms` | espera máxima por vaga antes do 503 |
| `RETRY_AFTER_SOBRECARGA` | `5` (s) | header do 503 (+ jitter de 0–3 s) |
| `LIMITE_DESAFIO_MIN`, `LIMITE_RESPOSTA_MIN`, `LIMITE_BLOCOS_MIN`, `LIMITE_IP_MIN` | `6`, `12`, `120`, `120` | rate limit por DevId (e por IP nas rotas sem token), **global entre réplicas** via Redis |
| `REDIS_ENDERECO`, `REDIS_SENHA_ARQUIVO` | — | Redis do rate limit (spec 01). Vazio ⇒ só o limitador local por réplica (testes, execução sem Redis) |
| `REDIS_TIMEOUT` | `50ms` | por chamada; estourou ⇒ conta como falha e cai para o limitador local |
| `PROXIES_CONFIAVEIS` | `""` (o compose define `172.30.10.0/24`, sub-rede fixa da `borda`) | CIDRs cujo `X-Real-IP` é aceito |
| `PERMITIR_PERFIL_NONE` | `false` | perfil sem autenticação só em bancada |
| `LOG_NIVEL`, `LOG_FORMATO` | `info`, `json` | slog |
| `WORKER_*`, `ANCORA_JANELA_GRUPO`, `DERIVA_MAX_PPM` | ver spec 06 | worker |

```go
package config

type Config struct {
    Ambiente           string
    HTTP               HTTPConfig
    Banco              BancoConfig
    Vault              VaultConfig
    JWT                JWTConfig
    Protocolo          ProtocoloConfig   // DesafioValidade, DesafioMaxPendentes, AncoraRTTMax, LoteMaxBlocos, TLatSugerido
    Limites            LimitesConfig
    Worker             WorkerConfig
    PermitirPerfilNone bool
}
func Carregar() (Config, error)
```

### Pacote `protocolo`

**[Decisão do orquestrador] Todos os inteiros são little-endian**, como no firmware (`BlockFormat.h`: `putU16`/`putU32`). A única exceção são os **2 bytes de valor do registro de PID**: são os bytes crus `A`, `B` da resposta OBD-II, na ordem em que chegam (`BlockBuilder::addPid` grava `p[3]=value[0]`, `p[4]=value[1]`). O valor bruto do PID é `A` (1 byte) ou `256·A + B` (2 bytes). O pacote expõe `A` e `B` separadamente e nunca interpreta esse campo como `u16`.

#### Envelope TLV (unidade de transporte em todos os endpoints)

**[Decisão] O que trafega no HTTP é a mensagem TLV completa, idêntica à do SYNC_RX/SYNC_TX** (sem o byte de controle de fragmentação do SYNC_TX). Assim o app nunca monta nem corta bytes: o TLV remontado do SYNC_TX vai como está ao servidor, e o TLV que vem do servidor vai como está para o SYNC_RX. O tipo viaja junto e é validado. Um lote de blocos é a concatenação de TLVs, autodelimitada.

| Offset | Campo | Tipo | Tamanho |
|---|---|---|---|
| 0 | `T` (tipo da mensagem) | u8 | 1 |
| 1 | `L` (comprimento de `V`) | u16 LE | 2 |
| 3 | `V` (corpo) | bytes | `L` |

Comprimentos de `V` por tipo (perfil `hmac`):

| T | Mensagem | `L` válido |
|---|---|---|
| 0x01 | Desafio | 45 (`flags_c` bit 0 = 0) ou 47 (bit 0 = 1) |
| 0x02 | Resposta | 50 |
| 0x03 | Bloco | `tamanho + 32`, com `17 ≤ tamanho ≤ 1024` |
| 0x04 | ACK | 25 |

#### Desafio 0x01 (servidor → dongle), `V` com 45/47 B

| Offset | Campo | Tipo | Tamanho |
|---|---|---|---|
| 0 | `c_S` | u32 LE | 4 |
| 4 | `N_S` | bytes | 16 |
| 20 | `ACK_seq` | u32 LE | 4 |
| 24 | `c_conf` | u32 LE | 4 |
| 28 | `flags_c` | u8 (bit 0 = modo diagnóstico; 1–7 reservados = 0) | 1 |
| 29 | `diag_min` | u16 LE — **só se `flags_c` bit 0 = 1** | 2 |
| 29 ou 31 | `tag` | `HMAC-SHA256_{K_auth}(…)[0:16]` | 16 |

#### Resposta 0x02 (dongle → servidor), `V` com 50 B

| Offset | Campo | Tipo | Tamanho |
|---|---|---|---|
| 0 | `c_S` (eco) | u32 LE | 4 |
| 4 | `N_S` (eco) | bytes | 16 |
| 20 | `TAIL_seq` | u32 LE | 4 |
| 24 | `HEAD_seq` | u32 LE | 4 |
| 28 | `boot_id` | u16 LE | 2 |
| 30 | `t_ms` | u32 LE | 4 |
| 34 | `tag` | `HMAC-SHA256_{K_auth}(…)[0:16]` | 16 |

#### ACK 0x04 (servidor → dongle), `V` com 25 B

| Offset | Campo | Tipo | Tamanho |
|---|---|---|---|
| 0 | `ACK_seq` | u32 LE | 4 |
| 4 | `c_conf` | u32 LE | 4 |
| 8 | `flags_a` | u8 (bit 0 = âncora falhou, repetir handshake; 1–7 = 0) | 1 |
| 9 | `tag` | `HMAC-SHA256_{K_auth}(…)[0:16]` | 16 |

#### Bloco 0x03 (dongle → servidor), `V` com `tamanho + 32` B

| Offset | Campo | Tipo | Tamanho |
|---|---|---|---|
| 0 | `seq` | u32 LE | 4 |
| 4 | `boot_id` | u16 LE | 2 |
| 6 | `t_ms` inicial (`t0_ms`) | u32 LE | 4 |
| 10 | `tamanho` (total da área de dados, cabeçalho incluído) | u16 LE | 2 |
| 12 | registros | — | `tamanho − 12` |
| `tamanho` | `tag` | `HMAC-SHA256_{K_rec}(…)` (sem truncar) | 32 |

Registros (concatenados de 12 até `tamanho`):

| Registro | Offset rel. | Campo | Tipo | Tam. | Total |
|---|---|---|---|---|---|
| PID (tipo ∉ {0xFD,0xFE,0xFF}) | 0 | tipo = código do PID | u8 | 1 | **5** |
| | 1 | Δt (ms desde `t0_ms`, **sem sinal**) | u16 LE | 2 | |
| | 3 | A | u8 | 1 | |
| | 4 | B (0 se o PID tem 1 byte) | u8 | 1 | |
| GPS | 0 | 0xFF | u8 | 1 | **15** |
| | 1 | Δt | u16 LE | 2 | |
| | 3 | lat (graus × 10⁷) | i32 LE | 4 | |
| | 7 | lon (graus × 10⁷) | i32 LE | 4 | |
| | 11 | alt (mm) | i32 LE | 4 | |
| BOOT / LOSS | 0 | 0xFE (BOOT) / 0xFD (LOSS) | u8 | 1 | **11** |
| | 1 | Δt | u16 LE | 2 | |
| | 3 | `from_seq` | u32 LE | 4 | |
| | 7 | `to_seq` | u32 LE | 4 | |

Regras de forma (o servidor rejeita o bloco se falharem):

- Um marcador (0xFE/0xFD) é o **único** registro do bloco: `tamanho == 23`.
- Os registros consomem exatamente `tamanho − 12` bytes (sem sobra).
- Um bloco de dados tem ao menos 1 registro (`tamanho ≥ 17`; V mínimo = 17 + tag). A Tabela 3 do `.tex` diz "55 a 1.056", o que está incorreto (um bloco de 1 PID tem 49 B); a Fase 1 (spec 04 §3) usa os mesmos limites `17 + T … 1024 + T`.
- LOSS tem `boot_id = 0` e `t0_ms = 0` (cabeçalho original possivelmente corrompido). O servidor não exige isso (só registra), porque a tag cobre o conteúdo.

#### Entrada do HMAC

```
entrada = "MALCONG" (7 bytes ASCII, sem terminador)
       || tipo     (1 byte: 0x01, 0x02, 0x03 ou 0x04)
       || DevId    (8 bytes crus)
       || payload
```

| Tipo | `payload` | Tamanho da entrada | Chave | Tag |
|---|---|---|---|---|
| 0x01 | `V[0:29]` ou `V[0:31]` (tudo antes da tag) | 45 ou 47 | K_auth | 16 primeiros bytes |
| 0x02 | `V[0:34]` | 50 | K_auth | 16 primeiros bytes |
| 0x04 | `V[0:9]` | 25 | K_auth | 16 primeiros bytes |
| 0x03 | área de dados `V[0:tamanho]` (inclui o cabeçalho com `seq`) | 16 + `tamanho` | K_rec | 32 bytes |

A comparação é sempre em tempo constante (`crypto/subtle.ConstantTimeCompare`) sobre o tamanho fixo da tag.

#### Perfis (benchmark da Fase 1)

```go
// Perfil de segurança do dongle: define só o tamanho da tag do BLOCO no fio.
// A tag do handshake (0x01, 0x02, 0x04) é SEMPRE HMAC K_auth de 16 B, em
// todos os perfis (reconciliado com a Fase 1, spec 01 §1).
type Perfil struct {
    Nome         string // "hmac" | "asym" | "none"
    TamTagBloco  int    // 0x03: 32 (hmac) | 64 (asym: ECDSA P-256 determinística, r‖s) | 0 (none)
}
const TamTagAuth = 16   // 0x01 (45/47 B), 0x02 (50 B), 0x04 (25 B): fixos
var (
    PerfilHMAC = Perfil{"hmac", 32}
    PerfilAsym = Perfil{"asym", 64} // depois do caminho HMAC estável (spec 07)
    PerfilNone = Perfil{"none", 0}  // só com PERMITIR_PERFIL_NONE=true
)
```

Os decodificadores de 0x01/0x02/0x04 não dependem do perfil (comprimentos fixos). Só `DecodificarBloco` recebe o `Perfil` do dongle (vem do cadastro) para validar `L == tamanho + TamTagBloco`.

**[Decisão, reconciliação com a Fase 1]** O perfil `none` do firmware (`NoAuth`) zera apenas a tag do bloco; o desafio continua sendo verificado e a resposta assinada com `K_auth` (o custo do handshake não faz parte do benchmark de blocos). O perfil `asym` troca a tag do bloco por ECDSA P-256 determinística (RFC 6979) com `r‖s` de 64 B, porque o mbedTLS 2.28 do Arduino 2.0.17 não tem Ed25519.

#### API do pacote

```go
package protocolo

const Rotulo = "MALCONG"

type Tipo uint8
const (
    TipoDesafio  Tipo = 0x01
    TipoResposta Tipo = 0x02
    TipoBloco    Tipo = 0x03
    TipoAck      Tipo = 0x04
)

type DevID [8]byte
func DevIDDeHex(s string) (DevID, error) // exatamente 16 dígitos hex minúsculos ou maiúsculos
func (d DevID) Hex() string              // minúsculo

// TLV
type TLV struct{ T Tipo; V []byte }       // V aponta para o buffer original (sem cópia)
func LerTLV(b []byte) (TLV, int, error)   // devolve bytes consumidos
func LerLote(corpo []byte, max int) ([]TLV, error) // concatenação; ErrLoteGrande se > max
func AnexarTLV(dst []byte, t Tipo, v []byte) []byte

// Entrada do HMAC
func EntradaTag(t Tipo, dev DevID, payload []byte) []byte
func CompararTag(calculada, recebida []byte) bool // tempo constante; tamanhos diferentes = false

// 0x01
type Desafio struct {
    CS, AckSeq, CConf uint32
    NS               [16]byte
    Flags            uint8
    DiagMin          uint16 // só se Flags&1
}
func (d Desafio) Payload() []byte                     // V sem a tag
func (d Desafio) Montar(tag []byte) ([]byte, error)   // TLV completo (T|L|V)
func DecodificarDesafio(v []byte) (Desafio, []byte /*tag*/, error) // usado pelo simulador; L fixo (45/47)

// 0x02
type Resposta struct {
    CS, TailSeq, HeadSeq, TMs uint32
    NS      [16]byte
    BootID  uint16
}
func DecodificarResposta(v []byte) (r Resposta, payload, tag []byte, err error) // L fixo (50)
func (r Resposta) Montar(tag []byte) []byte            // simulador

// 0x04
type Ack struct{ AckSeq, CConf uint32; Flags uint8 }
const FlagAckAncoraFalhou uint8 = 1 << 0
func (a Ack) Payload() []byte
func (a Ack) Montar(tag []byte) []byte
func DecodificarAck(v []byte) (Ack, []byte, error) // L fixo (25)

// 0x03
type CabecalhoBloco struct{ Seq uint32; BootID uint16; T0Ms uint32; Tamanho uint16 }
type Bloco struct {
    Cab   CabecalhoBloco
    Dados []byte // V[0:Tamanho] — entra no HMAC e em bloco_bruto.payload
    Tag   []byte // V[Tamanho:]
}
func DecodificarBloco(v []byte, p Perfil) (Bloco, error) // valida L == Tamanho + TamTagBloco

type TipoRegistro uint8
const (RegLoss TipoRegistro = 0xFD; RegBoot TipoRegistro = 0xFE; RegGPS TipoRegistro = 0xFF)
type RegistroPID struct{ PID uint8; Dt uint16; A, B uint8 }
type RegistroGPS struct{ Dt uint16; LatE7, LonE7, AltMm int32 }
type RegistroMarcador struct{ Tipo TipoRegistro; Dt uint16; FromSeq, ToSeq uint32 }

// Conteudo classifica o bloco depois de percorrer os registros.
type Conteudo struct {
    Marcador *RegistroMarcador // não nil ⇒ bloco BOOT/LOSS (único registro)
    PIDs     []RegistroPID
    GPS      []RegistroGPS
    DtMax    uint16            // para t_fim do bloco
    DtMin    uint16
}
func LerRegistros(dados []byte) (Conteudo, error) // ErrRegistroMalformado, ErrMarcadorComOutros

// Builders usados pelo simulador e pelos testes
type MontadorBloco struct{ /* ... */ }
func NovoMontadorBloco(seq uint32, bootID uint16, t0Ms uint32) *MontadorBloco
func (m *MontadorBloco) PID(pid uint8, dt uint16, a, b uint8) bool
func (m *MontadorBloco) GPS(dt uint16, lat, lon, alt int32) bool
func (m *MontadorBloco) Dados() []byte                     // fecha: escreve tamanho
func MontarMarcador(seq uint32, bootID uint16, t0 uint32, tipo TipoRegistro, from, to uint32) []byte
func MontarBlocoTLV(dados, tag []byte) []byte
```

Erros exportados (`erros.go`): `ErrTLVCurto`, `ErrTLVComprimento`, `ErrTipoInesperado`, `ErrLoteVazio`, `ErrLoteGrande`, `ErrTamanhoInvalido`, `ErrRegistroMalformado`, `ErrMarcadorComOutros`, `ErrFlagsReservadas`. A camada HTTP os mapeia para `400` (forma) ou para rejeição de bloco (conteúdo; spec 05).

Desempenho: `LerLote` e `DecodificarBloco` não copiam (fatias do corpo). `LerRegistros` aloca as fatias com a capacidade estimada (`(tamanho−12)/5`). Meta: < 5 µs por bloco de 1 KB (benchmark `BenchmarkLerRegistros`).

#### Vetores de teste

- **[Decisão] Vetores de referência gerados por uma implementação independente em Python** (`backend/scripts/gerar_vetores.py`, só stdlib: `hmac`, `hashlib`, `struct` com `'<'`) e gravados em `internal/protocolo/testdata/vetores.json`. Uma segunda implementação escrita a partir desta spec pega erros de interpretação que um teste "Go contra Go" não pegaria.
- **Contrato cruzado com a Fase 1 (reconciliado):** a Fase 1 versiona o gerador `esp32-firmware/tools/provision/vectors.py` e o arquivo `vectors.json` (chaves fixas de bancada, `DevId = 544343454C544101` = `"TCCELTA\x01"`), consumidos pelos testes do firmware e do app (Fase 1, spec 01 §12). Esse mesmo `vectors.json` é copiado para `testdata/vetores_firmware.json` (um conversor de formato em `scripts/`, se os esquemas JSON diferirem) e verificado por `TestVetoresFirmware` (pulado com `t.Skip` enquanto o arquivo não existir). Os vetores Python desta fase (`gerar_vetores.py`, DevId `0011223344556677`) continuam como segunda implementação independente; divergência entre os dois conjuntos é bug de um dos lados.

Formato de `vetores.json`:

```json
{
  "versao": 1,
  "dev_id": "0011223344556677",
  "k_auth": "<64 hex>",
  "k_rec":  "<64 hex>",
  "casos": [
    { "nome": "desafio_sem_diag", "tipo": 1,
      "campos": { "c_s": 7, "n_s": "<32 hex>", "ack_seq": 120, "c_conf": 6, "flags": 0 },
      "entrada_hmac": "<hex>", "hmac_completo": "<64 hex>", "tlv": "<hex>" },
    { "nome": "bloco_dados_2pids_1gps", "tipo": 3,
      "campos": { "seq": 121, "boot_id": 3, "t0_ms": 600000,
                  "registros": [ {"pid": 12, "dt": 0, "a": 23, "b": 112},
                                 {"gps": true, "dt": -150, "lat": -235505200, "lon": -466333100, "alt": 760000},
                                 {"pid": 13, "dt": 40, "a": 62, "b": 0} ] },
      "dados": "<hex>", "tag": "<64 hex>", "tlv": "<hex>" },
    { "nome": "bloco_boot", "tipo": 3, "campos": { "seq": 192, "boot_id": 4, "t0_ms": 0,
      "marcador": {"tipo": 254, "dt": 0, "from_seq": 130, "to_seq": 191} }, "...": "..." },
    { "nome": "bloco_loss", "...": "..." },
    { "nome": "resposta", "...": "..." },
    { "nome": "ack_flag_ancora", "...": "..." },
    { "nome": "desafio_com_diag", "...": "..." }
  ]
}
```

### Banco: pgx + sqlc

`backend/sqlc.yaml`:

```yaml
version: "2"
sql:
  - engine: postgresql
    schema: internal/banco/migrations
    queries: internal/banco/queries
    gen:
      go:
        package: consultas
        out: internal/banco/consultas
        sql_package: pgx/v5
        emit_interface: true          # Querier, para dublês nos testes
        emit_empty_slices: true
        emit_pointers_for_null_types: true
        query_parameter_limit: 4
        overrides:
          - db_type: uuid
            go_type: github.com/google/uuid.UUID
          - db_type: uuid
            nullable: true
            go_type: { import: github.com/google/uuid, type: UUID, pointer: true }
          - db_type: timestamptz
            go_type: time.Time
          - db_type: timestamptz
            nullable: true
            go_type: { type: time.Time, pointer: true }
          - db_type: geography
            go_type: tccelta/backend/internal/banco/geo.PontoZ
```

`internal/banco/pool.go`:

```go
// NovoPool cria o pool com o codec de geography registrado em cada conexão.
func NovoPool(ctx context.Context, cfg config.BancoConfig, log *slog.Logger) (*pgxpool.Pool, error)
// AfterConnect: consulta o OID de "geography" e registra pgtype.Type{Name: "geography", OID: oid, Codec: geo.CodecEWKB{}}.
```

`internal/banco/geo.go`: `type PontoZ struct{ LonGraus, LatGraus, AltM float64 }`. O codec registrado para o OID de `geography` **precisa implementar `PlanEncode` no formato binário**, porque o `CopyFrom` do pgx só usa binário. Sem isso, a falha aparece em tempo de execução, não de geração; o teste de integração de EWKB faz um `CopyFrom` real. Codificação binária EWKB (little-endian, tipo `0xA0000001` = PointZ + flag SRID, SRID 4326, 3×float64 = 33 bytes). **[Decisão] COPY binário com EWKB**, sem tabela temporária e sem chamar funções PostGIS no SQL (o sqlc não conhece `ST_MakePoint`). *Plano B* se o COPY de `geography` falhar no checkpoint: `posicao` ganha colunas `lat_e7 int, lon_e7 int, alt_mm int`, e `geom` vira coluna gerada `GENERATED ALWAYS AS (ST_SetSRID(ST_MakePoint(lon_e7/1e7, lat_e7/1e7, alt_mm/1000.0), 4326)::geography) STORED`.

`internal/banco/tx.go`:

```go
type OpcoesTx struct{ LockTimeout, StatementTimeout time.Duration }

// EmTransacao abre transação READ COMMITTED, aplica SET LOCAL de timeouts,
// executa fn com *consultas.Queries ligado à tx e faz commit/rollback.
// Erros de lock_timeout (SQLSTATE 55P03) viram ErrBancoOcupado (→ 503).
// Deadlock (40P01) e falha de serialização (40001): UMA retentativa automática de fn
// inteira (fn precisa ser reexecutável: nada de efeito externo irreversível dentro dela
// além de chamadas idempotentes ao KMS); na segunda falha, ErrBancoOcupado (→ 503).
func EmTransacao(ctx context.Context, pool *pgxpool.Pool, o OpcoesTx,
    fn func(ctx context.Context, q *consultas.Queries, tx pgx.Tx) error) error
```

Nota sobre `unnest` com vários arrays (specs 05): algumas versões do sqlc inferem mal os tipos das colunas de `unnest(@a, @b, …) AS u(…)`. Se o Checkpoint 3 falhar, troque por `ROWS FROM (unnest(@a::bigint[]), unnest(@b::int[]), …) AS u(…)`, que tem tipos explícitos.

Arquivos de queries (detalhados nas specs indicadas): `dongle.sql`, `pid.sql` (02); `desafio.sql`, `estado.sql`, `contexto.sql` (04); `blocos.sql` (05); `worker.sql` (06).

### HTTP

#### Rotas (contratos completos nas specs 04, 05, 08)

| Método e caminho | Autenticação | Request | Response 2xx |
|---|---|---|---|
| `POST /v1/dongles/{id}/desafio` | nenhuma (rate limit DevId + IP) | corpo vazio | `200 application/json` `{"desafio":"<b64 TLV 0x01>","expira_em":"<RFC3339>"}` |
| `POST /v1/dongles/{id}/desafio/resposta` | nenhuma (rate limit) | `application/json` `{"resposta":"<b64 TLV 0x02>"}` | `200 application/json` `{"ack":"<b64 TLV 0x04>","token":"<JWT>","token_expira_em":"<RFC3339>","token_validade_s":3600,"t_lat_s":60}` |
| `POST /v1/dongles/blocos` | `Authorization: Bearer <JWT>` | `application/octet-stream` (1–32 TLVs 0x03 concatenados) | `200 application/octet-stream` (TLV 0x04) + headers `X-Lote-*` |
| `GET /v1/dongles/{id}/confirmacao` | `Bearer` com `sub = {id}` | — | `200 application/octet-stream` (TLV 0x04) |
| `GET /saude/viva` | — | — | `200` (processo vivo) |
| `GET /saude/pronta` | — | — | `200` se banco ok, token Vault válido, PIDs carregados e chave pública JWT carregada; senão `503` |

`{id}` = DevId em 16 dígitos hexadecimais. Base64: **padrão RFC 4648 §4 com padding** (`base64.StdEncoding`). A API rejeita variantes URL-safe ou sem padding com 400.

**[Decisão] Contrato de corpos.** Os handshakes usam JSON, com os bytes binários em base64. O caminho quente (blocos e confirmação) usa `octet-stream` puro.

- A resposta do handshake carrega duas coisas heterogêneas: o ACK binário assinado e o JWT, mais metadados (`token_expira_em`, `t_lat_s` sugerido, que o doc prevê como opcional). Um envelope JSON é extensível e evita *headers* proprietários. O pedido de desafio e o envio da resposta seguem o mesmo estilo, por simetria.
- Base64 é uma codificação reversível dos **mesmos bytes**: o app não reinterpreta nem altera nada, e a tag protege a integridade de qualquer forma. A objeção do doc ("converter para JSON abriria espaço para alterações e custaria CPU") vale para o caminho de alto volume. Por isso, blocos e ACKs de lote seguem em `application/octet-stream`, sem conversão, exatamente como o doc pede.
- O handshake é raro (por conexão ou a cada ~1 h), então o custo do base64 é desprezível.

#### Erros: `application/problem+json` (RFC 9457)

```json
{ "type": "https://tccelta.local/erros/desafio_invalido", "title": "Desafio desconhecido, vencido ou já usado",
  "status": 409, "codigo": "desafio_invalido", "detail": "..." }
```

| Status | `codigo` | Onde | Ação do app (Tabela 5) |
|---|---|---|---|
| 400 | `corpo_malformado`, `base64_invalido`, `tlv_invalido`, `lote_malformado`, `devid_invalido` | todos | bug do app: registrar; no lote, descartar o buffer acima do ACK e reler (erro 0x07). Se o 400 persistir com os mesmos bytes relidos, o bloco do dongle é malformado: alerta; sem saída manual no servidor, corrige-se o firmware/chave do dongle (spec 05) |
| 401 | `token_ausente`, `token_invalido`, `token_expirado` | blocos, confirmação | novo handshake e reenvio |
| 403 | `token_outro_dongle` | confirmação | descartar o token e fazer handshake |
| 403 | `dongle_inativo` | todos | parar e avisar o usuário (não repetir) |
| 404 | `dongle_desconhecido` | desafio, resposta | parar e avisar o usuário |
| 404 | `sem_confirmacao` | confirmação | fazer handshake |
| 409 | `desafio_invalido`, `c_s_antigo` | resposta | pedir novo desafio |
| 413 | `corpo_grande` | todos | bug: lote acima de 32 × 1059 B |
| 415 | `tipo_conteudo` | todos | bug |
| 422 | `tag_invalida` | resposta | pedir novo desafio (com backoff) |
| 422 | `resposta_inconsistente` | resposta | novo handshake (com backoff) |
| 422 | `lote_sem_progresso` (corpo = ACK em `application/octet-stream`, **não** problem+json) | blocos | ver spec 05; regra geral: o corpo é um ACK ⇔ `Content-Type: application/octet-stream` |
| 429 | `limite_taxa`, `lote_grande` | todos | esperar `Retry-After`; K pela metade |
| 503 | `sobrecarga`, `kms_indisponivel`, `banco_ocupado` | todos | manter o lote no buffer; backoff exponencial 1 s–5 min; K pela metade |

`Retry-After` em segundos inteiros está sempre presente em 429 e 503.

#### Cadeia de middlewares (de fora para dentro)

```
recuperar(panic→500) → idRequisicao(X-Request-Id) → logAcesso(slog) → metricasHTTP
  → [rotas de handshake]  anotarS2 (só /resposta) → limitarIP → limitarDevId → handler
  → [rota de blocos]      limitarIP → autenticarJWT(401/403) → limitarDevId(429) → limitarConcorrencia(503) → limitarCorpo → handler
```

**[Decisão, revisão] Na rota de blocos, o JWT é validado antes do semáforo.** A validação é local (cerca de 50 µs). Se o semáforo viesse primeiro, uma enxurrada de requisições sem token ocuparia todas as vagas e geraria 503 para apps legítimos.

- `anotarS2` é o **primeiro** código específico da rota e roda antes de ler o corpo: `s2 := time.Now()` guardado no `context`. Os middlewares genéricos antes dele são O(µs) e não fazem I/O.
- `limitarCorpo`: `http.MaxBytesReader` com `LOTE_MAX_BLOCOS × 1059 + 64` B no lote e `2 KiB` nos JSON.
- `limitarConcorrencia`: `semaphore.Weighted(INGESTAO_MAX_SIMULTANEAS)` com `TryAcquire` + espera até `INGESTAO_ESPERA_VAGA`. Se não houver vaga, devolve `503` com `Retry-After = RETRY_AFTER_SOBRECARGA + jitter`.
- `limitarDevId`/`limitarIP` (`internal/limites`): **[Decisão] limitador distribuído em Redis** (escolha do autor, no lugar do limite só por réplica). Duas camadas, na ordem:
  1. **local, por réplica:** `rate.Limiter` por chave num `expirable.LRU` (100 mil chaves, TTL 10 min), sem rede — corta rajadas óbvias sem custar uma ida ao Redis;
  2. **global, no Redis:** contador de janela fixa de 1 minuto por escopo e chave, atômico por script Lua:
     ```lua
     -- KEYS[1] = "tccelta:rl:<escopo>:<chave>:<minuto_unix>", ARGV[1] = ttl da janela em ms (70 000)
     local n = redis.call('INCR', KEYS[1])
     if n == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
     return n
     ```
     `n > LIMITE_*_MIN` ⇒ 429 `limite_taxa` com `Retry-After` = segundos até o fim do minuto. O limite passa a ser **global** entre as réplicas (não mais até N× maior).
  Interface: `limites.Limitador.Permitir(ctx, escopo, chave string, limiteMin int) (ok bool, retryAfter time.Duration)`; implementações `LimitadorLocal`, `LimitadorRedis` e `LimitadorComFallback` (as duas camadas). **Falha do Redis nunca vira 503:** com erro ou `REDIS_TIMEOUT` estourado, a requisição vale só pelo limite local, o evento incrementa `tccelta_limite_redis_falhas_total` e, após 5 falhas seguidas, um *circuit breaker* pula o Redis por 10 s. O Redis só guarda contadores efêmeros (sem persistência; perdê-lo zera as janelas).
- IP do cliente: `X-Real-IP` só se `RemoteAddr` ∈ `PROXIES_CONFIAVEIS`; senão, `RemoteAddr`.

#### Servidor HTTP e desligamento gracioso

```go
srv := &http.Server{
    Addr: cfg.HTTP.Endereco, Handler: rotas,
    ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second,
    WriteTimeout: 30 * time.Second, IdleTimeout: 120 * time.Second,
    MaxHeaderBytes: 16 << 10,
}
```

Ao receber SIGTERM/SIGINT (`signal.NotifyContext`):

1. Marcar "não pronto" (`/saude/pronta` → 503), para o Caddy tirar a réplica do balanceamento.
2. Esperar 2 s.
3. `srv.Shutdown(ctx)` com prazo de 25 s (requisições em voo terminam; transações em andamento fazem commit ou rollback).
4. Parar a renovação do token do Vault.
5. `pool.Close()`.

O `stop_grace_period` do compose é de 30 s.

### Logging (`log/slog`)

- `slog.NewJSONHandler` (ou texto em dev), nível por `LOG_NIVEL`.
- Atributos padrão: `req_id`, `rota`, `dongle` (DevId hex), `c_s`, `seq_ini`, `seq_fim`, `ack_seq`, `duracao_ms`, `resultado`.
- **Nunca** logar tags completas, chaves, tokens JWT ou o `secret_id`. Um helper `slogx.Segredo(...)` mascara esses valores. Prefixos de 4 bytes da tag são permitidos em `debug`.
- Um evento estruturado por decisão de protocolo (`handshake.aceito`, `handshake.sem_ancora`, `lote.interrompido`, `lacuna.registrada`, `bloco.rejeitado`), no mesmo espírito dos logs de eventos do firmware (`docs/debug-events.md`).

### Métricas (`internal/metricas`, exportadas em `METRICAS_ENDERECO/metricas`)

| Métrica | Tipo | Rótulos |
|---|---|---|
| `tccelta_http_requisicoes_total` | counter | `rota`, `status` |
| `tccelta_http_duracao_segundos` | histogram (5 ms…10 s) | `rota` |
| `tccelta_blocos_total` | counter | `resultado` = `aceito`/`duplicado`/`tag_invalida`/`pid_desconhecido`/`marcador_invalido`/`malformado`/`descartado_apos_interrupcao` |
| `tccelta_lotes_total` | counter | `status` = `completo`/`interrompido`/`sem_progresso` |
| `tccelta_ingestao_tx_duracao_segundos` | histogram | — |
| `tccelta_linhas_inseridas_total` | counter | `tabela` |
| `tccelta_handshakes_total` | counter | `resultado` |
| `tccelta_ancora_rtt_segundos` | histogram (50 ms…10 s) | — |
| `tccelta_lacunas_total` | counter | `causa` |
| `tccelta_kms_chamadas_total` / `tccelta_kms_duracao_segundos` | counter / histogram | `operacao` = `hmac_auth`/`verify_rec`/`sign_jwt`, `resultado`, `fase` = `pre` (fora da transação) / `tx` (dentro) |
| `tccelta_rejeicoes_total` | counter | `motivo` = `sobrecarga`/`limite_taxa`/`kms`/`banco_ocupado` |
| `tccelta_limite_redis_falhas_total` | counter | erros/timeouts do Redis no rate limit (a API cai para o limite local) |
| `tccelta_pool_*` | gauge | coletor sobre `pgxpool.Stat()` |
| `tccelta_worker_*` | ver spec 06 | |

## Plano arquivo por arquivo

| Arquivo | Conteúdo |
|---|---|
| `cmd/tccelta/main.go` | `switch os.Args[1]`; `flag.FlagSet` por subcomando |
| `cmd/tccelta/api.go` | monta config, log, pool, KMS, emissor/validador JWT, caches, serviços e `httpapi.NovoRoteador`; roda o servidor HTTP e o de métricas; desligamento gracioso |
| `cmd/tccelta/saude.go` | `GET --url`, sai com 0/1 (healthcheck da distroless) |
| `internal/config/config.go` (+`_test`) | `Carregar`, leitura de `*_ARQUIVO` |
| `internal/protocolo/*.go` (+testes, fuzz) | formatos acima |
| `internal/banco/pool.go`, `tx.go`, `geo.go` (+testes) | infraestrutura de banco |
| `internal/dongles/cache.go` | `Buscar(ctx, DevID) (Dongle, error)`; LRU 10 mil entradas, TTL 60 s; `ErrDesconhecido` |
| `internal/pids/catalogo.go` | `Catalogo.Obter(pid) (Def, bool)`; `Recarregar(ctx)`; goroutine com `LISTEN pid_alterado` + recarga a cada 5 min |
| `internal/httpapi/rotas.go` | `NovoRoteador(deps) http.Handler` |
| `internal/httpapi/problema.go` | `escreverProblema(w, status, codigo, detalhe, retryAfter)` |
| `internal/httpapi/middlewares.go` | middlewares acima |
| `internal/httpapi/handshake.go`, `blocos.go`, `confirmacao.go`, `saude.go` | handlers finos (specs 04, 05, 08) |
| `internal/limites/taxa.go`, `redis.go`, `concorrencia.go` | limitadores (local, Redis e com *fallback*) |
| `internal/metricas/metricas.go` | registradores |
| `scripts/gerar_vetores.py` | implementação de referência em Python |
| `sqlc.yaml`, `.golangci.yml` | configuração (`errcheck`, `gosec`, `bodyclose`, `sqlclosecheck`, `contextcheck`) |

## Testes

- `protocolo`:
  - ida e volta de cada mensagem;
  - todos os casos de `vetores.json` (bytes e HMAC iguais);
  - tamanhos inválidos (44, 46, 48, 49, 51 B…);
  - `flags_c` com bits reservados → `ErrFlagsReservadas`;
  - marcador junto com PID → erro;
  - sobra de bytes → erro;
  - Δt = 0 e Δt = 65 535 (limites do `u16`, sem sinal);
  - `tamanho` 1024 (limite);
  - fuzz `FuzzLerLote` e `FuzzLerRegistros` (sem panics, sem leitura fora dos limites).
- `config`: falta de obrigatórias, `*_ARQUIVO`, `producao` + `PERMITIR_PERFIL_NONE` = erro.
- `httpapi` (com `httptest` e dublês de serviço): mapeamento de cada erro de serviço para status/código/`Retry-After`; `anotarS2` antes da leitura do corpo (o teste usa um corpo que bloqueia e confere que S2 < fim da leitura); 503 quando o semáforo está cheio.
- `limites`: *burst* e reabastecimento com relógio injetado; limitador Redis com `miniredis`: o contador sobe a cada chamada, a janela nova zera, **duas instâncias do limitador compartilham a contagem** (réplica A e B somam no mesmo limite) e o `Retry-After` é o resto do minuto; Redis derrubado (`miniredis.Close()`) ⇒ `LimitadorComFallback` segue só com o limite local, conta a falha e abre o *circuit breaker* após 5 falhas.
- `banco/geo`: EWKB de um ponto conhecido comparado com `ST_AsEWKB` (integração).

## Passos ordenados com checkpoints

1. `go.mod`, `cmd/tccelta` com `saude`/`api` vazios, `config`. **Checkpoint:** a imagem sobe e `/saude/viva` dá 200.
2. Pacote `protocolo` + `gerar_vetores.py` + testes. **Checkpoint:** `make vetores && make teste` passa, com cobertura do pacote ≥ 90 %.
3. `sqlc.yaml` + queries de cadastro + `banco/pool.go`/`tx.go`/`geo.go`. **Checkpoint:** `make sqlc` e o teste de integração de EWKB passam.
4. `httpapi` (rotas com handlers stub), middlewares, problem+json, métricas, desligamento gracioso. **Checkpoint:** `curl -X POST /v1/dongles/zz/desafio` devolve 400 `devid_invalido`; `/metricas` expõe as séries; `docker compose stop api` termina sem erro dentro do prazo.
5. `dongles` e `pids` (cache e LISTEN). **Checkpoint:** um `INSERT` em `pid` aparece no log `pids.recarregado` em menos de 1 s.

## Critérios de aceite

- [ ] O pacote `protocolo` não tem dependências fora da stdlib e passa nos vetores Python. O teste com vetores do firmware está pronto para rodar quando a Fase 1 os entregar.
- [ ] Nenhuma string SQL no código Go fora de `banco/tx.go` e `banco/escuta.go` (verificado com `grep -R "SELECT\|INSERT\|UPDATE" --include=*.go internal | grep -v consultas/`).
- [ ] `golangci-lint` limpo; `go test -race ./...` verde.
- [ ] Os contratos HTTP desta tabela são os mesmos usados pela spec 09 (app).

## Riscos

| Risco | Mitigação |
|---|---|
| Divergência de interpretação do formato com a Fase 1 (ex.: `L` big-endian, `tamanho` sem o cabeçalho) | vetores cruzados obrigatórios; tabela de contratos no README |
| EWKB via COPY não aceito | plano B (coluna gerada) |
| Rate limit distribuído depende do Redis | cai para o limite local por réplica se ele falhar (nunca 503); contadores efêmeros, sem persistência |
| `ServeMux` sem algum recurso | troca local por chi; os handlers não mudam |

## Decisões do autor (antes em aberto)

- **[Decisão] Rate limit distribuído em Redis** (descartados: limite só por réplica e a tabela `limite_janela` no Postgres). Serviço `redis` na stack (spec 01), com senha e sem persistência; a API cai para o limite local se ele falhar.
- **[Decisão] HTTP/3 não habilitado no Caddy** (`protocols h1 h2`): o app usa `dio`/HTTP 1.1–2, exigiria porta UDP e não traria ganho.
