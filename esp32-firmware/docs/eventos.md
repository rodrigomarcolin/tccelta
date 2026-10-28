# Eventos do firmware (log estruturado pela serial)

O firmware do coletor emite, pela porta serial, uma linha por **evento**. É o
principal instrumento para depurar e validar o dongle em bancada: dá para
reconstruir o agendador, a gravação em flash, o pareamento BLE e o handshake de
sincronização sem alterar o comportamento medido.

Este documento descreve **todos os eventos hoje emitidos** pelo código em
`src/`, seus parâmetros e o que cada um significa. O catálogo em inglês,
com receitas de reconstrução e o formato binário do bloco, está em
[`../../docs/debug-events.md`](../../docs/debug-events.md); aqui o foco é a
leitura rápida de cada evento.

## 1. Como habilitar

Os eventos só existem em builds com a flag `COLLECTOR_EVENTS`. Sem ela, nenhum
código de evento é compilado (`collector::kEventsEnabled = false`).

| Ambiente PlatformIO | Eventos | Observação |
|---|---|---|
| `collector_mock_debug`, `collector_twai_debug`, `collector_mcp2515_debug` | sim | Serial a **921600 baud**. |
| `collector_*_bench_*` | sim, mas `FLUSH` sem `hex`/`tag` | Flag `COLLECTOR_EVENTS_NO_HEX`, para aliviar a serial. |
| `collector_mock`, `collector_twai`, `collector_mcp2515`, `*_devsec`, `*_prod` | **não** | Builds sem `COLLECTOR_EVENTS`. |

Para ler:

```
pio device monitor -e collector_mock_debug
```

Flags de bancada que alteram o que aparece no fluxo:

- `COLLECTOR_WIPE_ON_BOOT`: a cada boot o log em flash começa vazio e o `seq`
  volta a 1 (o `boot` continua crescendo). Aparece como `BOOT wiped=1`.
- `SECURITY_DEV_FIXED_KEYS`: usa as chaves fixas de bancada quando a partição
  `secnvs` não foi provisionada (`KEYS source=dev_fixed`).
- Comandos de bancada pelo canal ELM327, só em builds com eventos: `AT#ACK n`
  (injeta um ACK válido, como se viesse do app), `AT#SYNC?` (estado do motor de
  sincronização), `AT#CORRUPT n` (corrompe o bloco `n` na flash) e `AT#WIPE`
  (apaga no próximo boot). Aparecem como `CMD txt="AT#ACK n"` seguido de
  `HS_ACK`.

## 2. Formato da linha

Um evento por linha, terminada em `\r\n`. Só as linhas que começam com `EV `
são eventos; **ignore as demais** (o firmware imprime logs livres como
`[BLE] Link up` na mesma porta).

```
EV <n> <t> <NOME> campo=valor campo=valor …
```

| Parte | Significado |
|---|---|
| `n` | Contador de eventos, compartilhado por todas as tarefas e atômico. Reinicia em 0 a cada boot. **A ordem de `n` é a ordem real dos eventos.** |
| `t` | Milissegundos desde o boot no instante da emissão (não da impressão). Eventos dos dois núcleos podem ter o mesmo `t`; desempate por `n`. |
| `NOME` | Maiúsculas e `_`. |
| `campo=valor` | Chave em minúsculas. Valor sem espaço, ou entre aspas com escapes `\\ \" \r \n`. |

Tipos de valor:

| Tipo | Codificação | Exemplo |
|---|---|---|
| `int` | Decimal, pode ser negativo | `dur=12`, `lat=-235505200` |
| `ms` | Milissegundos desde o boot (mesmo relógio de `t`) | `next_due=12284` |
| `us` | Microssegundos | `verify_us=840` |
| `bool` | `0` ou `1` | `ok=1` |
| `hex` | Hexadecimal maiúsculo, sem `0x`, tamanho par | `pid=0C`, `val=1AF8` |
| `list` | Itens separados por vírgula, sem espaço | `pids=0C,0D` |
| `str` | Texto entre aspas | `txt="AT#PID 0C 250"` |
| `enum` | Uma das palavras listadas, em minúsculas | `reason=deadline` |
| ausente | `-` (valor indisponível) | `val=-` |

Regras para quem lê:

- **Buracos em `n`** (por exemplo 41 seguido de 45) significam eventos perdidos.
  O firmware também emite `LOG_DROP` quando a fila do logger enche. O estado
  reconstruído depois de um buraco não é confiável até o próximo snapshot
  completo (`CVT`).
- `FORMAT` marca um novo boot: `n` e `t` recomeçam em 0.
- Ignore eventos e campos desconhecidos (compatibilidade futura).
- Resolução de 1 ms: leituras menores que isso aparecem com `dur=0`.

## 3. Catálogo

Convenção das tabelas: **campo** `tipo` — descrição. `a|-` indica que o campo
pode vir ausente (`-`).

### 3.1 Boot e configuração

| Evento | Campos | Significado |
|---|---|---|
| `FORMAT` | `v` int | Primeiro evento de cada boot. `v` é a versão do formato (hoje `2`). |
| `KEYS` | `profile` (none, hmac, asym) · `source` (partition, dev_fixed, mac, missing) · `dev_id` hex · `tag_len` int · `fe` (off, dev, release) · `sb` bool · `load_us` us | Chaves carregadas. `tag_len` é o tamanho da tag do perfil (0, 32 ou 64). `fe` é o estado da criptografia de flash e `sb` o do secure boot (`1` = ativo). **Nunca mostra material de chave.** |
| `BOOT` | `boot` · `wiped` bool · `rsv` · `rsv_new` · `legacy_last` int\|- · `boot_seq` · `from` · `to` · `seq_next` · `log_oldest` · `log_newest` · `lost` | Recuperação de `seq` no boot. `rsv` é a reserva lida da NVS (0 = ausente) e `rsv_new` a gravada para o bloco BOOT. `boot_seq` é o seq do marcador, e `[from, to]` o intervalo que ele declara (vazio se `to = from - 1`). `seq_next` é o próximo seq. `log_oldest`/`log_newest` são os limites do log em flash e `lost` o total de blocos perdidos. |
| `LOG_MIGRATE` | `removed` int | Arquivos do log v1 (`/log`) removidos no primeiro boot do firmware v2. |
| `NVS` | `free` · `used` · `total` | Entradas da partição `nvs` no boot (compartilhada por bonds e coleta). |
| `CONFIG` | `reason` (boot, apply) · `proto` int · `tmax` int · `flush` int · `u` int · `pids` list | Configuração ativa. `proto` 6 a 9 (CAN 11/29 bits, 500/250 kbps). `tmax` é o prazo de resposta do carro em ms. `flush` é o prazo de fechamento de bloco em ms (1000 a 30000, padrão 10000). `u` é a utilização planejada do barramento **em permil** (`Σ tmax/p × 1000`, ex. `782`). Seguido de uma linha `CVT` por PID. |
| `CVT` | `pid` hex · `p` int · `pmin` int · `due` ms · `last` ms · `val` hex\|- | Snapshot de uma linha da tabela do agendador, emitido para cada PID logo depois de `CONFIG`. `p` é o período, `pmin` o intervalo mínimo, `due` o próximo vencimento e `last` a última leitura. |

### 3.2 Agendador e barramento

| Evento | Campos | Significado |
|---|---|---|
| `DECIDE` | `rule` (due, early) · `pids` list · `late` int\|- · `slack` int\|- | O agendador decidiu ler. `rule=due`: regra 1, `late` = atraso em ms sobre o menor `due` (`slack=-`). `rule=early`: regra 2 (leitura antecipada com barramento ocioso), `slack` = ms até o menor `due` da tabela (`late=-`). Esperas **não** são registradas. |
| `READ` | `pid` hex · `ok` bool · `val` hex\|- · `start` ms · `dur` int · `early` bool · `next_due` ms | Um PID foi lido. `t` é o fim da leitura e passa a ser o novo `last` da linha. `dur = t - start` é a ocupação do barramento. `ok=0` = sem resposta (`dur ≈ tmax`). `next_due` é o novo vencimento. Há um `READ` por PID do `DECIDE` anterior. |
| `BUS` | `kind` (diag) · `start` ms · `dur` int | O barramento foi usado para algo fora da leitura agendada (uma consulta diagnóstica do app). A resposta do carro está no `RESP` seguinte. |

### 3.3 Blocos em RAM

| Evento | Campos | Significado |
|---|---|---|
| `BLK_OPEN` | `buf` int · `t0` ms | Um buffer foi tomado e um bloco aberto. `t0` é o início do bloco (instante do primeiro registro). |
| `REC` | `kind` (pid, gps) · `pid` hex\|- · `dt` int · `val` hex\|- · `lat` `lon` `alt` int\|- · `len` int | Um registro entrou no bloco aberto. `dt = tempo do registro - t0` (pode ser **negativo** no GPS, recebido pouco antes do bloco abrir). PID: `val` sempre com 2 bytes (PIDs de 1 byte vêm com `00` no segundo). GPS: `lat`/`lon` em graus × 10⁷, `alt` em mm. `len` = bytes usados no bloco depois do registro, cabeçalho incluso. |
| `REC_DROP` | `kind` (pid, gps) · `pid` hex\|- · `reason` (no_buffer) | Um registro não coube: sem buffer livre. |
| `BLK_CLOSE` | `buf` · `reason` (full, deadline, config, mode, stop) · `len` · `recs` · `age` | O bloco aberto foi fechado. `age = t - t0`. `full`: o próximo registro não cabia. `deadline`: passou `CONFIG.flush` ms. As demais: mudança de configuração, de modo ou reinício. O bloco segue para a gravação em flash. |
| `BLK_RELEASE` | `buf` int | Um bloco vazio foi fechado e o buffer devolvido sem gravar. |

### 3.4 Flash

| Evento | Campos | Significado |
|---|---|---|
| `FLUSH` | `buf` int\|- · `kind` (data, boot) · `seq` · `boot` · `len` · `tag_len` · `ok` bool · `attempts` · `reserved` bool · `reserve_us` · `sign_us` · `crc_us` · `write_us` · `dur` · `crc` hex · `hex` hex\|- · `tag` hex\|- | Terminou a gravação de um bloco. `t` é o fim; `dur` é o tempo total em ms. `reserved=1`: o `seq_rsv` foi gravado na NVS antes (`reserve_us`). `hex` traz **os bytes exatos gravados** e `tag` os `tag_len` bytes da tag; `crc` é o CRC32 de dados e tag, 8 dígitos hex (gravado em little-endian na flash). Builds `*_bench_*` imprimem `hex=-` e `tag=-`. `ok=0`: as duas tentativas falharam, o bloco foi descartado e o `seq` **não** foi consumido. |
| `MARKER` | `kind` (boot, loss) · `seq` · `from` · `to` · `reason` (boot, crc, missing) | Um bloco marcador foi montado: BOOT no boot (antes do `FLUSH kind=boot`) ou LOSS na hora do envio (nunca é gravado). |
| `SEG_NEW` | `first` · `tag_len` · `reason` (first, full, boot, sealed, tag_len) | Um novo segmento do log foi aberto. |
| `SEG_SEAL` | `first` · `last` · `reason` (write_fail, scan_tail) | Um segmento deixou de aceitar escrita (falha de gravação, ou cauda inválida encontrada no boot). |
| `SEG_DROP` | `first` · `last` · `count` | Flash cheia: o segmento mais antigo (blocos `first..last`) foi apagado. Esses blocos estão perdidos. |

### 3.5 Comandos do app (canal ELM327)

| Evento | Campos | Significado |
|---|---|---|
| `CMD` | `txt` str | Comando recebido (`AT…`, `AT#…` ou requisição OBD como `010C`), sem CR/LF. |
| `RESP` | `txt` str · `dur` int | Resposta enviada ao `CMD` anterior. CR/LF aparecem escapados como `\r` e `\n`. `dur` é o tempo de processamento em ms. |

### 3.6 Modos, GPS e medição de Tmax

| Evento | Campos | Significado |
|---|---|---|
| `MODE` | `diag` bool · `running` bool | O modo mudou. `running=1` = agendador ativo. Fica 0 no modo diagnóstico, durante a medição de Tmax e depois de uma parada. |
| `GPS` | `lat` · `lon` · `alt` · `t_rx` ms | Uma posição do app foi aceita. `t_rx` é quando o comando chegou. Seguida de `REC kind=gps` ou `REC_DROP`. |
| `GPS_DROP` | `reason` (diag, rate, queue) · `t_rx` ms | Uma posição foi descartada: `diag` = modo diagnóstico; `rate` = menos de 1 s (menos 100 ms de tolerância) desde a última aceita; `queue` = fila da tarefa de aquisição cheia. |
| `TMAX_START` | `pids` list · `samples` int | Medição de Tmax iniciada, com `samples` leituras por PID. |
| `TMAX_SAMPLE` | `pid` hex · `ok` bool · `lat` int | Uma requisição da medição. `lat` é a latência da resposta em ms. |
| `TMAX_DONE` | `tmax` int | Fim da medição, com o Tmax sugerido em ms. |

### 3.7 Conexão, SYNC e handshake

Campos `*_us` são microssegundos e nunca incluem o custo de formatar o evento.

| Evento | Campos | Significado |
|---|---|---|
| `CONN` | `up` bool · `gen` int | Sessão **autorizada** aberta ou fechada (enlace cifrado com bond LESC válido). `gen` é a geração da conexão. Uma conexão recusada nunca produz `CONN`. |
| `NET` | `up` bool | O app informou Internet disponível ou indisponível (informativo). |
| `SYNC_SUB` | `on` bool | Notificações em SYNC_TX ligadas ou desligadas. |
| `SYNC_MTU` | `mtu` int | O MTU negociado mudou. |
| `SYNC_RX` | `type` hex\|- · `len` · `q_us` · `ok` bool · `reason` (ok, short, bad_len, unexpected, queue_full) | Mensagem recebida em SYNC_RX. `queue_full` (com `type=-`, `len` = mensagens perdidas) sai da tarefa de sync a partir de um contador. |
| `SYNC_TX` | `type` hex · `len` · `frags` · `mtu` · `dur_us` · `retries` · `ok` bool | Uma mensagem foi enviada (todas as notificações). |
| `SYNC_ERR` | `dir` (tx, rx) · `code` hex | Mensagem de erro (0x06) enviada ou recebida. |
| `HS_CHALLENGE` | `c_s` · `last_cs` · `ack` · `c_conf` · `flags` hex · `diag_min` int\|- · `len` · `verify_us` · `result` (ok, bad_len, bad_tag, old_cs, nvs_fail) | Desafio do servidor processado. |
| `HS_RESPONSE` | `c_s` · `tail` · `head` · `boot` · `t_ms` ms · `deleted_upto` int\|- · `nvs_us` · `tag_us` · `total_us` · `q_us` | Resposta enviada. `t_ms` é a âncora de tempo. `deleted_upto` aparece quando o desafio liberou flash. |
| `HS_ACK` | `ack` · `c_conf` · `flags` hex · `verify_us` · `result` (ok, bad_len, bad_tag, bad_seq) · `advanced` bool · `opened` bool · `anchor_ok` bool | ACK processado. `opened=1`: abriu a sessão. |
| `SYNC_SESSION` | `open` bool · `reason` (connect, ack, overflow, disconnect) | O estado da sessão mudou. |
| `SYNC_NOTICE` | `head` · `flags` hex · `reason` (subscribe, flush, ack, resync, overflow, flash, retry) | Aviso de novo bloco (0x07) enviado. |
| `SYNC_REQ` | `k` · `cursor` · `head` · `end` int\|- · `result` (ok, no_session, empty, ignored) | Pedido de lote recebido. |
| `SYNC_BLOCK` | `seq` · `kind` (data, boot, loss) · `len` · `tag_len` · `read_us` · `crc_us` us\|- · `crc_ok` bool\|- · `sign_us` us\|- · `tx_us` · `frags` · `mtu` · `cursor` | Um bloco foi enviado. `cursor` é o valor depois do envio. |
| `SYNC_BATCH_END` | `reason` (done, link, overflow, app_error, superseded, empty) · `sent` · `bytes` · `dur_us` | Fim de um lote. |
| `SYNC_CURSOR` | `from` · `to` · `reason` (connect, ack, app_error, boot_gap, loss) | O cursor saltou fora do +1 normal por bloco. |
| `SYNC_FREE` | `upto` · `segs` · `oldest` · `dur_us` | Flash liberada depois de um ACK ou desafio. |
| `SYNC_OVERFLOW` | `upto` · `ack` | O descarte por flash cheia atingiu blocos não confirmados; a sessão foi invalidada. |

### 3.8 Saúde do sistema

| Evento | Campos | Significado |
|---|---|---|
| `STACK` | `task` (acquisition, flush, elm327, sync, pairing, logger) · `free` int | A cada 60 s, uma linha por tarefa: mínimo de pilha livre já observado, em bytes. |
| `HEAP` | `free` · `min` | A cada 60 s: heap livre atual e mínimo, em bytes. |
| `LOG_DROP` | `count` int | `count` eventos foram perdidos porque a fila do logger encheu. Também aparece como buraco em `n`. |

### 3.9 Pareamento e segurança do enlace (LESC Just Works)

`conn` é o handle de conexão do NimBLE (o mesmo em todos os eventos de uma
conexão; pode ser reutilizado depois de `LINK_DOWN`).

| Evento | Campos | Significado |
|---|---|---|
| `PAIR_BTN` | `action` (press, hold, release, blocked) · `held` int · `fired` bool | Transição do botão de pareamento (BOOT, GPIO0). `press`: apertou, depois do debounce (`held=0`). `hold`: atingiu `PAIR_BUTTON_HOLD_MS` (`fired=1`). `release`: soltou; `held` é a duração total e `fired=1` se disparou. `blocked`: o firmware ligou com o botão apertado e o ignora até ser solto. |
| `PAIR_WINDOW` | `state` (open, close) · `reason` (boot, button, timeout, bonded, error) · `deadline` ms\|- · `dur` int\|- | Janela de pareamento. `open` (sempre `reason=button`): `deadline` é o instante em que fecha. `close`: `dur` é o tempo aberta (`-` em `reason=boot`). `bonded`: um celular pareou. `timeout`: passou o prazo (45 s). `error`: não abriu porque apagar os bonds falhou (`dur=0`). |
| `BOND_DELETE` | `reason` (button, policy, recover) · `count` · `ok` bool · `attempts` · `dur` | Bonds apagados. `button`: ao abrir a janela. `policy`: higiene no boot (mais de um bond, ou bond legado) ou o bond de uma conexão recusada (`count=1`, `dur=0`). `recover`: o namespace `nimble_bond` inteiro é apagado antes de reiniciar. |
| `BOND_VERIFY` | `conn` · `ok` bool · `lesc` bool · `persisted` bool | O bond novo foi conferido no armazenamento logo depois do pareamento. `lesc=0` indica bond legado. `ok=0` recusa a conexão e mantém a janela aberta. |
| `BOND_FULL` | `event` (full, overflow) · `action` (reject, allow) · `obj` int · `conn` int\|- · `window` bool | `full`: chegou um pedido de pareamento com o armazenamento cheio (qualquer pedido quando já existe um bond, `MAX_BONDS=1`); `reject` fora da janela (o bond existente fica intacto) e `allow` dentro dela. `overflow`: tentativa de gravar um segundo bond; sempre `reject`, com `obj` = tipo de objeto do NimBLE. |
| `CONN_DROP` | `reason` (button) · `count` | Todas as conexões foram derrubadas (`0x13`) para um novo pareamento. Cada uma ainda gera seu `LINK_DOWN`. |
| `ADV` | `mode` (off, normal, pairing) · `window` bool · `bonded` bool | O anúncio foi reconfigurado (só quando algo muda). `window` e `bonded` são os bits do campo de dados de fabricante `FF FF 01 flags`. |
| `LINK_UP` | `conn` · `addr` hex · `atype` (pub, rnd) · `window` bool | Enlace aceito, **ainda não autorizado**. `addr` é o endereço no ar (ordem de exibição, sem `:`; em celulares costuma ser um RPA que rotaciona). `window=1` se a janela estava aberta. |
| `LINK_ENC` | `conn` · `up` bool · `bonded` bool · `fresh` bool · `key` int · `reason` (ok, pairing, other) · `rc` int · `dur` int | Resultado da criptografia (`ENC_CHANGE`). `fresh=1`: chave nova (pareamento); `0`: LTK restaurada de um bond. `key` é o tamanho da chave em bytes (16). `up=0`: `pairing` = o pareamento falhou (veja `PAIR_RESULT`); `other` = qualquer outro motivo. Restaurar sem LTK no dongle não emite nada. `dur = t - LINK_UP.t`. |
| `PAIR_RESULT` | `conn` · `ok` bool · `reason` (ok, window_closed, not_bonded, legacy, not_persisted, busy, peer_reject, local_reject, timeout, other) · `late` bool · `rc` int · `dur` int | Fim de uma tentativa de pareamento que chegou ao Security Manager (não é emitido em restaurações, nem em pedidos recusados por `BOND_FULL`). `ok=1` só com bond LESC criado na janela e confirmado no armazenamento. `late=1`: admitido na janela e concluído depois que ela fechou. `dur` inclui o diálogo do sistema. |
| `CONN_REJECT` | `conn` · `reason` (not_bonded, enc_fail, key_size, auth_timeout, busy) · `hci` hex · `dur` int | O dongle derruba a conexão. `hci` é o motivo enviado (`05` recusa, `09` ocupado). `not_bonded`: sem bond fora da janela. `auth_timeout`: não autenticou a tempo. |
| `LINK_DOWN` | `conn` · `rc` int · `hci` hex\|- · `auth` bool · `dur` int | Enlace caiu. `rc` é o motivo bruto do NimBLE; `hci = rc - 0x200` para erros HCI (`13` remoto, `16` local, `08` timeout). `auth=1` se era a sessão autorizada. |
| `WRITE_DROP` | `conn` · `chr` (nus_rx, sync_rx) · `len` | Escrita vinda de conexão não autorizada, descartada. |

## 4. Como ler sequências comuns

**Pareamento bem-sucedido** (logs reais da bancada):

```
PAIR_BTN action=press held=0 fired=0
PAIR_BTN action=hold held=5000 fired=1
BOND_DELETE reason=button count=0 ok=1 attempts=1 dur=0
PAIR_WINDOW state=open reason=button deadline=296933 dur=-
ADV mode=pairing window=1 bonded=0
LINK_UP conn=0 addr=… atype=rnd window=1
LINK_ENC conn=0 up=1 bonded=1 fresh=1 key=16 reason=ok rc=0 dur=3286
BOND_VERIFY conn=0 ok=1 lesc=1 persisted=1
PAIR_RESULT conn=0 ok=1 reason=ok late=0 rc=0 dur=3286
PAIR_WINDOW state=close reason=bonded deadline=- dur=32820
CONN up=1 gen=…
ADV mode=off window=0 bonded=1
```

`lesc=1` e `key=16` confirmam pareamento LESC com chave de 128 bits. Depois do
bond, os dados de fabricante passam de `0100` para `0102` (bit 1 = há bond).

**Conexão recusada fora da janela:** `LINK_UP … window=0` → `PAIR_RESULT ok=0
reason=window_closed` (ou `not_bonded`) → `CONN_REJECT reason=not_bonded hci=05`
→ `LINK_DOWN`. Não aparece `CONN`.

**Reconexão com bond:** `LINK_UP` → `LINK_ENC up=1 fresh=0` (chave restaurada)
→ `CONN up=1`.

**Janela expira sem pareamento:** `PAIR_WINDOW state=close reason=timeout
dur≈45000` e `ADV mode=normal`.

**Bloco do coletor até a flash:** `BLK_OPEN` → vários `READ` e `REC` → `BLK_CLOSE`
→ `FLUSH ok=1`.

**Sincronização:** `SYNC_SUB on=1` → `SYNC_NOTICE` → `HS_CHALLENGE result=ok` →
`HS_RESPONSE` → `HS_ACK result=ok opened=1` → `SYNC_SESSION open=1` →
`SYNC_REQ result=ok` → `SYNC_BLOCK` (um por bloco) → `SYNC_BATCH_END
reason=done` → `SYNC_FREE`.

## 5. Limites

- Eventos perdidos (`LOG_DROP`) não se recuperam; o visualizador deve mostrar o
  buraco em `n`.
- Esperas do agendador não são registradas; infira-as pela tabela reconstruída.
- Os tempos são relativos ao boot. O tempo absoluto só vem da âncora enviada pelo
  servidor (`HS_RESPONSE t_ms`).
- O log pela serial a 921600 baud pode atrasar a impressão, mas não a medição:
  `t` e `n` são tomados na emissão, e a impressão acontece em uma tarefa de baixa
  prioridade.
