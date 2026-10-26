# 03 — Firmware: eventos de depuração, testes nativos e roteiro de bancada

> Fase 2 · spec 3 de 4. Fecha o contrato de observabilidade das specs 01 e
> 02 (catálogo para `docs/debug-events.md`), a suíte nativa da política e do
> botão e o roteiro de validação em bancada (que também serve de base para a
> avaliação experimental do TCC).

## 1. Objetivo

1. Toda transição de segurança do enlace aparece no fluxo `EV n t NOME k=v`
   (formato de `docs/debug-events.md` §3), com `dur` sempre que houver tempo
   mensurável.
2. `PairButton` e `PairingPolicy` têm testes nativos (`pio test -e native`)
   que cobrem todas as transições das tabelas da spec 02, com relógio manual.
3. Um roteiro de bancada reprodutível (ESP32 DevKit + Android + iOS, sniffer
   opcional) prova os critérios de aceite da fase.

## 2. Catálogo de eventos novos

### 2.1 Regras gerais

- Formato, tipos e escapes: `docs/debug-events.md` §3 (sem mudança).
  Todos os eventos novos cabem no buffer pequeno (256 B) do `collector::Event`.
- `FORMAT v` **não** muda: os eventos são aditivos e o contrato manda os
  parsers ignorarem eventos desconhecidos. Se a Fase 1 já tiver subido a
  versão, manter a dela.
- `conn` é o *connection handle* da NimBLE (inteiro, igual em todos os
  eventos da mesma conexão; pode ser reutilizado depois de `LINK_DOWN`).
- Eventos emitidos na tarefa *host* da NimBLE seguem o precedente do `CONN`
  atual (emitido em callback): o custo é copiar a linha para o *ring buffer*.

### 2.2 Tabela (nova seção §5.9 "Pareamento e segurança do enlace")

| Evento | Campos (nesta ordem) | Fonte | Significado |
|---|---|---|---|
| `PAIR_BTN` | `action:enum(press,hold,release,blocked)` `held:int` `fired:bool` | `pairing::PairButton` | Transição do botão de pareamento. `press`: pressão confirmada pelo *debounce* (`held=0`). `hold`: atingiu `PAIR_BUTTON_HOLD_MS` (`held` = tempo real segurando, `fired=1`). `release`: soltou (`held` = duração total; `fired=1` se a pressão disparou). `blocked`: o firmware subiu com o botão pressionado; ignora até soltar. |
| `PAIR_WINDOW` | `state:enum(open,close)` `reason:enum(boot,button,timeout,bonded,error)` `deadline:ms\|-` `dur:int\|-` | `pairing::PairingPolicy` | Janela de pareamento. `open` (sempre `reason=button`): `deadline` = instante em que fecha. `close`: `dur` = tempo que ficou aberta (`-` em `reason=boot`, estado inicial). `bonded`: fechou porque um celular pareou. `error`: não abriu porque apagar os bonds falhou. |
| `BOND_DELETE` | `reason:enum(button,policy,recover)` `count:int` `ok:bool` `attempts:int` `dur:int` | `PairingController` / `BleSecurity` | Bonds apagados. `count` = quantos foram apagados. `policy`: higiene no boot (mais de um bond ou bond legado) ou bond de conexão recusada (legado, segundo bond na janela, pareamento que terminou depois da recusa). `recover`: apagamento do namespace inteiro antes de reiniciar (falha do `button`). `dur` = tempo gasto (escrita na NVS). |
| `BOND_VERIFY` | `conn:int` `ok:bool` `lesc:bool` `persisted:bool` | `PairingPolicy` | Conferência do bond novo no store logo após o pareamento. `ok=0` ⇒ conexão recusada e janela mantida. |
| `NVS` | `free:int` `used:int` `total:int` | `BleSecurity::sanitizeBondsAtBoot` | Entradas da partição `nvs` (`nvs_get_stats`) no boot; a partição é dividida entre bonds e coleta. |
| `CONN_DROP` | `reason:enum(button)` `count:int` | `PairingController` | Todas as conexões foram derrubadas (`0x13`) para um novo pareamento. Cada uma ainda gera seu `LINK_DOWN`. |
| `ADV` | `mode:enum(off,normal,pairing)` `window:bool` `bonded:bool` | `BleConnectivity::applyAdvertising` | Advertising (re)configurado; só quando muda algo. `window`/`bonded` = bits dos dados de fabricante. |
| `LINK_UP` | `conn:int` `addr:hex` `atype:enum(pub,rnd)` `window:bool` | `BleConnectivity::onConnect` | Conexão de enlace aceita (ainda **não** autorizada). `addr` = endereço no ar (6 bytes, ordem de exibição da NimBLE, sem `:`); em celulares é um RPA que muda. `window=1` se admitida com a janela aberta. |
| `LINK_ENC` | `conn:int` `up:bool` `bonded:bool` `fresh:bool` `key:int` `reason:enum(ok,pairing,other)` `rc:int` `dur:int` | `pairing::PairingPolicy` | Resultado de cifração (`BLE_GAP_EVENT_ENC_CHANGE`). `up=1`: cifrado. `fresh=1`: chave nova (pareamento), `0`: LTK restaurada. `key` = tamanho da chave em bytes. `reason=ok` sempre que `up=1`; com `up=0`: `pairing` = o pareamento falhou (detalhe no `PAIR_RESULT`), `other` = demais (ex.: erro HCI na cifração). Restauração sem LTK no dongle **não** gera este evento (a NimBLE não emite `ENC_CHANGE` nesse caso; spec 02 §4.3.2). `rc` = status NimBLE cru. `dur` = `t − LINK_UP.t`. |
| `PAIR_RESULT` | `conn:int` `ok:bool` `reason:enum(ok,window_closed,not_bonded,legacy,not_persisted,busy,peer_reject,local_reject,timeout,other)` `late:bool` `rc:int` `dur:int` | `pairing::PairingPolicy` | Fim de uma tentativa de **pareamento** que chegou ao SM (não emitido em restauração nem em pedido recusado no `BOND_FULL`). `ok=1` só com bond LESC criado na janela e confirmado no store. `legacy`: o central pareou sem LESC (bond apagado). `not_persisted`: bond não gravado na NVS. `late=1`: começou com a janela aberta e terminou depois. `dur` = `t − LINK_UP.t` = **tempo de pareamento** (inclui o diálogo do SO). |
| `CONN_REJECT` | `conn:int` `reason:enum(not_bonded,enc_fail,key_size,auth_timeout,busy)` `hci:hex` `dur:int` | `pairing::PairingPolicy` | O dongle derrubou a conexão. `hci` = motivo enviado (`05` recusa, `09` ocupado). `dur` = tempo conectado até a recusa. |
| `LINK_DOWN` | `conn:int` `rc:int` `hci:hex\|-` `auth:bool` `dur:int` | `BleConnectivity::onDisconnect` | Enlace caiu. `rc` = motivo NimBLE cru; `hci` = `rc − 0x200` quando é erro HCI (ex.: `13` remoto, `16` local, `08` *timeout*). `auth=1` se era a sessão autorizada. `dur` = tempo de conexão. |
| `WRITE_DROP` | `conn:int` `chr:enum(nus_rx,sync_rx)` `len:int` | `BleConnectivity::onWrite` / classe do SYNC | Escrita recebida de conexão não autorizada e descartada. |
| `BOND_FULL` | `event:enum(full,overflow)` `action:enum(reject,allow)` `obj:int` `conn:int\|-` `window:bool` | `ble_security::BondStoreGuard` | `full`: chegou um Pairing Request com o store cheio (com `MAX_BONDS=1`, todo pedido havendo bond); `reject` fora da janela (o celular recebe *Pairing Failed* e o bond existente fica intacto), `allow` dentro dela. `overflow`: tentativa de gravar um segundo bond; sempre `reject`, sem apagar o existente. `obj` = tipo NimBLE (`1` our_sec, `2` peer_sec). |

Mudanças em eventos existentes:

| Evento | Mudança |
|---|---|
| `CONN up` (§5.7) | Mesmos campos; passa a significar **sessão autorizada** (enlace cifrado com bond válido) aberta/fechada. Uma conexão recusada nunca gera `CONN`. |
| `STACK task` (§5.8) | Enum ganha `pairing`. |

### 2.3 Receitas para o visualizador (nova §6.6)

- **Tempo de pareamento:** `PAIR_RESULT ok=1 dur`. Tempo desde o botão:
  `PAIR_RESULT.t − PAIR_BTN(action=hold).t`.
- **Tempo de reconexão com bond:** `LINK_ENC fresh=0 up=1 dur` (enlace →
  cifrado); até a sessão: `CONN(up=1).t − LINK_UP.t` da mesma conexão.
- **Uso da janela:** pares `PAIR_WINDOW open`/`close`; `reason=timeout`
  sem `PAIR_RESULT ok=1` no intervalo = janela desperdiçada.
- **Tentativas indevidas:** `CONN_REJECT` por `reason`; `WRITE_DROP` por
  `conn` (cliente insistindo sem autorização).
- **Disponibilidade (A1.5):** soma de `dur` dos `LINK_DOWN auth=0` = tempo em
  que estranhos ocuparam slots.

### 2.4 Exemplos (para `docs/debug-events.md` §8)

Pareamento novo pelo botão (Android):

```
EV 120 30112 PAIR_BTN action=press held=0 fired=0
EV 121 35113 PAIR_BTN action=hold held=5021 fired=1
EV 122 35113 ADV mode=off window=0 bonded=1
EV 123 35114 CONN_DROP reason=button count=1
EV 124 35131 BOND_DELETE reason=button count=1 ok=1 attempts=1 dur=17
EV 125 35131 PAIR_WINDOW state=open reason=button deadline=80131 dur=-
EV 126 35132 ADV mode=pairing window=1 bonded=0
EV 127 35190 LINK_DOWN conn=1 rc=534 hci=16 auth=1 dur=812345
EV 128 35191 CONN up=0
EV 129 35905 PAIR_BTN action=release held=5792 fired=1
EV 130 41220 LINK_UP conn=2 addr=5A1F3C2B9D44 atype=rnd window=1
EV 131 44877 LINK_ENC conn=2 up=1 bonded=1 fresh=1 key=16 reason=ok rc=0 dur=3657
EV 132 44877 BOND_VERIFY conn=2 ok=1 lesc=1 persisted=1
EV 133 44877 PAIR_RESULT conn=2 ok=1 reason=ok late=0 rc=0 dur=3657
EV 134 44877 PAIR_WINDOW state=close reason=bonded deadline=- dur=9746
EV 135 44877 ADV mode=off window=0 bonded=1
EV 136 44878 CONN up=1
```

Reconexão com bond (após reboot do dongle):

```
EV 40 2210 LINK_UP conn=1 addr=6B02AA17C0E1 atype=rnd window=0
EV 41 2398 LINK_ENC conn=1 up=1 bonded=1 fresh=0 key=16 reason=ok rc=0 dur=188
EV 42 2398 ADV mode=off window=0 bonded=1
EV 43 2399 CONN up=1
```

Estranho fora da janela, dongle **com** bond (pedido de pareamento recusado
no store; o bond existente fica intacto):

```
EV 280 70100 LINK_UP conn=2 addr=4C9E01D2A7B3 atype=rnd window=0
EV 281 71620 BOND_FULL event=full action=reject obj=2 conn=2 window=0
EV 282 75100 CONN_REJECT conn=2 reason=auth_timeout hci=05 dur=5000
EV 283 75140 LINK_DOWN conn=2 rc=534 hci=16 auth=0 dur=5040
```

Estranho fora da janela, dongle **sem** bond (pareia sem bond e é derrubado):

```
EV 300 90100 LINK_UP conn=2 addr=4C9E01D2A7B3 atype=rnd window=0
EV 301 93950 LINK_ENC conn=2 up=1 bonded=0 fresh=1 key=16 reason=ok rc=0 dur=3850
EV 302 93950 PAIR_RESULT conn=2 ok=0 reason=window_closed late=0 rc=0 dur=3850
EV 303 93950 CONN_REJECT conn=2 reason=not_bonded hci=05 dur=3850
EV 304 93990 LINK_DOWN conn=2 rc=534 hci=16 auth=0 dur=3890
```

## 3. Testes nativos

Pasta nova `esp32-firmware/test/test_pairing/test_main.cpp` (Unity), com
`setUp` instalando um sink de captura. Extrair a classe `CaptureSink` de
`test/test_events/test_main.cpp` para `test/support/CaptureSink.h` e usá-la
nos dois (refatoração mecânica, sem mudar os testes existentes).
`platformio.ini [env:native]` ganha
`+<connectivity/pairing/PairButton.cpp> +<connectivity/pairing/PairingPolicy.cpp>`
no `build_src_filter` (spec 02 §7). Os fontes da pasta `pairing/` só podem
incluir `collector/Events.h` e a biblioteca padrão.

Relógio: variável `uint32_t now` manipulada pelo teste (os métodos recebem
`nowMs`); helper `void feed(PairButton&, bool pressed, uint32_t fromMs, uint32_t toMs, uint32_t step = 20)`.

### 3.1 `PairButton` (debounce 50 ms, hold 5000 ms)

| Caso | Roteiro | Esperado |
|---|---|---|
| BTN-01 | 1ª amostra solta; 1 s solto | Só `None`; nenhum evento. |
| BTN-02 | Pulso pressionado de 30 ms | Nenhuma borda (ruído abaixo do *debounce*). |
| BTN-03 | Pressionado estável 100 ms | Um `Pressed`; `PAIR_BTN action=press held=0 fired=0`. |
| BTN-04 | Pressionado 5100 ms em passos de 20 ms | Exatamente um `HoldFired`; `held` em [5000, 5020]. |
| BTN-05 | Pressionado 3000 ms e solta | `Released`, `fired=0`, `held` ≈ 3000 (±70). Nenhum `HoldFired`. |
| BTN-06 | Pressionado 12 s | Um único `HoldFired`. |
| BTN-07 | Hold disparado e solta | `release ... fired=1`. |
| BTN-08 | 1ª amostra pressionada; segue 10 s pressionado; solta; pressiona 5 s | `blocked` no início; nenhum `HoldFired` nos 10 s; depois um `HoldFired`. |
| BTN-09 | Soltura com repique (pulsos de 10 ms por 40 ms) | Um único `Released`. |
| BTN-10 | `now` começa em `0xFFFFF000` e atravessa a virada | `HoldFired` em 5000 ms (comparação com sinal). |
| BTN-11 | Com hold 2000 ms (construtor) | Dispara em 2000 ms (o valor não está fixo no código). |

### 3.2 `PairingPolicy` (`Timings{45000, 5000, 35000}`, `maxConnections = 3`)

Convenção: `enc(bonded, key=16, status=0)` monta `EncryptionInfo{status, true, bonded, key}`.

| Caso | Roteiro | Esperado |
|---|---|---|
| POL-01 | `begin(now, hasBond=false)` | `Closed`; `setBonding(false)`; `adv=Normal window=0 bonded=0`; `PAIR_WINDOW state=close reason=boot`. |
| POL-02 | `begin(hasBond=true)` | `adv=Normal bonded=1`. |
| POL-03 | Conexão autorizada (restauração) + `onButtonHold` | `advertisingOffFirst`, `disconnectAll`, `deleteAllBonds`; `isAuthorized(c)==false` **imediatamente**; estado `Wiping`. |
| POL-04 | POL-03 + `onBondsDeleted(true, t)` | `Open`, `deadline=t+45000`; `setBonding(true)`; `adv=Pairing window=1 bonded=0`; `PAIR_WINDOW open`. |
| POL-05 | POL-04 + `onLinkDown(c)` da antiga | `deauthorize=true` (a sessão antiga é encerrada para a coleta). |
| POL-06 | Janela aberta; `tick(t+44999)`; `tick(t+45000)` | Nada; depois `close reason=timeout dur=45000`, `setBonding(false)`, `adv=Normal`. |
| POL-07 | Janela aberta; `onLinkUp(c)`; `onEncryptionChange(c, enc(bonded=1))` | `authorize=c`; `PAIR_RESULT ok=1 late=0`; `PAIR_WINDOW close reason=bonded`; `setBonding(false)`; `adv=Off`; `hasBond()`. |
| POL-08 | Fechada com bond; `onLinkUp(c)`; `enc(bonded=1)` | `authorize=c`; `LINK_ENC fresh=0`; **nenhum** `PAIR_RESULT`. |
| POL-09 | Fechada; `onLinkUp(c)`; `enc(bonded=0)` | `reject{c,0x05}`; `PAIR_RESULT ok=0 reason=window_closed`; `CONN_REJECT reason=not_bonded`. |
| POL-10 | Aberta; `onLinkUp(c)`; `enc(bonded=0)` | `PAIR_RESULT reason=not_bonded`; recusa; janela **continua** aberta. |
| POL-11 | Fechada; `onLinkUp(c)` em t; `tick(t+4999)`; `tick(t+5000)` | Nada; depois `reject{c,0x05}`, `CONN_REJECT reason=auth_timeout dur=5000`. |
| POL-12 | Aberta; `onLinkUp(c)` em t; janela fecha em t+10000; `tick(t+35000)` | `auth_timeout` só em t+35000. |
| POL-13 | Aberta; `onLinkUp(c)`; janela fecha por timeout; `enc(bonded=1)` | Aceito: `PAIR_RESULT ok=1 late=1`; janela já estava fechada (sem segundo `PAIR_WINDOW`). |
| POL-14 | Aberta; `onLinkUp(a)`, `onLinkUp(b)`; `enc(a, bonded=1)`; `enc(b, bonded=1)` | `a` autorizado; `b` recusado `0x09`, `PAIR_RESULT ok=0 reason=busy`. |
| POL-15 | Autorizada existe; `onLinkUp(d)` | `reject{d,0x09}` imediato, `CONN_REJECT reason=busy`. |
| POL-16 | `enc(status=0x508)`, `enc(status=0x403)`, `enc(status=13)` em conexões pendentes | `PAIR_RESULT reason=peer_reject` / `local_reject` / `timeout`; todas recusadas `enc_fail`. |
| POL-17 | Fechada; `enc(status=0x206)` (erro HCI na cifração) | `LINK_ENC up=0 reason=other`; **sem** `PAIR_RESULT`; `CONN_REJECT reason=enc_fail`. |
| POL-18 | `enc(bonded=1, key=7)` | `CONN_REJECT reason=key_size`. |
| POL-19 | Autorizada; `enc(status=0)` de novo (renovação) | Continua autorizada; nenhuma ação. |
| POL-20 | 3 conexões pendentes | `adv=Off`; ao cair uma, `adv=Normal`. |
| POL-21 | `enc` para conexão `Rejecting`; `onLinkDown` de handle desconhecido | Ignorados (sem ações, sem eventos). |
| POL-22 | Conexão `Rejecting` sem `onLinkDown` por 2000 ms | `reject` reemitido. |
| POL-23 | `onButtonHold`; `onBondsDeleted(false)` | `PAIR_WINDOW close reason=error`; `setBonding(false)`; `recoverStore=true`; janela não abre. |
| POL-24 | Janela aberta; `onButtonHold` de novo | Volta a `Wiping`; nova janela com novo `deadline` depois do `onBondsDeleted`. |
| POL-25 | Propriedade (sequência pseudoaleatória com semente fixa, 10 000 passos, mistura de todos os eventos e `tick`s; o teste modela o SM: `bonded` só pode ser `true` num pareamento se `bondingAllowed()` era `true` quando a conexão "pediu" pareamento, ou se havia bond antes) | Invariantes: (1) `bondingAllowed() ⇒ window()==Open`; (2) no máximo um `PAIR_RESULT ok=1` entre dois `onBondsDeleted(true)`; (3) no máximo uma conexão autorizada; (4) `advMode()==Off` sempre que há autorizada. |
| POL-26 | `now` atravessando `0xFFFFFFFF` com janela aberta | Fecha em 45000 ms. |
| POL-27 | `onButtonHold`; `onLinkUp(c)` (em `Wiping`); `onBondsDeleted(true)`; `enc(c, bonded=1)` | `onLinkUp` já recusa com `0x09` (`CONN_REJECT reason=busy`); o `enc` posterior não autoriza. Variante: conexão `Pending` admitida antes do *hold* que escapou do `disconnectAll` passa a `admittedInWindow=true` no `onBondsDeleted`. |
| POL-28 | Aberta; `onLinkUp(c)`; `enc(bonded=1, lesc=false)` | `reject{c,0x05}`; `deleteBond(c)`; `PAIR_RESULT ok=0 reason=legacy`; janela continua aberta. |
| POL-29 | Aberta; `onLinkUp(c)`; `enc(bonded=1, persisted=false)` | `reject`, `deleteBond(c)`; `BOND_VERIFY ok=0`; `PAIR_RESULT reason=not_persisted`; janela aberta e não consumida. |
| POL-30 | Aberta; `onLinkUp(c)` em t; `tick(t+35000)` (recusa); `enc(c, bonded=1)` | `deleteBond(c)`, `BOND_DELETE reason=policy`; sem autorização; janela não consumida. |
| POL-31 | POL-14 com a segunda conexão já tendo bond | `deleteBond(b)` junto com o `reject{b,0x09}`; o bond de `a` não é tocado. |

## 4. Roteiro de bancada

### 4.1 Material

- ESP32 DevKit (botão BOOT no GPIO0), firmware `collector_mock_debug`
  (sem carro) ou `collector_twai_debug`, serial a 921600 baud capturada em
  arquivo (`pio device monitor -e collector_mock_debug | tee bancada.log`).
- Android ≥ 10 com o app (spec 04) e **nRF Connect**.
- iPhone com o app e **LightBlue** (ou nRF Connect iOS).
- Opcional: sniffer **nRF52840 Dongle + nRF Sniffer for Bluetooth LE** no
  Wireshark; `crackle` para a prova negativa.
- Antes da primeira gravação deste firmware: `pio run -e <env> -t erase`
  (zera bonds de firmwares antigos) e, nos celulares, esquecer o dongle.

### 4.2 Cenários

| # | Cenário | Passos | Eventos esperados | App esperado | Evidência |
|---|---|---|---|---|---|
| B1 | Boot sem bond | Ligar o dongle | `PAIR_WINDOW close reason=boot`; `ADV mode=normal window=0 bonded=0` | Busca mostra o dongle "aguardando pareamento" | log |
| B2 | Toque curto | Segurar BOOT 3 s | `PAIR_BTN press` / `release held≈3000 fired=0` | — | log |
| B3 | Abrir janela | Segurar 5 s | sequência da spec 02 §10 item 1 | Busca mostra "pronto para parear" | log + foto do LED (se houver) |
| B4 | Parear Android | B3; no app, tocar no dongle | `LINK_UP window=1` → `LINK_ENC fresh=1` → `PAIR_RESULT ok=1` → `PAIR_WINDOW close reason=bonded` → `CONN up=1` | Diálogo do sistema; depois "Conectado"; painel lê RPM | log + captura de tela; anotar `PAIR_RESULT dur` |
| B5 | Janela expira | B3 e não parear | `PAIR_WINDOW close reason=timeout dur≈45000` | Tela de espera volta a "aperte o botão" | log |
| B6 | Estranho fora da janela | (a) Com B4 feito, outro celular (nRF Connect) conecta e pede *bond*; (b) repetir depois de B5 sem nenhum bond no dongle | (a) `BOND_FULL event=full action=reject`, `CONN_REJECT reason=auth_timeout`; o primeiro celular **continua** com bond. (b) `PAIR_RESULT ok=0 reason=window_closed`, `CONN_REJECT reason=not_bonded` | nRF Connect: pareamento falha/desconecta | log; depois reconectar o celular de B4 sem novo pareamento |
| B7 | Acesso sem pareamento | nRF Connect conecta **sem** parear: escrever `010C\r` no RX com resposta, sem resposta, e inscrever no TX | Escrita com resposta: erro 0x05 (*Insufficient Authentication*) na tela; sem resposta: nenhum `CMD` no log; inscrição: erro 0x05; `CONN_REJECT reason=auth_timeout` em 5 s | — | prints do nRF Connect + log |
| B8 | Reconexão Android | Reiniciar o dongle (reset) com o app aberto | `LINK_ENC fresh=0 up=1` (sem `PAIR_RESULT`), `CONN up=1` | Reconecta sozinho, sem diálogo | log; anotar `LINK_ENC dur` |
| B9 | Parear e reconectar iOS | Esquecer bond do Android não é preciso; B3; parear o iPhone; resetar o dongle | Como B4 e B8 | iOS mostra "Solicitação de Emparelhamento"; depois reconecta sem diálogo | log |
| B10 | Botão com Android conectado | Com sessão Android ativa, segurar 5 s | `CONN_DROP`, `BOND_DELETE`, `PAIR_WINDOW open`; Android tenta reconectar: `LINK_UP` sem `LINK_ENC` (o dongle não tem a LTK; a NimBLE não avisa) e depois `CONN_REJECT reason=auth_timeout` ou `LINK_DOWN` pedido pelo celular; em seguida o pareamento novo (`PAIR_RESULT ok=1`) | App detecta bond perdido, remove o bond do Android e pareia de novo (janela aberta) | log + vídeo curto |
| B11 | Botão com iPhone conectado | Idem com iOS | Idem | App mostra instruções "Esquecer este dispositivo" nos Ajustes | log + print |
| B12 | Sniffer | Capturar B4 e um uso do painel; repetir com o firmware anterior à fase (tag/commit sem LESC) | Na Fase 2: *Pairing Request* com `AuthReq` `SC=1`, `Bonding`; PDUs *Pairing Public Key*; `LL_ENC_REQ`/`LL_START_ENC_REQ`; depois só PDUs cifrados (Wireshark não decodifica ATT). No firmware antigo: `010C` e `41 0C …` legíveis | — | `.pcapng` das duas capturas; `crackle -i captura.pcapng` não recupera chave (LESC) |
| B13 | DoS de conexões | nRF Connect em outro aparelho conectando/desconectando em laço; enquanto isso, resetar o dongle com o celular pareado por perto | `CONN_REJECT` repetidos do estranho; o pareado obtém `CONN up=1` (adv continua com conexões pendentes) | Reconecta em < 10 s | log; medir tempo até `CONN up=1` |
| B14 | Falta de energia na janela | B3; desligar a alimentação; religar | Boot com `PAIR_WINDOW close reason=boot`, `ADV ... bonded=0` | Celular antigo não reconecta (bond apagado) → app orienta | log |
| B15 | Usuário cancela | B3; no diálogo do sistema tocar "Cancelar" | `PAIR_RESULT ok=0 reason=peer_reject` | "Pareamento recusado" | log |
| B16 | Central sem LESC (opcional) | Um central BLE 4.0/4.1 ou script com pareamento legado | O pareamento legado completa e só então é recusado: `PAIR_RESULT ok=0 reason=legacy`, `BOND_DELETE reason=policy` | — | log |

### 4.3 Métricas para o TCC

Repetir B4/B8 em Android e iOS, N = 10 cada, e tabular média e p95 de:
`PAIR_RESULT dur` (pareamento), `LINK_ENC dur` com `fresh=0` (restauração),
`CONN(up=1).t − LINK_UP.t` (até a sessão). Comparar com o tempo de conexão
do firmware anterior à fase (sem cifra) para o cenário C0×C1 da
`docs/seguranca-ble-modelagem-ameacas.md` §7.

## 5. Plano arquivo por arquivo

| Arquivo | Ação |
|---|---|
| `docs/debug-events.md` | Nova §5.9 (tabela §2.2), ajuste de `CONN` e `STACK`, nova §6.6 (§2.3), exemplos no §8 (§2.4). |
| `esp32-firmware/test/support/CaptureSink.h` | Novo (extraído de `test_events`). |
| `esp32-firmware/test/test_events/test_main.cpp` | Passa a incluir `support/CaptureSink.h`. |
| `esp32-firmware/test/test_pairing/test_main.cpp` | Novo: BTN-01…11, POL-01…31. |
| `esp32-firmware/platformio.ini` | `build_src_filter` do `native` (spec 02 §7). |

## 6. Passos e checkpoints

1. Extrair `CaptureSink`. *Checkpoint:* `pio test -e native` verde, sem
   mudança de resultado.
2. Escrever BTN-* junto com a spec 02 passo 1. *Checkpoint:* verde.
3. Escrever POL-* junto com a spec 02 passo 2. *Checkpoint:* verde,
   incluindo POL-25 com 10 000 passos em < 1 s.
4. Atualizar `docs/debug-events.md`. *Checkpoint:* cada evento emitido no
   código aparece na tabela com os mesmos campos e ordem (conferir com
   `grep -rn 'Event("' esp32-firmware/src/connectivity`).
5. Bancada B1–B15 (B16 opcional), anexando logs/capturas em
   `docs/bancada/fase-2-lesc/` (pasta nova, só artefatos).

## 7. Critérios de aceite

1. Todos os eventos das specs 01/02 documentados e emitidos com os campos
   na ordem da tabela; `dur` presente em `BOND_DELETE`, `PAIR_WINDOW close`,
   `LINK_ENC`, `PAIR_RESULT`, `CONN_REJECT`, `LINK_DOWN`.
2. `pio test -e native` verde com os testes novos e os antigos.
3. B1–B15 executados com os eventos e comportamentos esperados, nos dois
   sistemas operacionais.

## 8. Riscos

| # | Risco | Mitigação |
|---|---|---|
| R12 | Rajada de eventos de um atacante enchendo o *ring buffer* do logger. | O logger já descarta com `LOG_DROP`; a coleta não é afetada. |
| R13 | Sniffer perde o pareamento (salto de canal) e a captura não mostra a troca de chaves. | Repetir; iniciar a captura seguindo o anúncio do dongle antes de conectar. |

## 9. Decisões e pontos em aberto

- **[Decisão] D13** — `FORMAT v` não muda: eventos aditivos (§2.1).
- **[Decisão] D14** — `CONN` passa a significar sessão autorizada, sem
  renomear (o visualizador e a coleta já tratam `CONN` como "app presente").
- **[Decisão] D15** — `LINK_UP` registra o endereço no ar do celular (RPA)
  para correlacionar tentativas; não identifica a pessoa (RPA muda a cada
  ~15 min).
- **[Aberto] A7** — Guardar os artefatos de bancada no repositório
  (`docs/bancada/…`) ou só no TCC? *Recomendação:* logs de texto no
  repositório (pequenos, úteis para o visualizador); `.pcapng` fora do git.
