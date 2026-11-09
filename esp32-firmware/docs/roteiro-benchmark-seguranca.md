# Roteiro de benchmark: com e sem segurança (none, hmac, asym)

Mede no dongle o custo de cada perfil de segurança do bloco pelo log de eventos, com as
métricas B1 a B12 da spec `.spec/fase-1-seguranca-telemetria/06-firmware-eventos-benchmark.md` §Benchmark.
Ferramentas em `tools/event-bench/` (`capture.py`, `bench_report.py`).

Todos os comandos são para o **Git Bash** e partem da pasta `esp32-firmware`, com a mesma preparação
de sessão do `docs/roteiro-teste-limite-blocos.md` §0.3 (`conda activate tccelta`, `PYTHONUTF8=1`).

## 1. O que cada coisa mede

O fluxo do app tem três atividades diferentes, cada uma com seus eventos:

| Atividade | Quando acontece | Eventos no dongle | Métricas |
|---|---|---|---|
| **Gravação** do bloco na flash | A cada bloco fechado (~1 a cada 6–10 s no mock) | `FLUSH` | B1, B2 (gravação), B3, B9, B11, B12 |
| **Handshake** (desafio → resposta → ACK de abertura) | Uma vez por conexão ou boot, ou pelo botão "Forçar handshake" | `HS_CHALLENGE`, `HS_RESPONSE`, `HS_ACK opened=1` | B4 |
| **Lote BLE** (app pede, dongle envia) | A cada aviso de bloco novo; em regime, ~1 bloco por lote | `SYNC_BLOCK`, `SYNC_BATCH_END` | B2 (leitura), B6, B8 |
| **POST + ACK** (app → servidor → ACK ao dongle) | Quando vence o `T_lat`, há 32 pendentes ou a flash está alta | `HS_ACK opened=0` | B10 |

O `capture.py` só escuta a serial. As métricas de handshake, lote e ACK só aparecem se o **app
pareado** estiver conectado e sincronizando durante a captura. O `tools/sync-bench/sync_bench.py`
não serve aqui: ele não pareia, e os envs de bench exigem LESC.

`bench_report.py` não calcula B5 (handshake ponta a ponta) nem B7 (overhead de fragmentação), e
B6 sai só por lote (`.spec/fase-1-seguranca-telemetria/DESVIOS-E-MELHORIAS.md`).

## 2. Preparação (uma vez)

### 2.1 Firmware

Envs `collector_mock_bench_{none,hmac,asym}` (ou `collector_twai_bench_*` no carro). Herdam do
`*_debug` (eventos a 921 600 baud, `SECURITY_DEV_FIXED_KEYS`) e tiram o hexa do `FLUSH`
(`COLLECTOR_EVENTS_NO_HEX`) para aliviar a serial. Use **o mesmo hardware base** (todos mock ou
todos twai) nas três séries.

**Chaves iguais nos três perfis.** Cada perfil exige chaves diferentes na `secnvs`
(`src/storage/KeyStore.cpp`): hmac precisa de `k_rec`, asym de `k_sig`, none só do `dev_id`. Faltando
alguma, o build cai nas chaves fixas de desenvolvimento, com **outro** `dev_id` e outra `K_auth`.
Escolha um caminho:

- **`secnvs` vazia** (dongle nunca provisionado): os três perfis usam as chaves fixas. É o mais simples.
- **Provisionado com `--security asym`** (`tools/provision/provision.py`): a partição tem `k_rec` e `k_sig`, e os três perfis usam as mesmas chaves.

Um dongle provisionado como hmac **não** serve: o build asym cairia nas chaves fixas.

Confira no evento `KEYS` de cada execução: `profile` é o esperado e `source` e `dev_id` são iguais
nas três séries.

### 2.2 App

Build **debug** com o servidor de bancada embutido, para não depender do backend:

```bash
cd ../mobile-app
fvm flutter run --dart-define=SYNC_SERVER=bench
# se o dongle foi provisionado (2.1), passe as chaves do bench_keys.json:
#   --dart-define=SYNC_BENCH_KAUTH=<hex> --dart-define=SYNC_BENCH_KREC=<hex>
```

O servidor de bancada assina o handshake e os ACKs com `K_auth`, verifica a tag hmac com `K_rec`
e aceita blocos none/asym sem verificar. Assim, as métricas do dongle não dependem do perfil
cadastrado num servidor. Já o **B10 não inclui o custo de verificação do servidor real** (KMS no
hmac, ECDSA no asym). Esse custo é medido à parte, no próprio servidor (seção 6).

Na tela de configurações de sincronização:

- Fixe o **`T_lat`** e não mude mais até o fim das séries. Na seção "Estado", confira que o `T_lat`
  efetivo é o mesmo em todas as execuções.
- O botão **"Forçar handshake"** (seção "Depuração", só em debug) é usado na fase B.

Pareie o celular com o dongle antes da primeira execução. O bond fica na NVS e sobrevive ao upload
do firmware.

### 2.3 O que anotar

Modelo do celular, versão do Android ou iOS, `T_lat`, MTU (evento `SYNC_MTU`), commit do firmware,
`CONFIG_BT_NIMBLE_MSYS1_BLOCK_COUNT` (24) e `SYNC_TX_MAX_CREDITS` (4).

## 3. Uma execução (~35 min)

Cada execução gera **três arquivos de log**, um por fase, porque o `bench_report.py` calcula tudo
sobre o arquivo inteiro e não separa as fases. Nome sugerido:
`logs/<env>-r<execução>-<fase>.log`, por exemplo `logs/collector_mock_bench_hmac-r2-regime.log`.

### 3.0 Gravar e reiniciar

```bash
pio run -e collector_mock_bench_hmac -t upload
```

O upload já reinicia a placa. Não é preciso apagar a flash nem forçar um `boot_id` novo: o
relatório não usa o `boot_id`. Como o upload ocupa a porta serial, o `BOOT` e o `KEYS` passam antes
de a captura começar. Por isso:

1. Inicie a captura da fase A (abaixo).
2. Aperte **EN uma vez** para o log ter `BOOT` e `KEYS` (confira o perfil e o `dev_id`).
3. Espere o app reconectar e abrir a sessão (`HS_ACK opened=1`).

**Não reinicie a placa no meio da execução.** Um boot abre segmento novo e marcador `BOOT`, e força
um handshake fora do roteiro.

### Fase A: regime (10 min)

```bash
python tools/event-bench/capture.py --port COMx --out logs/collector_mock_bench_hmac-r1-regime.log
```

Deixe o app conectado e em primeiro plano, **sem tocar em nada**, por 10 min. Esperado: ~60–90
`FLUSH` e um ACK a cada `T_lat`. Pare com Ctrl+C.

O handshake da abertura (logo após o EN) cai nesta fase. Ele é igual nas três séries, então não
distorce a comparação, mas o B4 desta fase tem `n` = 1. O B4 vem da fase B.

### Fase B: handshakes (10 min)

```bash
python tools/event-bench/capture.py --port COMx --out logs/collector_mock_bench_hmac-r1-handshake.log
```

Aperte **"Forçar handshake" a cada 30 s**, 20 vezes no total.

- O servidor real limita a 6 desafios por minuto por dongle (`LIMITE_DESAFIO_MIN`). Não force com
  menos de ~12 s de intervalo se um dia usar o modo `https`.
- Enquanto o handshake roda, o app descarta os blocos que chegam (sessão em `handshaking`) e depois
  pede tudo num lote maior. Nada se perde, mas os lotes e as gravações desta fase não são de
  regime. Por isso esta fase fica num arquivo separado.

### Fase C: acúmulo e vazão (~12 min)

```bash
python tools/event-bench/capture.py --port COMx --out logs/collector_mock_bench_hmac-r1-vazao.log
```

Repita 3 vezes: desligue o Bluetooth do celular (ou afaste-o) por **3 min**, religue, e espere o
lote grande terminar (`SYNC_BATCH_END`) e o buffer esvaziar. Com ~1 bloco a cada 6–10 s, cada
lote leva ~18–30 blocos. Use **o mesmo tempo desconectado** em todas as execuções: o tamanho do
lote muda a vazão.

Em regime, cada lote leva ~1 bloco, e o B6 da fase A reflete mais a latência do BLE do que a
vazão. O B6 que vale é o desta fase.

## 4. Séries: 3 execuções por perfil, intercaladas

Ordem: none r1, hmac r1, asym r1, none r2, hmac r2, asym r2, none r3, hmac r3, asym r3.

Intercalar espalha pelos três perfis qualquer deriva ao longo do dia (bateria e temperatura do
celular, aquecimento da placa). São 9 execuções, ~5 h. Dá para dividir em dois dias, mantendo o
mesmo celular, o mesmo app e o mesmo `T_lat`.

Amostras por perfil, somando as 3 execuções:

| Métrica | Fonte | n por perfil |
|---|---|---|
| B1, B2, B3, B9, B10 | `FLUSH` da fase A | ~180–270 |
| B4 | handshakes da fase B | 60 |
| B6 | lotes grandes da fase C | 9 |
| B12 | mínimo de cada execução | 3 |

Com `n` ≈ 20, o p95 ainda fica colado no máximo (`bench_report.py` interpola em `(n−1)·0,95`). Média
e mediana já são estáveis com 20–30. Para citar p95, junte as 3 execuções (`n` ≥ 60).

## 5. Relatório e análise

Por fase, com os nove logs (3 perfis × 3 execuções) de uma vez:

```bash
python tools/event-bench/bench_report.py logs/*-regime.log    --out-dir report/regime
python tools/event-bench/bench_report.py logs/*-handshake.log --out-dir report/handshake
python tools/event-bench/bench_report.py logs/*-vazao.log     --out-dir report/vazao
```

Cada `bench.md` tem uma tabela por log, e `bench.csv` tem uma linha por evento, com a coluna `log`,
para juntar as execuções de um perfil numa planilha ou no pandas.

Métricas a usar por fase:

| Fase | Use | Ignore (distorcido pela fase) |
|---|---|---|
| Regime | B1, B2, B3, B8, B9, B10, B11, B12 | B4 (`n` = 1), B6 (lotes de 1 bloco) |
| Handshake | B4 | B3, B6, B8, B10 |
| Vazão | B6 | B10 |

**Já é suficiente?** Compare a diferença **entre perfis** com a diferença **entre execuções do
mesmo perfil**. Se as faixas das 3 execuções não se sobrepõem entre perfis, a conclusão está
firme. Se se sobrepõem (provável no B6, em que o BLE domina), essa é a conclusão: a diferença fica
dentro da variação normal. Mais 1 ou 2 execuções só se quiser afirmar algo além disso.

Checagens:

- `KEYS` de cada log: perfil certo e mesmo `dev_id` nas séries.
- B11 (`SEG_DROP` / `SYNC_OVERFLOW`) deve ser `0 / 0`. Se não for, a execução teve perda e deve ser refeita.
- Buraco na sequência `n` dos eventos: a serial perdeu linhas.

## 6. Custo no servidor e validação ponta a ponta

As séries acima medem o dongle. O servidor real não muda nenhuma métrica do dongle: os `*_us` são
medidos dentro do ESP32, e o `HS_RESPONSE total_us` não inclui a ida ao servidor. A única métrica
que mudaria é o B10, mas ele é dominado pelo `T_lat` (dezenas de segundos) e pela rede do celular.
A verificação no servidor (µs a dezenas de ms) se perde nesse ruído. Por isso o custo do servidor
é medido no próprio servidor, e o servidor real entra só numa validação curta.

### 6.1 Custo de verificação por perfil: simulador + histograma

O simulador da Fase 3 (`.spec/fase-3-servidor/10-simulador-e-testes-de-carga.md`) cria dongles
virtuais do perfil pedido e roda o cenário `load` contra a API real. A API registra o tempo de
verificação no histograma `tccelta_block_verify_duration_seconds{profile}`
(`backend/internal/usecase/ingestion/receive_batch.go`). Ele mede só a verificação, sem rede,
banco nem `T_lat`.

Comandos a partir de `backend/`, com a stack de desenvolvimento no ar e o Prometheus ligado:

```bash
docker compose --profile observabilidade up -d prometheus      # http://127.0.0.1:9099
# none só é aceito com PERMITIR_PERFIL_NONE=true na API (proibido em produção; docs/ambientes.md)

make scenario C=load PROFILE=hmac N=20 ARGS='--duration 10m --tlat 30s --report /saida/carga-hmac.json'
make scenario C=load PROFILE=asym N=20 ARGS='--duration 10m --tlat 30s --report /saida/carga-asym.json'
make scenario C=load PROFILE=none N=20 ARGS='--duration 10m --tlat 30s --report /saida/carga-none.json'
```

- Cada `make scenario` cria dongles novos (o servidor guarda o estado por DevId).
- Os três perfis precisam de **N, `--tlat` e `--duration` iguais**. Uma observação do histograma
  cobre uma fatia de até `KMS_VERIFY_LOTE` blocos, então o tamanho do lote muda o valor.
- O `load` sai de um único IP e respeita o `LIMITE_IP_MIN` (120/min). Com 20 dongles e `--tlat 30s`
  são ~40 POSTs por minuto, com folga. Se subir N, suba o `--tlat` junto.

Consultas no Prometheus (`http://127.0.0.1:9099`), uma linha por perfil:

```promql
# média por chamada de verificação (exata)
sum by (profile) (tccelta_block_verify_duration_seconds_sum)
  / sum by (profile) (tccelta_block_verify_duration_seconds_count)

# p95 (aproximado: os buckets vão de 0,1 ms a 1 s)
histogram_quantile(0.95, sum by (le, profile) (rate(tccelta_block_verify_duration_seconds_bucket[10m])))
```

Os contadores são acumulados desde que a API subiu. Como o rótulo `profile` separa os perfis, não
é preciso reiniciar entre as rodadas, desde que nada mais esteja enviando blocos. O `none` pode
não aparecer, se a API não chegar a chamar a verificação nesse perfil: então o custo é zero.
Complementos: `tccelta_kms_duration_seconds` (só hmac), `tccelta_http_duration_seconds` por rota
e o `relatorio` JSON de cada rodada (p50/p95/p99 por rota).

### 6.2 Validação ponta a ponta com o dongle real (1 execução curta por perfil)

Mostra que o caminho completo (dongle → app → API → banco) funciona nos três perfis. É uma
demonstração, não uma medida estatística: os números de desempenho vêm das séries (seções 3–5)
e da 6.1. **Ainda não executado**; confirme cada passo na primeira vez.

Preparação (uma vez):

1. Provisione o dongle com `tools/provision/provision.py --profile dev --security asym --flash`.
   A `secnvs` fica com `k_rec` e `k_sig` e serve aos três builds (2.1).
2. Copie o `kms_record.json` para `backend/saida/` e cadastre-o com
   `make kms-import FILE=/saida/kms_record.json` (em `backend/`; o caminho é o de dentro do
   contêiner; KMS primeiro, banco depois). O cadastro fica com `security_profile = 'asym'` e a `public_key`.
3. Na API, `PERMITIR_PERFIL_NONE=true` (só para a rodada do none).
4. App em build normal (`SYNC_SERVER=https`, que é o padrão), apontando para a API.

O `tccelta provision register` só insere (um dongle já cadastrado fica como está). Por isso, a
troca de perfil é feita à mão no banco, **3 vezes no total** (uma por perfil):

1. Deixe o app esvaziar o buffer (na tela de estado: buffer 0 e ACK = HEAD). Senão a API
   verifica blocos do perfil anterior com o perfil novo e os rejeita.
2. Troque o perfil no cadastro (`make psql` em `backend/`):
   ```sql
   UPDATE dongle SET security_profile = 'hmac' WHERE dev_id_hex = '<DEVID>';
   ```
3. Espere **mais de 60 s** (TTL do cache de dongles da API, `backend/internal/adapter/cache/dongles.go`)
   ou reinicie a API.
4. Grave o env do perfil (`collector_mock_bench_hmac`), capture uns 10 min de regime como na
   fase A e force 2 ou 3 handshakes.

Passa se, ao fim de cada perfil:

- `rejected_block` não tem linhas novas do dongle;
- `dongle_state.contiguous_seq` = HEAD do dongle (último `FLUSH seq` do log);
- `tccelta_blocks_total` cresceu, e o app não mostra POSTs com falha.

Ao terminar, volte o cadastro para `asym` (o perfil com que foi provisionado) e desligue o
`PERMITIR_PERFIL_NONE`.
