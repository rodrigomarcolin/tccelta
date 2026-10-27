# 02 — Firmware: botão, janela de pareamento e política de enlace

> Fase 2 · spec 2 de 4. Define o núcleo puro `pairing::PairingPolicy` (que a
> spec 01 usa para autorizar conexões), o detector do botão e a tarefa que
> os liga ao hardware. Eventos na spec 03.

## 1. Objetivo

Implementar o controle físico de pareamento exigido pelo documento de
segurança (seção 5): segurar o botão **BOOT (GPIO0) por 5 s** derruba todas
as conexões, apaga todos os bonds e abre uma **janela de 45 s** em que o
dongle aceita **um** novo bond; fora da janela nenhum bond é criado e só o
celular já pareado é aceito. Tempos configuráveis por build flag.

## 2. Escopo

Dentro: leitura do GPIO com *debounce* e detecção de "segurar"; máquina de
estados da janela; tabela de conexões e regra de autorização; *timeouts* de
autenticação; derivação do modo de advertising; tarefa FreeRTOS; LED
opcional; build flags; ordem de inicialização.

Fora: chamadas NimBLE (spec 01), catálogo de eventos e roteiro de bancada
(spec 03), app (spec 04).

## 3. Estado atual do código (verificado)

| Item | Onde | Situação |
|---|---|---|
| GPIO | `esp32-firmware/src/**` | Nenhum uso de `pinMode`/`digitalRead`/`attachInterrupt`. GPIO0 livre. CAN usa 21/22 (TWAI) ou 18/19/23/5 (MCP2515). |
| Tarefas | `src/tasks/*.cpp`, `src/rtos/SerialEventLog.cpp`, `src/app/CollectorApp.cpp` | Aquisição: núcleo 1, prio 6, 8192 B. Flush: núcleo 1, prio 4, 6144 B. ELM327: sem afinidade (`xTaskCreate`), prio 5, 8192 B. Session: núcleo 0, prio 3, 4096 B. Upload: núcleo 0, prio 2, 6144 B. Logger: núcleo 0, prio 1, 4096 B. Pilha NimBLE (host): núcleo 0 (`CONFIG_BT_NIMBLE_PINNED_TO_CORE 0`), prio `configMAX_PRIORITIES - 4` (`nimble_port_freertos.c:40`). A Fase 1 pode ter acrescentado tarefas; conferir `CollectorApp.cpp` antes de implementar. |
| Utilitário de tarefas | `src/rtos/Adapters.h` (`rtos::startTask`) | Cria tarefa fixada em núcleo chamando `self->run()`. |
| Eventos | `src/collector/Events.h` | `collector::Event` (C++ puro, compila no `native`). `IEventSink` permite capturar linhas em teste (`test/test_events/test_main.cpp`, classe `CaptureSink`). |
| Testes nativos | `platformio.ini` `[env:native]` | `build_src_filter = +<can/isotp/IsoTpClient.cpp> +<collector/>`; dublês em `test/support/Fakes.h` (`FakeClock`). |
| Config local | `platformio_local.ini.example` | Redefine `[base] build_flags` — **substitui** o valor do `platformio.ini` (hoje vazio). Flags novos não podem morar em `[base]`, ou o arquivo local os apagaria. |

## 4. Design

### 4.1 Hardware: botão BOOT e LED

- GPIO0 no ESP32 DevKit: botão para GND com *pull-up* externo de 10 kΩ ⇒
  **ativo em nível baixo**. Configurar `pinMode(pin, INPUT_PULLUP)` mesmo
  assim (placas sem resistor externo).
- GPIO0 é pino de *strapping*: segurado durante o reset, o ESP32 entra em
  modo de download e o firmware nem roda. Consequência prática: o usuário
  deve apertar o botão **com o dongle já ligado**. Se o firmware subir com o
  botão pressionado (ex.: botão travado), o detector fica **bloqueado** até
  ver o botão solto (evita apagar bonds no boot por defeito mecânico).
- LED: o DevKitC v4 não tem LED de usuário; DevKits "DOIT" têm LED no
  GPIO2. Por isso o LED é opcional (`PAIR_LED_PIN=-1` desliga).

| Situação | LED (se `PAIR_LED_PIN >= 0`) |
|---|---|
| Botão pressionado, contando até o *hold* | pisca 5 Hz |
| Janela aberta | pisca 2 Hz |
| Sessão autorizada | aceso |
| Demais | apagado |

### 4.2 Detector do botão (`pairing::PairButton`)

Classe pura, alimentada por amostras `(rawPressed, nowMs)` a cada 20 ms.
*Debounce* por estabilidade: uma mudança só vale se o nível cru ficar
diferente do estável por `debounceMs` (padrão 50 ms ⇒ ~3 amostras).

| Estado | Entrada | Próximo | Saída (`Edge`) | Evento |
|---|---|---|---|---|
| `Init` | 1ª amostra solta | `Released` | `None` | — |
| `Init` | 1ª amostra pressionada | `Blocked` | `None` | `PAIR_BTN action=blocked held=0 fired=0` |
| `Blocked` | solto estável ≥ `debounceMs` | `Released` | `None` | — |
| `Released` | pressionado estável ≥ `debounceMs` | `Counting` (`pressStart` = início da instabilidade) | `Pressed` | `PAIR_BTN action=press held=0 fired=0` |
| `Counting` | `now − pressStart ≥ holdMs` | `Fired` | `HoldFired` | `PAIR_BTN action=hold held=<now−pressStart> fired=1` |
| `Counting` | solto estável ≥ `debounceMs` | `Released` | `Released` | `PAIR_BTN action=release held=<dur> fired=0` |
| `Fired` | continua pressionado | `Fired` | `None` (dispara **uma** vez por pressão) | — |
| `Fired` | solto estável ≥ `debounceMs` | `Released` | `Released` | `PAIR_BTN action=release held=<dur> fired=1` |

Comparações de tempo sempre como `static_cast<int32_t>(now - t) >= 0`
(seguras na virada do `millis()` em ~49 dias).

**[Decisão] D7 — Amostragem em tarefa, não ISR.** Com um gesto de 5 s, a
latência de 20 ms é irrelevante; a amostragem dispensa ISR + fila +
*debounce* com timer, é testável com relógio falso e não arrisca chamar
`collector::Event` (que usa mutex com `portMAX_DELAY`, `SerialEventLog.cpp:37`)
em contexto de interrupção. Custo: ~50 *wake-ups*/s de uma tarefa de prioridade 2.

### 4.3 Política (`pairing::PairingPolicy`)

Classe pura (sem NimBLE nem FreeRTOS), com duas partes: a **janela** e a
**tabela de conexões**. Cada entrada devolve um `Actions` com o que a camada
RTOS deve executar, **fora** do mutex, na ordem dos campos.

#### 4.3.1 Janela

| Estado | Evento | Condição | Próximo | Ações | Eventos |
|---|---|---|---|---|---|
| `Closed` (boot) | `begin(now, hasBond)` | — | `Closed` | `setBonding(false)`, advertising `Normal` | `PAIR_WINDOW state=close reason=boot deadline=- dur=-` |
| qualquer | `onButtonHold(now)` | — | `Wiping` | advertising `Off`; todas as conexões → `Rejecting(Button)`; `disconnectAll`; `deleteAllBonds` | — (o `PAIR_BTN hold` já saiu) |
| `Wiping` | `onBondsDeleted(ok=true, now)` | — | `Open`, `deadline = now + windowMs`, `consumed=false`, `hasBond=false`; toda conexão ainda `Pending` passa a `admittedInWindow=true` (cinto de segurança; ver `onLinkUp` em `Wiping`) | `setBonding(true)`, advertising `Pairing` | `PAIR_WINDOW state=open reason=button deadline=<ms> dur=-` |
| `Wiping` | `onBondsDeleted(ok=false, now)` | (o controlador já tentou 2×) | `Closed` | `setBonding(false)`, `recoverStore` (spec 01 §5.6: apaga o namespace `nimble_bond` e reinicia) | `PAIR_WINDOW state=close reason=error deadline=- dur=0` |
| `Open` | `tick(now)` | `now ≥ deadline` | `Closed` | `setBonding(false)`, advertising recalculado | `PAIR_WINDOW state=close reason=timeout deadline=- dur=<now−opened>` |
| `Open` | `onEncryptionChange` aceito como **bond novo** | — | `Closed`, `consumed=true`, `hasBond=true` | `setBonding(false)`, advertising recalculado | `PAIR_WINDOW state=close reason=bonded deadline=- dur=<now−opened>` |
| `Open` | `onButtonHold` | — | `Wiping` | igual à linha "qualquer" (reinicia: derruba inclusive quem acabou de parear) | — |
| `Closed` | `tick` | — | `Closed` | — | — |

Invariantes: `bondingAllowed() == (estado == Open)`; com a janela fechada
nenhum bond pode ser criado (spec 01 §5.2).

**[Decisão] D8 — Janela estrita.** Fechar a janela zera `sm_bonding` na
hora, mesmo com uma conexão admitida na janela ainda sem ter pedido
pareamento (ex.: usuário demorando no diálogo do iOS). Um "período de
graça" com `sm_bonding=1` permitiria que uma conexão admitida **depois** da
janela criasse bond, e aí bond-restaurado e bond-novo deixariam de ser
distinguíveis (a regra da tabela abaixo depende de "fora da janela não se
cria bond"). Pareamento já iniciado antes do fechamento termina com bond e
é aceito (`PAIR_RESULT late=1`).

**[Decisão] D9 — Sem janela automática no boot.** O documento manda abrir
pareamento só pelo botão; um dongle novo exige o gesto também ([Decisão] A5).

#### 4.3.2 Conexões

Tabela fixa de `kMaxLinks = 4` posições (≥ `CONFIG_BT_NIMBLE_MAX_CONNECTIONS = 3`):
`{conn, state, connectedAt, rejectAt, admittedInWindow, wasAuthorized}`.

`EncryptionInfo` traz, além de `status/encrypted/bonded/keySize`, o
resultado da leitura do bond no store feita pela spec 01
(`lesc` = `sc == 1`, `persisted` = LTK presente). As linhas são avaliadas
**em ordem**; vale a primeira cuja condição for verdadeira.

| Estado | Evento | Condição | Próximo | Ações | Eventos |
|---|---|---|---|---|---|
| `Free` | `onLinkUp(conn, now)` | janela em `Wiping` | `Rejecting` | `reject(conn, 0x09)` | `CONN_REJECT reason=busy dur=0` |
| `Free` | `onLinkUp(conn, now)` | já existe `Authorized` | `Rejecting` | `reject(conn, 0x09)` | `CONN_REJECT reason=busy dur=0` |
| `Free` | `onLinkUp(conn, now)` | — | `Pending`, `admittedInWindow = (janela Open)` | advertising recalculado (religa se houver slot) | — (`LINK_UP` é emitido pela spec 01) |
| `Pending` | `onEncryptionChange` | `status ≠ 0` ou `!encrypted` | `Rejecting` | `reject(conn, 0x05)` | `LINK_ENC up=0 reason=pairing\|other …`; se `status` é de pareamento (tabela abaixo), `PAIR_RESULT ok=0 reason=<classe>`; `CONN_REJECT reason=enc_fail` |
| `Pending` | `onEncryptionChange` | cifrado, `keySize < 16` | `Rejecting` | `reject(conn, 0x05)`; se `bonded`: `deleteBond(conn)` | `LINK_ENC up=1 …`; `CONN_REJECT reason=key_size` |
| `Pending` | `onEncryptionChange` | cifrado, **sem** bond | `Rejecting` | `reject(conn, 0x05)` | `LINK_ENC up=1 bonded=0 fresh=1`; `PAIR_RESULT ok=0 reason=window_closed` (janela fechada) ou `reason=not_bonded` (janela aberta, o celular não pediu bond); `CONN_REJECT reason=not_bonded` |
| `Pending` | `onEncryptionChange` | cifrado, com bond, `!lesc` | `Rejecting` | `reject(conn, 0x05)`; `deleteBond(conn)` | `PAIR_RESULT ok=0 reason=legacy` (se `admittedInWindow`); `CONN_REJECT reason=not_bonded`; `BOND_DELETE reason=policy` |
| `Pending` | `onEncryptionChange` | cifrado, com bond, `!admittedInWindow` | `Authorized` | `authorize(conn)`; advertising `Off` | `LINK_ENC up=1 bonded=1 fresh=0` (restauração) |
| `Pending` | `onEncryptionChange` | cifrado, com bond, `admittedInWindow`, `consumed` | `Rejecting` | `reject(conn, 0x09)`; `deleteBond(conn)` | `PAIR_RESULT ok=0 reason=busy`; `CONN_REJECT reason=busy` |
| `Pending` | `onEncryptionChange` | cifrado, com bond, `admittedInWindow`, `!persisted` | `Rejecting` | `reject(conn, 0x05)`; `deleteBond(conn)`; janela **não** é consumida | `BOND_VERIFY ok=0`; `PAIR_RESULT ok=0 reason=not_persisted`; `CONN_REJECT reason=enc_fail` |
| `Pending` | `onEncryptionChange` | cifrado, com bond, `admittedInWindow`, `!consumed`, `persisted` | `Authorized` | `authorize(conn)`; fecha a janela (linha "bond novo" acima); `hasBond=true` | `LINK_ENC up=1 bonded=1 fresh=1`; `PAIR_RESULT ok=1 reason=ok late=<janela já fechada?>` |
| `Pending` | `tick(now)` | `now − connectedAt ≥ (admittedInWindow ? pairAuthTimeoutMs : linkAuthTimeoutMs)` | `Rejecting` | `reject(conn, 0x05)` | `CONN_REJECT reason=auth_timeout dur=<…>` |
| `Authorized` | `onEncryptionChange` | `status = 0` e cifrado | `Authorized` | — (renovação de chave) | `LINK_ENC up=1 …` |
| `Authorized` | `onEncryptionChange` | falhou | `Rejecting` | `reject(conn, 0x05)` | `CONN_REJECT reason=enc_fail` |
| `Pending`/`Authorized` | `onButtonHold` | — | `Rejecting` (`wasAuthorized` preservado) | (dentro do `disconnectAll`) | `CONN_DROP reason=button count=<n>` (emitido pelo controlador) |
| `Rejecting` | `onEncryptionChange` | cifrado com `bonded=1` e `admittedInWindow` (pareamento que terminou depois da recusa, ex.: usuário tocou "Emparelhar" tarde) | `Rejecting` | `deleteBond(conn)` | `BOND_DELETE reason=policy` |
| `Rejecting` | `onEncryptionChange` | demais | `Rejecting` | — (ignorado) | — |
| `Rejecting` | `tick` | `now − rejectAt ≥ 2000` | `Rejecting` | `reject(conn, motivo)` de novo | — |
| qualquer ≠ `Free` | `onLinkDown(conn)` | — | `Free` | se `wasAuthorized`: `deauthorize`; advertising recalculado | — (`LINK_DOWN` é emitido pela spec 01) |
| `Free` | `onLinkDown`/`onEncryptionChange` de `conn` desconhecida | — | — | — (ignorado) | — |

Classe do `PAIR_RESULT reason` a partir do `status` do NimBLE (`ble_hs.h`):

| `status` | Classe | Observação |
|---|---|---|
| `0x500–0x5FF` (`BLE_HS_ERR_SM_PEER_BASE`) | `peer_reject` | O celular abortou (usuário tocou "Cancelar", pareamento não suportado). |
| `0x400–0x4FF` (`BLE_HS_ERR_SM_US_BASE`) | `local_reject` | O dongle recusou (ex.: parâmetros inválidos, chave curta). |
| `13` (`BLE_HS_ETIMEOUT`) | `timeout` | Timeout de 30 s do SMP (o host não derruba a conexão; a política recusa). |
| outros (`0x200+` HCI etc.) | `other` (sem `PAIR_RESULT`; `LINK_ENC up=0 reason=other`) | `rc` cru no evento. |

Restauração sem LTK (o celular tem bond, o dongle não — ex.: depois do
botão) **não gera `ENC_CHANGE`**: o host manda *negative reply* sem avisar a
aplicação (spec 01 §3.1). O dongle vê só o `LINK_UP`, escritas recusadas
pelo ATT e depois `CONN_REJECT reason=auth_timeout` ou a queda iniciada
pelo celular. Quem detecta "bond perdido" é o app.

Recusa de pareamento com bond gravado e janela fechada (spec 01 §5.6) também
não gera `ENC_CHANGE` (o SM responde *Pairing Failed* antes de alocar o
procedimento); aparece como `BOND_FULL event=full action=reject` e, depois,
`CONN_REJECT reason=auth_timeout`.

Por que a regra de autorização é correta (com o gate da spec 01 §5.2):

1. Fora da janela `sm_bonding = 0` (e, havendo bond, todo Pairing Request é
   recusado no `FULL`) ⇒ nenhum pareamento cria bond ⇒ um enlace cifrado
   **com** bond numa conexão admitida fora da janela só pode ser
   restauração de LTK que já estava no store.
2. Conexões que chegam enquanto os bonds são apagados (`Wiping`) são
   recusadas, e `sm_bonding` só liga depois do apagamento ⇒ nenhuma conexão
   "de fora da janela" consegue parear com bond.
3. Ao abrir a janela, todos os bonds foram apagados ⇒ dentro dela não há o
   que restaurar ⇒ bond numa conexão admitida na janela é pareamento novo.
4. `consumed` garante **um** bond por janela; `MAX_BONDS=1` e o
   `BondStoreGuard` garantem que um segundo não é gravado nem despeja o
   primeiro; bond de conexão recusada é apagado.
5. Só vale bond LESC (`sc == 1`, 16 B) confirmado no store.

#### 4.3.3 Advertising

`advMode()` é recalculado depois de cada entrada; se mudou (ou mudaram
`windowOpen`/`hasBond`), `Actions.setAdvertising = true`:

| Condição (em ordem) | Modo |
|---|---|
| Estado da janela = `Wiping` | `Off` |
| Existe `Authorized` | `Off` |
| Conexões ≠ `Free` ≥ `maxConnections` | `Off` |
| Janela `Open` | `Pairing` |
| senão | `Normal` |

### 4.4 Condições de corrida e casos de borda

| Situação | Comportamento resultante |
|---|---|
| Botão segurado durante um pareamento em andamento | Conexão cai (`0x13`), o SM aborta; bonds apagados; janela abre limpa. |
| Janela expira com Pairing Request já recebido | `sm_bonding` foi lido no pedido ⇒ termina com bond ⇒ aceito (`late=1`), janela já fechada. |
| Janela expira antes do Pairing Request (usuário lento no diálogo) | Pareia sem bond ⇒ recusado (`window_closed`). App orienta apertar o botão de novo (spec 04). |
| Usuário toca "Emparelhar" depois que a conexão já foi recusada por *timeout* | O pareamento pode terminar com bond numa conexão `Rejecting`; a política apaga esse bond (`BOND_DELETE reason=policy`) e a janela não é consumida. |
| Conexão chega entre o *hold* e o fim do apagamento (`Wiping`) | Recusada com `0x09`; sem isso ela seria admitida "fora da janela" e, com `sm_bonding` ligado logo depois, pareia com bond e seria tomada por restauração. |
| Dois celulares conectam na janela | O SM do NimBLE atende poucos pareamentos ao mesmo tempo (`BLE_SM_MAX_PROCS`). O primeiro bond fecha a janela; o segundo pareia sem bond ⇒ recusado, ou, se já tinha pedido bond, é recusado com `busy` e o bond dele é apagado. Se o atacante vencer, o dono vê falha no app e segura o botão de novo, o que derruba o atacante (comportamento previsto no documento). |
| Botão segurado e mantido | Dispara uma vez; nova ação só depois de soltar. |
| Dois *holds* seguidos | O segundo reinicia tudo, inclusive apagando o bond recém-criado. |
| Celular volta ao alcance antes do *supervision timeout* do enlace antigo | Advertising está desligado enquanto o antigo está `Authorized`; o celular só conecta depois que o antigo cai. Se conectar (corrida), recebe `0x09` (transitório). |
| `ENC_CHANGE` chega para conexão já `Rejecting` | Ignorado, exceto se criou bond (apagado, linha acima). |
| Falta de energia com a janela aberta | Boot com janela fechada; o que foi apagado continua apagado (NVS é síncrona). |
| `deleteAllBonds` falha | Controlador tenta de novo uma vez; se falhar, `BOND_DELETE ok=0`, `PAIR_WINDOW close reason=error` e recuperação: apaga o namespace `nimble_bond` e reinicia (spec 01 §5.6). Abrir a janela com bond residual seria inútil (com `MAX_BONDS=1` o pareamento novo nunca gravaria) e deixá-la fechada para sempre seria *lockout*. |
| NVS sem espaço ao gravar o bond novo | `BOND_VERIFY ok=0`, conexão recusada, janela continua aberta; o evento `NVS` do boot ajuda a diagnosticar. |
| Mais conexões que slots | O controlador NimBLE recusa no enlace; advertising já está `Off`. |
| Virada do `millis()` | Comparações por diferença com sinal. |

### 4.5 Camada RTOS

**`PairingController`** (núcleo nenhum; é chamado de duas tarefas):

- Guarda a `PairingPolicy` sob um mutex FreeRTOS (`xSemaphoreCreateMutex`,
  com herança de prioridade: a tarefa *host* do NimBLE tem prioridade 21 e
  a `PairingTask`, 2).
- Toda entrada: trava, chama a política, copia `Actions`, destrava,
  **executa** as ações chamando a interface `ILinkControl` (implementada pelo
  `BleConnectivity`). Executar fora do mutex evita segurar o nosso lock
  enquanto a NimBLE pega o `ble_hs_lock` (sem ciclo de locks).
- Ordem de execução das ações: advertising `Off` (se pedido) → `disconnectAll`
  (emite `CONN_DROP`) → `deleteAllBonds` (emite `BOND_DELETE`, até 2
  tentativas) → `policy.onBondsDeleted(ok)` (re-trava) → `deleteBond[]` →
  `reject[]` → `authorize`/`deauthorize` → `setBonding` → `setAdvertising`
  → `recoverStore` (por último; não retorna).
- Herança de prioridade não é transitiva no FreeRTOS: se a `PairingTask`
  segura o mutex do controlador e espera o mutex do logger (segurado pela
  tarefa do logger, prioridade 1), a tarefa *host* que espera o controlador
  depende do logger, que não herda prioridade. A seção crítica do logger é
  curta e sem E/S (`SerialEventLog.cpp:37-56`), então é latência limitada, não
  *deadlock*. Se a bancada mostrar atraso relevante no *host*, a política
  passa a acumular as linhas de evento no `Actions` e o controlador as emite
  depois de soltar o mutex.
- `isAuthorized()` não trava: o `BleConnectivity` mantém
  `std::atomic<uint16_t> _authorizedConn`, gravado por `authorize`/`deauthorize`.

**`PairingTask`**: núcleo 0, prioridade 2, pilha 4096 B, período 20 ms
(`vTaskDelayUntil`). Laço: lê o GPIO → `PairButton::update` → se
`HoldFired`, `controller.onButtonHold()` → `controller.tick()` → LED.

| Tarefa | Núcleo | Prioridade | Pilha | Período/gatilho |
|---|---|---|---|---|
| Host NimBLE (callbacks de conexão/cifra) | 0 | `configMAX_PRIORITIES − 4` (21) | da biblioteca (4096) | eventos do rádio |
| `PairingTask` (nova) | 0 | 2 | 4096 B | 20 ms |
| Upload / Session / Logger (existentes) | 0 | 2 / 3 / 1 | — | — |
| Aquisição / Flush (existentes) | 1 | 6 / 4 | — | — |

**[Decisão] D10 — Núcleo 0, prioridade 2.** O núcleo 1 é da coleta
(tempo real do barramento CAN); a `PairingTask` não pode atrasá-la. No
núcleo 0 ela fica abaixo da `SessionTask` (protocolo com o app) e divide a
prioridade com a `UploadTask` por *time slicing*; passa quase todo o tempo
bloqueada.

| Timer | Valor | Dono | Precisão |
|---|---|---|---|
| Amostragem do botão | 20 ms | `PairingTask` | ±1 tick |
| *Debounce* | `PAIR_DEBOUNCE_MS` (50) | `PairButton` | +20 ms |
| *Hold* | `PAIR_BUTTON_HOLD_MS` (5000) | `PairButton` | +20 ms |
| Janela | `PAIR_WINDOW_MS` (45000) | `PairingPolicy::tick` | +20 ms |
| Autenticação fora da janela | `LINK_AUTH_TIMEOUT_MS` (5000) | `PairingPolicy::tick` | +20 ms |
| Autenticação de conexão admitida na janela | `PAIR_AUTH_TIMEOUT_MS` (35000) | `PairingPolicy::tick` | +20 ms |

Sobre o `PAIR_AUTH_TIMEOUT_MS`: como o dongle não manda *Security Request*
a peer desconhecido (spec 01 D21), o timer de 30 s do SMP só começa quando o
celular envia o Pairing Request, que no iOS vem **depois** do toque em
"Emparelhar". O prazo real para o usuário decidir é, portanto, o da política
(35 s desde a conexão), e o app usa um prazo menor ou igual (spec 04).
| Reenvio de desconexão | 2000 ms | `PairingPolicy::tick` | +20 ms |

## 5. Configuração por build flags

### 5.1 Flags

Padrões em `src/connectivity/pairing/PairingConfig.h` (`#ifndef … #define`),
com `static_assert` de faixa; o `platformio.ini` repete os dois tempos
principais para ficarem visíveis e sobrescrevíveis.

| Flag | Padrão | Faixa aceita | Uso |
|---|---|---|---|
| `PAIR_BUTTON_PIN` | `0` | 0–39 | GPIO do botão (BOOT). |
| `PAIR_BUTTON_ACTIVE_LOW` | `1` | 0/1 | Nível ativo. |
| `PAIR_BUTTON_HOLD_MS` | `5000` | 1000–30000 | Tempo segurando para abrir a janela. |
| `PAIR_DEBOUNCE_MS` | `50` | 10–200 | *Debounce*. |
| `PAIR_WINDOW_MS` | `45000` | 10000–300000 | Duração da janela. |
| `PAIR_AUTH_TIMEOUT_MS` | `35000` | 10000–60000 | Prazo para cifrar, conexão admitida na janela (diálogo do SO + pareamento). |
| `LINK_AUTH_TIMEOUT_MS` | `5000` | 1000–30000 | Prazo para cifrar, conexão admitida fora da janela. |
| `PAIR_LED_PIN` | `-1` | −1 ou 0–33 | LED de status; −1 desliga. |
| `CONFIG_BT_NIMBLE_MAX_BONDS` | `1` | `== 1` | Um bond por vez (spec 01 §5.6). |
| `MYNEWT_VAL_BLE_SM_MAX_PROCS` | `3` | 1–3 | Procedimentos de segurança simultâneos no host (spec 01 §5.1; [verificar] em bancada). |

`platformio.ini` (acrescentar; a seção **não** é `[base]` para o arquivo
local não apagá-la ao redefinir `[base] build_flags`):

```ini
; ── Pareamento BLE (Fase 2) ───────────────────────────────────────────────────
; Botão BOOT (GPIO0) segurado PAIR_BUTTON_HOLD_MS abre a janela de pareamento
; por PAIR_WINDOW_MS. Para mudar só na sua máquina, redefina [pairing] no
; platformio_local.ini (mantendo CONFIG_BT_NIMBLE_MAX_BONDS=1).
[pairing]
build_flags =
    -DPAIR_BUTTON_HOLD_MS=5000
    -DPAIR_WINDOW_MS=45000
    -DCONFIG_BT_NIMBLE_MAX_BONDS=1
    -DMYNEWT_VAL_BLE_SM_MAX_PROCS=3
```

E em cada `[env:collector_mock]`, `[env:collector_twai]`,
`[env:collector_mcp2515]`: `build_flags = ${base.build_flags} ${pairing.build_flags} -DUSE_…`.
As variantes `_debug` já herdam via `${env:collector_*.build_flags}`.
O `[env:native]` não precisa: os testes constroem `Timings` explicitamente.

`platformio_local.ini.example` (acrescentar):

```ini
; Pareamento: tempos curtos para bancada (mantenha o MAX_BONDS).
; [pairing]
; build_flags =
;     -DPAIR_BUTTON_HOLD_MS=2000
;     -DPAIR_WINDOW_MS=20000
;     -DCONFIG_BT_NIMBLE_MAX_BONDS=1
;     -DMYNEWT_VAL_BLE_SM_MAX_PROCS=3
```

**[Decisão] D11 — Seções do PlatformIO em vez de variáveis do SO.** O pedido
fala em "variáveis de ambiente do PlatformIO": a forma robusta são os
ambientes/seções do `.ini`. `PLATFORMIO_BUILD_FLAGS` no SO **soma** flags e
geraria `-D` duplicado com valores diferentes (aviso de macro redefinida);
`${sysenv.X}` vazio gera `-DPAIR_WINDOW_MS=` e quebra a compilação. Os
padrões no header garantem que um `.ini` sem a seção ainda compila com
5000/45000.

## 6. Ordem de inicialização (`CollectorApp.cpp`)

Substituir o passo "6. BLE" por:

1. `static pairing::PairingController pairing(ble, timingsFromConfig(), CONFIG_BT_NIMBLE_MAX_CONNECTIONS);`
2. `ble.setPairingController(&pairing);` e `ble.setConnectionCallback(...)` (como hoje).
3. `static Elm327Task elm327Task(...)` (como hoje, registra o callback do RX).
4. `ble.begin()` — `NimBLEDevice::init`, `BleSecurity::configure()`, serviços
   e características com as flags novas. **Não** inicia o advertising.
5. `ble_security::BleSecurity::sanitizeBondsAtBoot();`
6. `pairing.begin();` — `policy.begin(now, bondCount() > 0)` ⇒
   `setBonding(false)` e advertising `Normal` com os dados de fabricante.
7. No passo "7. Tarefas": `static PairingTask pairingTask(pairing, PairingTaskConfig::fromBuildFlags()); eventLog.watch("pairing", pairingTask.start());`

## 7. Plano arquivo por arquivo

| Arquivo | Ação | Conteúdo |
|---|---|---|
| `esp32-firmware/src/connectivity/pairing/PairingConfig.h` | novo | Padrões e `static_assert` dos flags; `struct PairingTaskConfig { int pin; bool activeLow; uint32_t debounceMs, holdMs, periodMs; int ledPin; static PairingTaskConfig fromBuildFlags(); }`; `pairing::Timings timingsFromConfig();`. Só incluído pelo código ESP32. |
| `esp32-firmware/src/connectivity/pairing/PairButton.{h,cpp}` | novo | §4.2. C++ puro; inclui `collector/Events.h`. |
| `esp32-firmware/src/connectivity/pairing/PairingPolicy.{h,cpp}` | novo | §4.3. C++ puro; inclui `collector/Events.h`. |
| `esp32-firmware/src/connectivity/pairing/ILinkControl.h` | novo | Interface que o `BleConnectivity` implementa. |
| `esp32-firmware/src/connectivity/pairing/PairingController.{h,cpp}` | novo | §4.5 (FreeRTOS). |
| `esp32-firmware/src/tasks/PairingTask.{h,cpp}` | novo | §4.5 (Arduino GPIO + FreeRTOS). |
| `esp32-firmware/src/connectivity/ble/BleConnectivity.{h,cpp}` | alterar | Implementa `ILinkControl`; spec 01 §5.4/§7. |
| `esp32-firmware/src/app/CollectorApp.cpp` | alterar | §6. |
| `esp32-firmware/platformio.ini` | alterar | `[pairing]` §5.1; `[env:native] build_src_filter` ganha `+<connectivity/pairing/PairButton.cpp> +<connectivity/pairing/PairingPolicy.cpp>`. |
| `esp32-firmware/platformio_local.ini.example` | alterar | §5.1. |
| `esp32-firmware/src/tasks/README.md`, `esp32-firmware/src/connectivity/README.md` | alterar (doc) | Nova tarefa e a regra "dados só para a conexão autorizada". |
| `docs/rtos-freertos.md` | alterar (doc) | Linha da `PairingTask` na tabela de prioridades. |

Assinaturas:

```cpp
// PairButton.h
namespace pairing {
class PairButton {
public:
    enum class Edge : uint8_t { None, Pressed, Released, HoldFired };
    PairButton(uint32_t debounceMs, uint32_t holdMs);
    /** Uma amostra crua (true = pressionado). Emite PAIR_BTN nas transições. */
    Edge update(bool rawPressed, uint32_t nowMs);
    bool counting() const;   // pressionado, ainda sem disparar (LED 5 Hz)
    bool blocked() const;    // subiu pressionado; espera soltar
};
}  // namespace pairing
```

```cpp
// PairingPolicy.h
namespace pairing {

constexpr size_t   kMaxLinks = 4;
constexpr uint16_t kNoConn   = 0xFFFF;  // = BLE_HS_CONN_HANDLE_NONE

enum class AdvMode : uint8_t { Off, Normal, Pairing };
enum class WindowState : uint8_t { Closed, Wiping, Open };

struct Timings {
    uint32_t windowMs;
    uint32_t linkAuthTimeoutMs;
    uint32_t pairAuthTimeoutMs;
    uint32_t rejectRetryMs = 2000;
};

/** Espelho do NimBLEConnInfo + status do ENC_CHANGE (sem tipos da NimBLE). */
struct EncryptionInfo {
    int     status;      // 0 = ok; códigos BLE_HS_* da NimBLE
    bool    encrypted;
    bool    bonded;
    uint8_t keySize;
    bool    lesc;        // bond no store com sc == 1 (só faz sentido com bonded)
    bool    persisted;   // bond no store com LTK presente
};

struct Reject { uint16_t conn; uint8_t hciReason; };  // 0x05 ou 0x09

/** O que a camada RTOS executa, fora do mutex, nesta ordem. */
struct Actions {
    bool     advertisingOffFirst = false;
    bool     disconnectAll = false;
    bool     deleteAllBonds = false;
    uint16_t deleteBond[kMaxLinks] = {};   // apagar o bond do peer desta conexão
    uint8_t  deleteBondCount = 0;
    bool     recoverStore = false;          // apagar nimble_bond e reiniciar
    Reject   reject[kMaxLinks] = {};
    uint8_t  rejectCount = 0;
    uint16_t authorize = kNoConn;
    bool     deauthorize = false;
    bool     setBonding = false;
    bool     bondingAllowed = false;
    bool     setAdvertising = false;
    AdvMode  adv = AdvMode::Off;
    bool     windowOpen = false;
    bool     hasBond = false;
};

class PairingPolicy {
public:
    PairingPolicy(const Timings& timings, uint8_t maxConnections);

    void begin(uint32_t nowMs, bool hasBond, Actions& out);
    void onButtonHold(uint32_t nowMs, Actions& out);
    void onBondsDeleted(bool ok, uint32_t nowMs, Actions& out);
    void onLinkUp(uint16_t conn, uint32_t nowMs, Actions& out);
    void onEncryptionChange(uint16_t conn, const EncryptionInfo& info, uint32_t nowMs, Actions& out);
    void onLinkDown(uint16_t conn, uint32_t nowMs, Actions& out);
    void tick(uint32_t nowMs, Actions& out);

    WindowState window() const;
    uint32_t    windowDeadline() const;
    bool        bondingAllowed() const;
    bool        hasBond() const;
    uint16_t    authorizedConn() const;
    bool        isAuthorized(uint16_t conn) const;
    AdvMode     advMode() const;
    size_t      activeLinks() const;
};

}  // namespace pairing
```

```cpp
// ILinkControl.h
namespace pairing {
class ILinkControl {
public:
    virtual ~ILinkControl() = default;
    virtual size_t disconnectAll() = 0;                         // 0x13; devolve quantas
    virtual bool   deleteAllBonds(uint32_t& durMs) = 0;
    virtual void   deleteBond(uint16_t conn) = 0;                // bond do peer desta conexão
    [[noreturn]] virtual void recoverStore() = 0;               // apaga nimble_bond e reinicia
    virtual void   reject(uint16_t conn, uint8_t hciReason) = 0;
    virtual void   setAuthorized(uint16_t conn) = 0;            // liga APP_CONNECTED (CONN up=1)
    virtual void   clearAuthorized() = 0;                       // desliga (CONN up=0)
    virtual void   setBondingAllowed(bool allowed) = 0;
    virtual void   applyAdvertising(AdvMode mode, bool windowOpen, bool hasBond) = 0;
    virtual int    bondCount() = 0;
};
}  // namespace pairing
```

```cpp
// PairingController.h
namespace pairing {
class PairingController {
public:
    PairingController(ILinkControl& link, const Timings& timings, uint8_t maxConnections);
    void begin();                                                     // CollectorApp
    void onLinkUp(uint16_t conn);                                     // tarefa host NimBLE
    void onEncryptionChange(uint16_t conn, const EncryptionInfo& info);
    void onLinkDown(uint16_t conn);
    void onButtonHold();                                              // PairingTask
    void tick();
    bool windowOpen() const;                                          // LED
    bool authorized() const;                                          // LED
private:
    void execute(Actions actions);
    ILinkControl&     _link;
    PairingPolicy     _policy;
    SemaphoreHandle_t _mutex;
};
}  // namespace pairing
```

```cpp
// PairingTask.h
class PairingTask {
public:
    PairingTask(pairing::PairingController& controller, const PairingTaskConfig& config);
    TaskHandle_t start();   // rtos::startTask(this, "Pairing", 4096, 2, 0)
    void run();
private:
    pairing::PairingController& _controller;
    PairingTaskConfig           _config;
    pairing::PairButton         _button;
    void updateLed(uint32_t nowMs);
};
```

## 8. Testes

Testes nativos de `PairButton` e `PairingPolicy` (lista completa na spec 03
§3), com relógio manual e `CaptureSink` para conferir as linhas de evento.
Nada de FreeRTOS/NimBLE no `native`.

## 9. Passos ordenados com checkpoints

1. **`PairButton` + testes nativos.** *Checkpoint:* `pio test -e native -f test_pairing`
   verde para os casos BTN-* da spec 03 §3.1.
2. **`PairingPolicy` + testes nativos.** *Checkpoint:* casos POL-* da spec 03
   §3.2 verdes, incluindo "fora da janela nunca autoriza bond novo" e "um
   bond por janela".
3. **Flags e `PairingConfig.h`**; seção `[pairing]`; exemplo local.
   *Checkpoint:* build `collector_mock_debug`; um `-DPAIR_WINDOW_MS=5` no
   local falha no `static_assert`.
4. **`ILinkControl` no `BleConnectivity` + `PairingController`** (spec 01
   passos 2–5 juntos). *Checkpoint:* bancada B1, B6, B7.
5. **`PairingTask` + GPIO + LED** e ordem de inicialização. *Checkpoint:*
   bancada B2, B3, B5, B14; `STACK task=pairing free=` > 1024 após 10 min.
6. **Documentação** (`src/tasks/README.md`, `src/connectivity/README.md`,
   `docs/rtos-freertos.md`). *Checkpoint:* revisão.

## 10. Critérios de aceite

1. Segurar o BOOT 5 s (±0,1 s) com o dongle ligado gera, nesta ordem:
   `PAIR_BTN action=hold`, `ADV mode=off`, `CONN_DROP`, `BOND_DELETE ok=1`,
   `PAIR_WINDOW state=open`, `ADV mode=pairing`.
2. Soltar antes de 5 s não muda nada além de `PAIR_BTN press/release`.
3. Sem pareamento, a janela fecha em 45 s (±0,1 s) com `reason=timeout`.
4. O primeiro pareamento na janela fecha a janela com `reason=bonded` e
   autoriza a conexão; um segundo celular na mesma janela é recusado.
5. Com os flags do arquivo local (ex.: 2000/20000), os tempos medidos nos
   eventos acompanham.
6. Boot com o botão pressionado gera `PAIR_BTN action=blocked` e não apaga
   bonds.
7. Testes nativos da spec 03 verdes; nenhum teste nativo existente quebra.

## 11. Riscos

| # | Risco | Mitigação |
|---|---|---|
| R8 | Usuário segura o botão durante o reset e cai no modo download. | Documentar no manual de bancada; LED (se houver) só pisca com o firmware rodando. |
| R9 | Botão BOOT ruidoso/oxidado gera *hold* falso. | *Debounce* de 50 ms + exigência de 5 s contínuos; bloqueio no boot. |
| R10 | Janela de 45 s curta para o fluxo do iOS (esquecer nos Ajustes → voltar ao app → parear). | O app instrui esquecer **antes** de apertar o botão (spec 04); flag ajustável. |
| R11 | Mutex do controlador segurado enquanto emite eventos (mutex do logger). | Ordem de locks fixa (política → logger); o logger nunca chama a política. |

## 12. Decisões e pontos em aberto

- **[Decisão] D7** — Amostragem a 20 ms em tarefa, sem ISR (§4.2).
- **[Decisão] D8** — Janela estrita; pareamento já iniciado termina (§4.3.1).
- **[Decisão] D9** — Sem janela automática no boot (§4.3.1).
- **[Decisão] D10** — `PairingTask` no núcleo 0, prioridade 2 (§4.5).
- **[Decisão] D11** — Configuração por seção `[pairing]` do `.ini` (§5.1).
- **[Decisão] D12** — Botão pressionado no boot bloqueia o detector até soltar (§4.1).
- **[Decisão] A5** — **Não abrir a janela automaticamente** no primeiro boot
  (dongle sem nenhum bond); o documento exige o botão e o gesto é simples.
  Sem flag `PAIR_WINDOW_ON_FIRST_BOOT`.
- **[Decisão] A6** — LED: GPIO2 não é o padrão; deixar `-1`
  no repositório e ligar `-DPAIR_LED_PIN=2` no `platformio_local.ini` de quem
  tiver placa DOIT.
