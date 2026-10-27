# 10 — Simulador de dongles, testes de integração e de carga

> Fase 3 · Servidor. Depende de: 03 (`protocolo`), 04/05/06 (endpoints e worker), 07 (chaves de teste no Vault). Fecha a fase: produz as evidências do README e os números que substituem as estimativas do `server-specification.md` ("Esses números substituem as estimativas da tabela acima no texto final do TCC").

## Objetivo

1. **Simulador em Go** (`tccelta simulador`) de dongles virtuais que geram blocos assinados com as **mesmas chaves importadas no Vault de teste**, incluindo BOOT, LOSS, OVERFLOW, reconexões com acúmulo offline e handshakes com `t_ms` e deriva realistas. Cada dongle virtual também faz o papel do app (HTTP, buffer, `T_lat`, K adaptativo, retentativas).
2. **Testes de integração** com testcontainers (por pacote) e **testes ponta a ponta** com o compose (stack inteira).
3. **Testes de carga** com metas explícitas e relatório — **documentados, mas não executados nesta fase** (decisão do autor: não fazer testes de carga agora, apenas documentar). O simulador, as metas M1–M6, os cenários `carga`/`reconexao`/`joelho` e o formato do relatório ficam especificados e implementados como código; **medir e versionar o relatório** fica para depois.

## Escopo

Dentro: pacote `internal/simulador`, subcomando, cenários, infraestrutura de teste (`internal/testeinfra`), `compose.test.yaml`, metas e formato do relatório (documentados; execução da carga adiada), CI do backend. Fora: testes do app Flutter (spec 09) e do firmware (Fase 1).

## Design

### Dongle virtual (`internal/simulador/dongle.go`)

Ele reproduz o comportamento do firmware descrito no `.tex` §4–5, usando o **mesmo** pacote `protocolo` da API (como o doc pede: "O mesmo pacote alimenta o simulador de carga").

```go
type Chaves struct{ Dev protocolo.DevID; KAuth, KRec [32]byte }

type ConfigDongle struct {
    BlocosPorSegundo   float64       // 0,10–0,17 (Investigação-3 §2.6.6)
    RegistrosPorBloco  int           // ~200 (fecha ao encher 1024 B ou após 10 s)
    GPSPeriodo         time.Duration // 1 s
    DerivaPPM          float64       // deriva do cristal simulado (ex.: +25)
    CapacidadeFlash    int           // blocos; pequena para forçar OVERFLOW
    ReservaSeq         uint32        // 64 (seq + 64 na NVS a cada 64 blocos)
    Relogio            Relogio       // real ou acelerado (fator)
}

type DongleVirtual struct {
    ch          Chaves
    cfg         ConfigDongle
    // "NVS"
    bootID      uint16
    reservaNVS  uint32
    ultimoCS    uint32
    // "RAM"
    seq         uint32
    inicioBoot  time.Time        // t_ms = (agora − inicioBoot)·(1 + deriva)
    ackLocal    uint32
    sessaoOK    bool             // flag "sem handshake nesta conexão"
    // "flash"
    flash       *FilaFlash       // blocos com tag e CRC simulado; descarta o mais antigo quando cheia
}

// Lado dongle do protocolo
func (d *DongleVirtual) ProcessarDesafio(tlv []byte) (respostaTLV []byte, err error) // verifica tag K_auth(16), c_S > ultimoCS, libera flash até ACK_seq
func (d *DongleVirtual) ProcessarAck(tlv []byte) error                                // verifica tag, avança ackLocal, libera flash
func (d *DongleVirtual) GerarBlocosAte(t time.Time)                                    // produz blocos PID/GPS assinados com K_rec
func (d *DongleVirtual) LerParaEnvio(k int) [][]byte                                   // TLVs 0x03; troca bloco corrompido por LOSS
// Eventos
func (d *DongleVirtual) PerderEnergia()   // RAM perdida; no religar: seq = reserva; bloco BOOT [último na flash+1, R−1]; boot_id++
func (d *DongleVirtual) CorromperBloco(seq uint32)  // CRC falha ⇒ LOSS no envio
func (d *DongleVirtual) NovaConexaoBLE()  // sessaoOK = false
```

Valores dos PIDs: uma pequena máquina de estados de condução (parado → acelera → cruzeiro → freia), coerente entre RPM (0x0C), velocidade (0x0D), carga (0x04) e pedal (0x49). Temperatura (0x05) e combustível (0x2F) variam devagar. Os períodos por PID seguem a Tabela 1 da Investigação-3. O GPS anda numa trajetória sintética em São Paulo (lat ≈ −23,55°).

### App virtual (`internal/simulador/app.go`)

É a reimplementação em Go do comportamento da spec 09: mesmo contrato HTTP, buffer de até `B_max = 64`, envio a cada `T_lat` ou com 32 blocos, renovação do token com menos de 5 min, `X-Lote-Acao`, 429/503 com `Retry-After`, K adaptativo e backoff de 1 s–5 min. É ele que exercita a API exatamente como o app real exercitará.

```go
type AppVirtual struct {
    http     *http.Client   // keep-alive; TLS opcional (--ca)
    base     string
    dongle   *DongleVirtual
    buffer   [][]byte
    token    string; tokenExpira time.Time
    k        int
    malicioso ModoMalicioso // nenhum | trocarByte | reenviarAntigos | foraDeOrdem | tokenDeOutro | omitirAck
    met      *MetricasSim
}
func (a *AppVirtual) Executar(ctx context.Context) error
```

### Subcomando e cenários

```
tccelta simulador --cenario <nome> --api http://api:8080 [--ca /path/ca.pem]
                  --chaves /testdata/dongles-teste.csv | --provisionar N
                  [--dongles N] [--duracao 10m] [--acelerar 1] [--tlat 60s]
                  [--acumulado 0] [--rampa 30s] [--banco-url-env SIM_BANCO_URL] [--relatorio /saida/relatorio.json]
```

`--provisionar N` cria N dongles via `internal/provisionamento` (com o token do provisionador, só fora de produção) e guarda as chaves em memória: é assim que a carga obtém mil dongles com chaves no Vault de teste. `--chaves` usa os 3 dongles fixos.

| Cenário | O que faz | Verificações (só `SELECT`; em dev/teste o simulador conecta como `tccelta_dono` via `--banco-url`) |
|---|---|---|
| `e2e` | 3 dongles, cada um com: handshake → 200 blocos → perda de energia (BOOT) → 50 blocos → bloco corrompido (LOSS) → flash pequena + 5 min offline (OVERFLOW no handshake) → reconexão → drenagem. O dongle 3 também faz **queda de energia seguida de OVERFLOW que engole o segmento antes do BOOT** (caso 16 da spec 05) e um **BOOT corrompido** (LOSS `[ACK+1, seq]`, caso 18) | `seq_contiguo = HEAD` de cada dongle; `lacuna` com BOOT, LOSS e OVERFLOW nas faixas exatas; `count(leitura)` = registros gerados − perdidos; `boot.a_ms` não nulo nos boots com handshake e `origem_tempo='vizinhos'` no boot sem handshake; `ancora_tempo` ≥ 1 por boot com handshake; `bloco_rejeitado` vazio |
| `protocolo-negativo` | app malicioso: troca 1 byte (⇒ `reler`, rejeitado); reenvia lotes antigos (⇒ duplicados, 0 linhas novas); fora de ordem (⇒ `salto`); token de outro DevId na confirmação (⇒ 403); resposta reapresentada (⇒ 409); 33 blocos (⇒ 429); desafios em rajada (⇒ 429 `desafios_pendentes`) | status e headers esperados; o estado final é igual ao de um app honesto, porque nenhum ataque altera dados aceitos |
| `tempo` | 1 dongle com `DerivaPPM=+25`, `--acelerar 60` (6 h simuladas em 6 min), handshake a cada 20 min simulados | `boot.b` ≈ 25 ppm (±5); `incerteza_base_ms` ≤ meia largura do menor RTT; sem `deriva_excessiva`; com `DerivaPPM=+60` ⇒ alerta |
| `geracao` | 1 dongle com `boot_id` inicial 65534 que reinicia 3× (65534 → 65535 → 1 → 2, a volta do u16), enviando blocos em cada boot; depois, o app virtual roda `tccelta worker reprocessar` (ensaio e `--aplicar`) sobre esse dongle | `dongle_estado.geracao_boot = 1` e `ultimo_boot_id = 2`; `boot` com `(0,65534)`, `(0,65535)`, `(1,1)`, `(1,2)` sem colisão de PK; `bloco_bruto`/`leitura`/`posicao` com a geração certa; alerta `geracao_boot_avancou` uma vez; após o reprocessamento `count(leitura)` igual ao anterior e `valor_bruto` intacto; `grupo_ancora` coerente com as âncoras de cada boot |
| `carga` | N dongles em regime (`BlocosPorSegundo` realista × `--acelerar`) | métricas abaixo |
| `reconexao` | N dongles acumulam `--acumulado` blocos offline e conectam todos dentro de `--rampa` | tempo de drenagem; 503 e 429 tolerados; `seq_contiguo = HEAD` ao fim para todos |
| `joelho` | sobe N em degraus (100, 250, 500, 1.000, 2.000, …) a cada 2 min | primeiro degrau com p99 > 1 s ou 503 > 1 % ⇒ capacidade reportada |

Saída: um resumo legível no stdout e `--relatorio` em JSON (`cenario`, `parametros`, `p50`/`p95`/`p99` por rota, `blocos_aceitos_por_s`, `linhas_por_s`, contagem por status, `drenagem_s`, `rtt_ancora_p50/p95`, `chamadas_kms_por_post`). O código de saída é 0 se todas as verificações e metas passaram e 1 caso contrário (usado pelo `make e2e` e pelo CI).

### Métricas-alvo

> **Status: documentadas, não executadas nesta fase.** A tabela abaixo é o que se mediria; os números ficam em branco até alguém rodar `make carga` e versionar o relatório. Nada na fase depende de eles terem sido medidos.

Medidas num host de referência, que precisa ser documentado no relatório: por exemplo, uma VM de 4 vCPU e 8 GB com tudo no mesmo host; banco com `TS_TUNE_MEMORY=4GB`. Na carga, o banco roda com `ALTER DATABASE tccelta SET tccelta.compressao_apos = '1 day'` **antes** do `migrar` (spec 02), porque `leitura` gera cerca de 150 GB/dia sem compressão a 34 mil linhas/s. Cada execução começa de volume limpo (`make limpar`).

| # | Cenário | Meta |
|---|---|---|
| M1 | `carga`, 1.000 dongles, 0,17 bloco/s cada (170 blocos/s, ~34 mil linhas/s), `T_lat = 60 s` | p99 `POST /blocos` < 300 ms; 0 respostas 5xx; 0 `40P01` (deadlock) no log do banco; CPU da API < 50 % |
| M2 | `carga`, M1 com `T_lat = 10 s` (~6× mais POSTs, lotes menores) | p99 < 300 ms; KMS ≤ 2 chamadas por POST |
| M3 | `reconexao`, 200 dongles × 2.000 blocos acumulados (400 mil blocos), rampa de 60 s | drenagem completa em < 10 min (≥ ~700 blocos/s); nenhum bloco perdido; 503 permitido, com o app virtual se recuperando |
| M4 | `joelho` | capacidade reportada (dongles e blocos/s); ela substitui a tabela de estimativas do doc no TCC |
| M5 | handshakes | p95 do handshake completo (sem BLE) < 150 ms; `rtt_ancora` = S2 − S1 dominado pela rede |
| M6 | KMS | `tccelta_kms_chamadas_total / tccelta_lotes_total ≤ 2`; ≥ 95 % dos lotes sem chamada ao KMS dentro da transação (métrica `tccelta_kms_chamadas_total{fase="tx"}`); com Vault em outro contêiner com latência injetada de 20 ms (`tc netem`, simulando VPN), M1 continua válida |

Comparações a registrar no relatório (alimentam decisões em [Aberto]):

- `VAULT_VERIFY_LOTE` 32/1 vs 8/4;
- `INGESTAO_MAX_SIMULTANEAS` 16/32/64;
- `API_REPLICAS` 1 vs 2;
- decodificação síncrona vs. assíncrona, se o [Aberto] da spec 05 for implementado.

### Infraestrutura de testes de integração (`internal/testeinfra`, build tag `integracao`)

```go
// SubirBanco sobe timescale/timescaledb-ha:pg17 (uma vez por pacote, via TestMain),
// aplica as migrations e devolve pools por papel (dono, api, worker, leitura).
func SubirBanco(ctx context.Context) (*Banco, func(), error)
func (b *Banco) Pool(papel string) *pgxpool.Pool
func (b *Banco) Limpar(t *testing.T) // TRUNCATE ... RESTART IDENTITY CASCADE entre testes

// SubirVault sobe hashicorp/vault em -dev, roda o bootstrap (mesmas políticas HCL de deploy/)
// e importa as chaves de teste; devolve o cliente da API (AppRole) e o admin.
func SubirVault(ctx context.Context) (*Vault, func(), error)

// SubirAPI monta o handler HTTP real com dependências reais (banco e Vault) em httptest.Server.
func SubirAPI(t *testing.T, b *Banco, v *Vault, ajustes ...func(*config.Config)) *httptest.Server
```

- O banco usa a estratégia de espera "`database system is ready to accept connections`" (2 ocorrências) + `pg_isready`.
- Rodando os testes **dentro** de um contêiner (`make teste-integracao`), o `Makefile` passa `-e TESTCONTAINERS_HOST_OVERRIDE=host.docker.internal --add-host=host.docker.internal:host-gateway` e monta `//var/run/docker.sock` (barra dupla e `MSYS_NO_PATHCONV=1` evitam a conversão de caminho do Git Bash). Os pacotes rodam com `-p 1`, para não subir várias instâncias de `timescaledb-ha` ao mesmo tempo. No host, `go test -p 1 -tags integracao ./...` funciona direto.
- Cada pacote reaproveita os contêineres no `TestMain` e limpa as tabelas entre testes. Tempo-alvo da suíte de integração: < 3 min.

### Ponta a ponta com a stack real (`compose.test.yaml` + `make e2e`)

`compose.test.yaml` = `banco` + `redis` + `vault` (dev) + `vault-bootstrap` + `migrar` + `api` (1 réplica) + `worker` + `ferramentas`, com os mesmos volumes e segredos separados da spec 01 e um nome de projeto próprio (`name: tccelta-teste`), para não colidir com a stack dev. `make e2e` (alvo da spec 01) executa, nesta ordem:

1. `docker compose -f compose.test.yaml up -d --build --wait`;
2. `provisionar importar --arquivo /testdata/dongles-teste.csv` (seed + teste dourado);
3. `simulador --cenario e2e --chaves … --banco-url-env SIM_BANCO_URL`;
4. `simulador --cenario protocolo-negativo …`;
5. `simulador --cenario tempo --acelerar 120 …`;
5b. `simulador --cenario geracao …` (volta do `boot_id` e reprocessamento);
6. `down -v`, **sempre** (mesmo com falha), devolvendo o código de saída dos cenários.

### CI do backend (`.github/workflows/backend-ci.yml`)

Gatilho: `paths: backend/**`. Jobs:

- `lint`: golangci-lint ≥ v1.64 (exigido por `go 1.24`) + `sqlc diff` (código gerado em dia) + `gofmt`;
- `unit`: `go test -race ./...`;
- `integracao`: `go test -tags integracao -race ./...` (o Docker do runner serve o testcontainers);
- `e2e`: `make e2e`;
- `vuln`: `govulncheck ./...`.

A carga **não** roda no CI (é manual, com relatório versionado em `backend/docs/relatorios/`).

## Plano arquivo por arquivo

| Arquivo | Conteúdo |
|---|---|
| `internal/simulador/dongle.go`, `flash.go`, `conducao.go` | dongle virtual |
| `internal/simulador/app.go` | app virtual |
| `internal/simulador/cenarios.go` | `e2e`, `protocolo-negativo`, `tempo`, `carga`, `reconexao`, `joelho` |
| `internal/simulador/verificacoes.go` | consultas SQL de verificação (`queries/verificacao.sql`, via sqlc) |
| `internal/simulador/relatorio.go` | histogramas (HDR simples em memória), JSON |
| `internal/simulador/*_test.go` | o dongle virtual contra a API real em `httptest` (smoke) |
| `internal/banco/queries/verificacao.sql` | consultas usadas só pelo simulador |
| `internal/testeinfra/*.go` | testcontainers |
| `cmd/tccelta/simulador.go` | subcomando |
| `compose.test.yaml` | stack de e2e |
| `.github/workflows/backend-ci.yml` | CI |
| `backend/docs/relatorios/AAAA-MM-DD-carga.md` | relatório versionado (gerado manualmente a partir do JSON) |

## Testes do próprio simulador

- O dongle virtual contra o `protocolo` (ida e volta), e o BOOT gerado obedece às regras do servidor (spec 05) em 1.000 perdas de energia aleatórias.
- A flash cheia descarta o mais antigo e o `TAIL_seq` relatado bate.
- A deriva configurada aparece no `t_ms` relatado (regressão linear contra o relógio real).

## Passos ordenados com checkpoints

1. `testeinfra` (banco + Vault). **Checkpoint:** os testes de integração das specs 02/04/05/07 usam a infraestrutura e passam.
2. Dongle virtual + app virtual mínimo (handshake + lotes). **Checkpoint:** `simulador --cenario e2e` com 1 dongle e sem eventos chega a `seq_contiguo = HEAD`.
3. Eventos (BOOT, LOSS, OVERFLOW, reconexão) + verificações SQL. **Checkpoint:** `make e2e` verde.
4. `protocolo-negativo` e `tempo`. **Checkpoint:** verdes.
5. `carga`, `reconexao`, `joelho` + relatório: **implementar os cenários e o gerador de relatório** (testes do próprio simulador verdes); **executar M1–M6 fica adiado** (decisão do autor). Quando for executado, **checkpoint:** relatório M1–M6 registrado; metas não atingidas viram *issues* com a decisão de mitigação (por exemplo, o modo assíncrono da spec 05).
6. CI. **Checkpoint:** PR no `backend/` roda lint, unit, integração e e2e verdes.

## Critérios de aceite

- [ ] O simulador usa o pacote `protocolo` da API e as chaves que estão no Vault de teste (nenhuma "porta dos fundos" de verificação).
- [ ] `make e2e` cobre BOOT, LOSS, OVERFLOW, âncora, vizinhos, rejeição por tag, duplicados e reconexão, e sai com 0.
- [ ] Metas de carga M1–M6 documentadas e cenários `carga`/`reconexao`/`joelho` implementados; **relatório medido e versionado: adiado** (não bloqueia a fase).
- [ ] CI do backend ativo.

## Riscos

| Risco | Mitigação |
|---|---|
| O simulador reimplementa o app e pode divergir do Flutter | os dois seguem a mesma tabela de ações (spec 05/09); os vetores são compartilhados; a validação final é com o app real (README) |
| Host de teste não representa o VPS | o relatório registra o hardware; repetir M1/M3 no VPS antes do texto final |
| Testcontainers no Windows | `make teste-integracao` dentro de contêiner com `host.docker.internal`; alternativa: WSL2 |
| Mil dongles provisionados poluem o Vault dev | o Vault dev é efêmero (memória); em prodlike, `--provisionar` é recusado sem `--confirmar` |

## Decisões do autor (antes em aberto)

- **[Decisão] Simulador próprio em Go** (descartados k6/vegeta): blocos assinados, estado por dongle e o Go-Back-N exigem a lógica do protocolo, que o simulador já tem, com o mesmo pacote `protocolo` da API e cenários BOOT/LOSS/OVERFLOW e K adaptativo.
- **[Decisão] Não executar os testes de carga agora, só documentá-los** (ver *Objetivo* e a tabela de metas).
- **[Decisão] Testes com o firmware real (1–3 dongles por 24 h):** não nesta fase. O procedimento para medir a deriva real do cristal e o RTT real da âncora está documentado na spec 06 (*Decisões do autor*, `DERIVA_MAX_PPM` mantido em 40 ppm sem medir).
