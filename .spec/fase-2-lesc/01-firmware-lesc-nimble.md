# 01 — Firmware: segurança de enlace LESC no NimBLE

> Fase 2 · spec 1 de 4. Depende do núcleo puro `pairing::PairingPolicy`
> (spec 02, §4) para a decisão de autorização. Os eventos citados aqui estão
> catalogados na spec 03. Revisada após o advisor (ver `README.md`).

## 1. Objetivo

Fazer o dongle aceitar **somente** enlaces BLE cifrados por uma chave de
longo prazo (LTK) obtida em pareamento **LE Secure Connections (LESC) Just
Works** e guardada como **bond**, e só entregar dados das características NUS
e SYNC a uma conexão **autorizada** (cifrada + com bond LESC válido). Fecha
as ameaças A1.1 (sniffing), A1.2 (replay), A1.4 (acesso direto) do documento
de segurança e reduz A1.3 (MITM) e A1.5 (DoS) junto com a janela de
pareamento (spec 02).

## 2. Escopo

Dentro:

- Configuração do Security Manager do NimBLE (bond, sem MITM, SC, E/S
  `NoInputNoOutput`, distribuição de chaves ENC + ID).
- Gate de criação de bond (`ble_hs_cfg.sm_bonding`) controlado pela janela e
  recusa de Pairing Request fora da janela quando já há bond (evento
  `BLE_STORE_EVENT_FULL`).
- "LESC-only" verificado no bond (campo `sc` do store), não no SM.
- Flags de segurança das características NUS (RX/TX) e SYNC (RX/TX) e do
  descritor CCCD.
- Autorização por conexão (decidida no evento de cifração), descarte de
  escritas e de notificações para conexões não autorizadas, desconexão com
  motivo explícito.
- Política de advertising (sempre anunciando enquanto não houver sessão
  autorizada; dados de fabricante com o estado da janela/bond).
- Proteção do armazenamento de bonds (um bond, sem despejo do bond legítimo,
  verificação de gravação, recuperação de falha).
- Persistência dos bonds na NVS e sua relação com a NVS da coleta e da Fase 1.

Fora (outras specs/fases):

- Botão, janela, timers e tarefa → spec 02.
- Catálogo de eventos, testes nativos e roteiro de bancada → spec 03.
- App → spec 04.
- Conteúdo do protocolo SYNC (TLV, handshake, blocos) → Fase 1. Esta spec só
  muda **quem** pode falar com o SYNC.
- Endereço privado do próprio dongle (RPA, ameaça T9) → [Aberto] A3.

## 3. Estado atual do código (verificado)

| Item | Onde | Situação |
|---|---|---|
| Servidor GATT NUS | `esp32-firmware/src/connectivity/ble/BleConnectivity.{h,cpp}` | `NimBLEDevice::init`, `setPower(ESP_PWR_LVL_P9)`, serviço `6E400001-…`, TX `NOTIFY`, RX `WRITE \| WRITE_NR`. **Nenhuma** chamada `setSecurity*`. `onConnect` só loga e chama `_onConnection(true)`; `onDisconnect` chama `_onConnection(false)` e `NimBLEDevice::startAdvertising()`. `sendResponse` faz `setValue` + `notify()` sem destino. No `begin()`, `setName` vem **antes** de `enableScanResponse(true)` (linhas 44–45), o que põe o nome no pacote de anúncio. |
| Ligação com a coleta | `esp32-firmware/src/app/CollectorApp.cpp` (`onConnection`) | Emite `CONN up`, incrementa `ctx.connections` e liga/desliga `APP_CONNECTED` (e limpa `INTERNET`, `SESSION_OK`). Hoje dispara na conexão **crua**. |
| Fila de comandos | `esp32-firmware/src/tasks/Elm327Task.cpp` | Callback do RX só copia para a fila (prioridade 5, sem afinidade). |
| Nome anunciado | `esp32-firmware/src/main.cpp` | `BleConnectivity ble("TCCeltaDongle")`. |
| NVS da coleta | `esp32-firmware/src/storage/NvsStore.cpp` | Namespace `collector` (chaves `config`, `config_ver`, `boot_id`, `last_seq`). Partição `nvs` 0x9000/0x5000 (`partitions_collector.csv`). |
| Camada segura antiga | `docs/seguranca-ble-modelagem-ameacas.md` §4 | Cita `connectivity/secure/`, que **não existe** em `esp32-firmware/src`. |
| Biblioteca | `platformio.ini` `[base] lib_deps = h2zero/NimBLE-Arduino@2.5.1` | O pacote instalado em `.pio/libdeps/base/NimBLE-Arduino` está na **2.5.0** (`.piopm`, `NimBLECppVersion.h`). **[verificar]** após `pio pkg install -e collector_twai` que a 2.5.1 foi baixada; as APIs abaixo foram conferidas na 2.5.0. |
| Framework | `~/.platformio/packages/framework-arduinoespressif32` | Arduino-ESP32 2.0.17; `sdkconfig.h` pré-compilado usa Bluedroid e **não** define `CONFIG_BT_NIMBLE_*` nem `CONFIG_NVS_ENCRYPTION`, logo o NimBLE-Arduino usa os padrões de `src/nimconfig.h` e aceita `-DCONFIG_BT_NIMBLE_*` por build flag. |
| Fase 1 (SYNC) | Pasta `.spec/fase-1-seguranca-telemetria/` | Cria o serviço SYNC (`7A3C0001-…`, RX `…0002`, TX `…0003`). Esta spec assume a Fase 1 implementada (ordem recomendada no `.spec/README.md`); se não estiver, aplica-se só ao NUS e o §5.4 vira contrato para a Fase 1. |

### 3.1 Fatos verificados na NimBLE-Arduino (2.5.0)

Caminhos relativos a `.pio/libdeps/base/NimBLE-Arduino/src/`.

| Fato | Fonte | Consequência no projeto |
|---|---|---|
| `setSecurityAuth(bool bonding, bool mitm, bool sc)` só grava `ble_hs_cfg.sm_bonding/sm_mitm/sm_sc`. | `NimBLEDevice.cpp:1179` | Pode ser chamado a qualquer momento; vale para o **próximo** Pairing Request/Response. |
| `init()` põe `sm_io_cap = NO_INPUT_OUTPUT`, `sm_bonding = sm_mitm = sm_sc = 0` e distribuição `ENC` + `ID` (porque `BLE_LL_CFG_FEAT_LL_PRIVACY` = 1). | `NimBLEDevice.cpp:991-1001`, `nimble/esp_port/port/include/esp_nimble_cfg.h:295` | Sem configuração não há bond nem SC. Fixamos as chaves explicitamente (§5.1). |
| **`ble_hs_cfg.sm_sc_only` muda a checagem de permissão do ATT**: com ele ligado, todo atributo com `*_ENC` exige enlace **autenticado** (MITM), e Just Works nunca é autenticado. | `nimble/nimble/host/src/ble_att_svr.c:329-343`, `ble_sm_sc.c:111-127` | **Não usar `sm_sc_only`**: com ele, nem o celular pareado escreve no RX ou inscreve no CCCD (erro 0x05 para sempre). "LESC-only" é verificado no bond (§5.4). |
| `CONFIG_BT_NIMBLE_SM_LEGACY` é definido como 1 **sem** `#ifndef`. | `nimconfig.h:280` | Não dá para desligar o pareamento legado por flag; a recusa do legado é feita na política (bond com `sc == 0` é apagado e a conexão recusada). |
| O bond só é gravado se **os dois lados** pedem bonding (`BLE_SM_PROC_F_BONDING`), e `sec_state.bonded` reflete isso; restauração com LTK marca `bonded=1`. | `ble_sm.c:1687-1700`, `ble_sm.c:2168`, `ble_sm.c:1246` | Zerar `sm_bonding` fora da janela impede criar bond; o enlace até cifra, mas fica `isBonded() == false` e é recusado. Não há caminho para `bonded=1` sem LTK prévia ou bonding na resposta. |
| `sec_state.bonded=1` é gravado **antes** de `ble_sm_persist_keys`; falha na escrita do store não muda o estado. | `ble_sm.c:2169`, `ble_sm.c:1028-1033` | Depois de um bond novo, conferir que ele está de fato no store (§5.6), senão o celular nunca mais reconecta. |
| Todo Pairing Request passa por `ble_sm_chk_store_overflow` **antes** de alocar o procedimento e antes do tratamento de `REPEAT_PAIRING`: conta bonds + procedimentos ativos e, se `>= MAX_BONDS`, emite `BLE_STORE_EVENT_FULL`; retorno ≠ 0 responde *Pairing Failed* e descarta o pedido. | `ble_sm.c:1094-1110`, `ble_sm.c:1903-1908`, `ble_sm.c:1971` | Com `MAX_BONDS=1` e um bond gravado, **todo** pedido de pareamento gera `FULL`. Devolver ≠ 0 fora da janela recusa qualquer pareamento antes de apagar o bond legítimo (fecha o vetor de `REPEAT_PAIRING`, §5.6). |
| `REPEAT_PAIRING` é tratado **dentro** do `NimBLEServer`: apaga o bond antigo e devolve `RETRY`, sem callback ao usuário; bloquear a deleção no store deixaria o laço `do…while(RETRY)` infinito. | `NimBLEServer.cpp:608-626`, `ble_sm.c:908-945` | Só é alcançado se o `FULL` deixar passar; com o guard do §5.6 isso só ocorre com a janela aberta (sem bonds). |
| `BLE_SM_MAX_PROCS = 1`: um único procedimento de segurança no host inteiro. Um *Security Request* (`startSecurity`) aloca um procedimento com timer de 30 s; com o slot ocupado, o pedido de LTK de **outro** celular é descartado sem resposta (`ENOMEM`), e ele fica preso até o timeout do LL. | `esp_nimble_cfg.h:928-929`, `ble_sm.c:2852-2860`, `ble_sm.c:1447-1449`, `1485-1487` | `startSecurity` só para peer que já tem bond (§5.4); `MAX_PROCS` aumentado como reforço ([verificar], §5.1). |
| Quando o dongle não tem a LTK pedida, o host manda *negative reply* e **não** emite `ENC_CHANGE`. | `ble_sm.c:1409-1415`, `1035-1038` | O dongle não vê "bond perdido" como evento de cifra; vê ATT 0x05 e, em seguida, *timeout* de autenticação ou queda iniciada pelo celular. Quem detecta o bond perdido é o app (spec 04). |
| `BLE_GAP_EVENT_ENC_CHANGE` é entregue aos *listeners* (`setCustomGapHandler`) **antes** do callback do servidor, com `status`; `onAuthenticationComplete(NimBLEConnInfo&)` não recebe o `status`. Há **um único** listener (`m_listener` estático). | `ble_gap.c:8460-8472`, `NimBLEServer.cpp:628-641`, `NimBLEDevice.cpp:1338-1348` | Um listener captura o `status`; a Fase 1 não pode registrar outro (contrato §5.4). |
| Sem cifra, o ATT devolve `0x0F` (*Insufficient Encryption*) só se já existe LTK para o peer; senão `0x05` (*Insufficient Authentication*). | `ble_att_svr.c:345-363` | Estranho recebe 0x05; celular pareado, antes de cifrar, 0x0F. Android e iOS reagem aos dois iniciando pareamento/cifração. |
| `notify(value, len)` sem `connHandle` envia para **todas** as conexões, inscritas ou não. | `NimBLECharacteristic.cpp:272-319` | Toda notificação passa a ter destino explícito: a conexão autorizada. |
| O CCCD é registrado com permissão global `perm_flags = READ \| WRITE`, alterável por `ble_gatts_set_clt_cfg_perm_flags(uint8_t)`, lida só durante `ble_gatts_start()` (disparado pelo primeiro `NimBLEAdvertising::start()` via `pServer->start()`). A função está em header **privado**. | `ble_gatts.c:32,183,1018`, `ble_gatt_priv.h:227`, `NimBLEAdvertising.cpp:199-205` | Chamar antes de qualquer `start()` (§5.1). |
| O `startSecurity` automático do `NimBLEServer` no `SUBSCRIBE` só ocorre se a inscrição for aceita sem cifra; com `WRITE_ENC` no CCCD ela nunca é aceita sem cifra. | `NimBLEServer.cpp:516-545` | `READ_ENC` na TX não serve de gatilho; mantido só como marca documental (§5.3). |
| A resolução do RPA do celular para a identidade acontece no host em toda conexão, **mesmo com o dongle em endereço público**: `ble_rpa_replace_peer_params_with_rl` é chamado fora do `if (ble_host_rpa_enabled())`, com os IRKs restaurados no boot e gravados no bond. `ble_sm_ltk_req_rx` usa o endereço resolvido. | `ble_hs_hci_evt.c:505-516,569-580`, `ble_hs.c:386-391`, `ble_store.c:236-247`, `ble_hs_conn.c:456-460`, `ble_sm.c:1478-1493` | Reconexão com bond funciona com endereço público, desde que o celular distribua o IRK (`setSecurityRespKey` com `ID`). No `onConnect`, `connInfo.getIdAddress()` de um celular pareado já é a identidade ⇒ `NimBLEDevice::isBonded()` funciona ali (§5.4). |
| Lista branca do controlador compara o endereço **no ar**; com privacidade no host (forçada no ESP32 clássico), `ble_gap_wl_*` devolve `ENOTSUP` quando o RPA do host está ligado. | `ble_gap.c:3119-3125`, `esp_nimble_cfg.h:897` | Whitelist de conexão **não serve** para celulares (RPA rotativo). Ver D2. |
| `ble_gap_unpair` (usado por `deleteBond`) só derruba a conexão se a achar pelo endereço de identidade. | `ble_gap.c:8271-8295` | Ao apagar bonds pelo botão, derrubar as conexões **explicitamente** antes (spec 02). |
| Store de bonds: `nvs_open("nimble_bond")` na partição `nvs` padrão. | `nimble/nimble/host/store/config/src/ble_store_nvs.c:45,141` | Namespace próprio, sem colisão com `collector` (§6). |
| `onStoreStatus` padrão = `ble_store_util_status_rr`: em *overflow* apaga o bond **mais antigo**. | `NimBLEDevice.cpp:1383-1386` | Sobrescrever (§5.6). |
| `CONFIG_BT_NIMBLE_MAX_BONDS` (padrão 3) e `CONFIG_BT_NIMBLE_MAX_CONNECTIONS` (padrão 3) aceitam `-D`; `MYNEWT_VAL_BLE_SM_MAX_PROCS` também (`#ifndef`). | `nimconfig.h:224-238`, `esp_nimble_cfg.h:928,985-986` | `MAX_BONDS=1` por build flag; `static_assert` confere. |
| Legacy advertising para ao conectar; `NimBLEServer` reinicia na desconexão se `advertiseOnDisconnect(true)` (padrão) e **sempre** que um CONNECT falha (ignora o `advertiseOnDisconnect`). | `NimBLEServer.cpp:453-457,497-501` | Desligar o automático; `applyAdvertising` mantém dados/intervalos sempre coerentes, porque a biblioteca pode religar o anúncio sozinha. |
| `setManufacturerData` grava no pacote de anúncio (cai no scan response se não couber); `refreshAdvertisingData()` atualiza com o anúncio ativo; `setName` só vai para o scan response se `enableScanResponse(true)` já tiver sido chamado. | `NimBLEAdvertising.cpp:396-403,519-548` | Ordem no `begin()`: `enableScanResponse(true)` → `setName` → `addServiceUUID` → `setManufacturerData`. |

## 4. Modelo de segurança (o que LESC Just Works dá e não dá)

| Propriedade | Com esta spec | Comentário |
|---|---|---|
| Confidencialidade contra escuta passiva (A1.1) | Sim | ECDH P-256 no pareamento + AES-CCM no enlace; a LTK não é derivável da captura (diferente do pareamento legado, quebrável com *crackle*). |
| Integridade e anti-replay no enlace (A1.2) | Sim | MIC de 4 B com contador de pacotes por direção; SKD aleatório por conexão. |
| Acesso direto por terceiro sem bond (A1.4) | Sim | Toda conexão sem bond LESC válido é derrubada (`0x05`); escritas descartadas; nada é notificado. |
| MITM ativo no **primeiro** pareamento (A1.3) | **Não** | Just Works não autentica. Mitigação: só há pareamento dentro da janela de 45 s aberta pelo dono no botão (spec 02); fora dela não se cria bond e, havendo bond, todo Pairing Request é recusado. O MITM precisa estar presente exatamente nessa janela. Um clone do dongle também pode atrair o **celular** (o app conectaria ao falso); o anúncio, inclusive o bit "janela aberta", pode ser forjado. |
| MITM/acesso depois do pareamento | Sim, enquanto o bond for do celular legítimo | O atacante não tem a LTK. Apertar o botão apaga o bond (do atacante, se ele tiver vencido a corrida na janela). |
| DoS de enlace (A1.5) | Parcial | Advertising continua enquanto não houver sessão autorizada; conexões pendentes caem por *timeout*; estranhos não ocupam o procedimento de segurança fora da janela (§5.4/§5.6). Residual: dentro da janela, um atacante que inicia um pareamento e trava segura o SM até 30 s. |
| Autenticar o **app** (e não só o celular) | Não | O bond é do SO; qualquer app do celular pareado usa o enlace. A autenticidade dos dados até o servidor é da Fase 1 (HMAC com `K_rec`). |
| Forward secrecy depois do bond | Não | Quem obtiver a LTK (dump de flash de um dos lados) decifra capturas antigas. Ver §6 (NVS). |

## 5. Design

### 5.1 Configuração do Security Manager

Feita uma vez, em `BleSecurity::configure()`, logo depois de
`NimBLEDevice::init()` e **antes** de `createServer()`/`createService()` e de
qualquer `NimBLEAdvertising::start()`/`NimBLEServer::start()`:

| Chamada | Valor | Motivo |
|---|---|---|
| `NimBLEDevice::setSecurityAuth(false, false, true)` | bond **desligado** no boot, MITM não, SC sim | O bond só é ligado quando a janela abre (§5.2). MITM `false`: com `NoInputNoOutput` o método é Just Works e o par não é autenticado. `sc=true` faz o dongle anunciar suporte a LESC; centrais modernas escolhem LESC. |
| `NimBLEDevice::setSecurityIOCap(BLE_HS_IO_NO_INPUT_OUTPUT)` | `0x03` | O dongle não tem E/S ⇒ Just Works (documento, seção 5). |
| `NimBLEDevice::setSecurityInitKey(BLE_SM_PAIR_KEY_DIST_ENC \| BLE_SM_PAIR_KEY_DIST_ID)` | `0x03` | Chaves que o dongle distribui (LTK, IRK). |
| `NimBLEDevice::setSecurityRespKey(BLE_SM_PAIR_KEY_DIST_ENC \| BLE_SM_PAIR_KEY_DIST_ID)` | `0x03` | Chaves aceitas do celular; o IRK dele é o que permite resolver o RPA nas reconexões. |
| `ble_hs_cfg.sm_sc_only` | **fica 0** | Ver §3.1: ligado, quebra Just Works no ATT. |
| `ble_gatts_set_clt_cfg_perm_flags(BLE_ATT_F_READ \| BLE_ATT_F_WRITE \| BLE_ATT_F_READ_ENC \| BLE_ATT_F_WRITE_ENC)` | `0x27` | Inscrição em notify (escrita do CCCD) exige enlace cifrado. Declarado com `extern "C"` (header privado). Se a função sumir numa versão futura, o gate de aplicação (§5.4) continua garantindo que nada é notificado. |
| `NimBLEDevice::setDeviceCallbacks(&bondStoreGuard)` | §5.6 | Recusar pareamento fora da janela e nunca despejar o bond legítimo. |
| `NimBLEDevice::setCustomGapHandler(&BleSecurity::gapListener, this)` | §5.7 | Capturar o `status` do `ENC_CHANGE`. |
| `pServer->advertiseOnDisconnect(false)` | §5.5 | Advertising controlado pela política. |
| Build flag `-DMYNEWT_VAL_BLE_SM_MAX_PROCS=3` | em `[pairing]` | Reforço contra um peer segurando o único procedimento de segurança. **[verificar]** em bancada (B13) que compila e não aumenta o uso de RAM de forma relevante; se der problema, remover (a regra do `startSecurity` do §5.4 já resolve o caso comum). |

**[Decisão] D1 — "LESC-only" verificado no bond, não no SM.** O modo SC-only
da NimBLE (`sm_sc_only`) torna o ATT exigente demais (autenticação MITM) e o
legado não pode ser desligado por flag. Então: `sm_sc=1` (o dongle oferece
LESC), e a política só autoriza bond cujo registro no store tenha `sc == 1`
e chave de 16 B (`BleSecurity::bondIsLesc`, §5.4). Um bond legado (central
sem LESC, raríssimo em celulares atuais) é apagado e a conexão recusada
(`PAIR_RESULT reason=legacy`).

### 5.2 Gate de criação de bond

`BleSecurity::setBondingAllowed(bool)` grava `ble_hs_cfg.sm_bonding`. Os
campos `sm_*` são *bitfields* na mesma palavra, então **toda** escrita passa
por um `portMUX_TYPE` (seção crítica curta), já que a janela é aberta pela
`PairingTask` e fechada tanto por ela (timeout) quanto pelo callback da
pilha (bond criado). Só o nosso código escreve nesses campos depois do
`init`.

| Estado da janela (spec 02) | `sm_bonding` | `FULL` (há bond) | Efeito num Pairing Request |
|---|---|---|---|
| Fechada, com bond | 0 | recusado (§5.6) | *Pairing Failed* na hora; nada é apagado nem gravado; o SM nem aloca procedimento. |
| Fechada, sem bond | 0 | não ocorre | O par cifra (LESC) mas **sem** bond ⇒ conexão recusada; nada é gravado. |
| Aberta (bonds apagados) | 1 | não ocorre (ou permitido, §5.6) | LESC Just Works **com** bond; o primeiro bond fecha a janela (`reason=bonded`) e volta `sm_bonding=0`. |

O valor é lido pelo SM quando monta a Pairing Response; um pareamento
iniciado com a janela aberta termina com bond mesmo que a janela feche no
meio. A política (spec 02, §4.3) aceita esse caso só para conexões
**admitidas** durante a janela.

### 5.3 Flags das características

Constantes em `BleSecurity.h`, usadas pelo NUS e pelo SYNC:

```cpp
// Escrita app -> dongle: com e sem resposta, só em enlace cifrado.
constexpr uint32_t kSecureRxProps =
    NIMBLE_PROPERTY::WRITE | NIMBLE_PROPERTY::WRITE_NR | NIMBLE_PROPERTY::WRITE_ENC;
// Notificação dongle -> app. A proteção real está no CCCD (perm flags) e no
// gate de aplicação; READ_ENC (sem READ) só documenta a exigência.
constexpr uint32_t kSecureTxProps =
    NIMBLE_PROPERTY::NOTIFY | NIMBLE_PROPERTY::READ_ENC;
```

| Característica | UUID | Propriedades antes | Propriedades depois |
|---|---|---|---|
| NUS RX | `6E400002-B5A3-F393-E0A9-E50E24DCCA9E` | `WRITE \| WRITE_NR` | `kSecureRxProps` |
| NUS TX | `6E400003-B5A3-F393-E0A9-E50E24DCCA9E` | `NOTIFY` | `kSecureTxProps` |
| SYNC RX | `7A3C0002-9B1E-4F2A-8C5D-2E6F1A0B3C4D` | definido pela Fase 1 (`WRITE \| WRITE_NR`) | `kSecureRxProps` |
| SYNC TX | `7A3C0003-9B1E-4F2A-8C5D-2E6F1A0B3C4D` | definido pela Fase 1 (`NOTIFY`) | `kSecureTxProps` |
| CCCD de toda característica `NOTIFY` (inclusive *Service Changed* do GATT) | `0x2902` | `READ \| WRITE` | `READ \| WRITE \| READ_ENC \| WRITE_ENC` (global, §5.1) |

**Não** usar `*_AUTHEN`: exige chave autenticada (MITM), impossível com Just
Works. **Não** usar `*_AUTHOR`: o `NimBLEServer` não trata
`BLE_GAP_EVENT_AUTHORIZE`.

O que acontece com um cliente em cada situação:

| Operação do cliente | Enlace sem cifra | Cifrado **sem** bond (pareou fora da janela, sem bond no dongle) | Cifrado com bond LESC válido (autorizado) |
|---|---|---|---|
| Descobrir serviços | Permitido (UUIDs são públicos) | Permitido | Permitido |
| Escrita com resposta no RX | ATT `0x05` (estranho) ou `0x0F` (celular com LTK no dongle, antes de cifrar); Android/iOS reagem iniciando pareamento/cifração | Passa no ATT, mas o `onWrite` descarta (`WRITE_DROP`) e a conexão já está sendo derrubada com `0x05` | Entregue à fila (ELM327) ou ao SYNC |
| Escrita sem resposta no RX | Descartada pelo ATT em silêncio | Descartada no `onWrite` | Entregue |
| Inscrição no TX (CCCD) | ATT `0x05`/`0x0F` (perm flags) | Aceita no ATT; nenhuma notificação é enviada | Aceita |
| Leitura de RX/TX | `0x02` *Read Not Permitted* (sem `READ`) | idem | idem |
| Receber notificação | Nunca (destino é a conexão autorizada) | Nunca | Sim |

### 5.4 Autorização por conexão e gate de aplicação

A decisão é tomada **no `ENC_CHANGE`**, não no `onConnect`: a identidade
resolvida no `onConnect` diz quem o peer **afirma** ser, mas num pareamento
novo o atacante pode declarar qualquer endereço de identidade na
distribuição de chaves. O único fato confiável é "este enlace foi cifrado
com uma LTK que já estava no store" (restauração) ou "este enlace criou o
bond dentro da janela". Com `sm_bonding` e o `FULL` controlados pela janela
(§5.2, §5.6), `isEncrypted() && isBonded() && getSecKeySize() == 16` só é
verdade nesses dois casos; a política separa um do outro e aplica "um bond
por vez" (spec 02, §4.3).

Fluxo no `BleConnectivity` (todos os callbacks rodam na tarefa *host* do
NimBLE, núcleo 0, prioridade `configMAX_PRIORITIES - 4`):

| Callback NimBLE | O que faz |
|---|---|
| `onConnect(server, connInfo)` | Evento `LINK_UP`. `policy.onLinkUp(handle, now)`. **Só se** `NimBLEDevice::isBonded(connInfo.getIdAddress())`: `NimBLEDevice::startSecurity(handle)` (o celular pareado cifra na hora). Peer desconhecido não recebe *Security Request*: não ocupa o único procedimento de segurança e não ganha diálogo de pareamento fora de hora; quem quer parear inicia (Android: `createBond`; iOS: ao receber 0x05 na inscrição do CCCD). Executa as ações (pode religar o advertising). **Não** liga `APP_CONNECTED`. |
| `gapListener` (`ENC_CHANGE`) | Guarda `status` em `_encStatus[slot]`. |
| `onAuthenticationComplete(connInfo)` | Monta `EncryptionInfo{status, isEncrypted, isBonded, getSecKeySize, lesc, persisted}`, em que `lesc`/`persisted` vêm de `BleSecurity::bondIsLesc(idAddr)` (lê `ble_store_read_peer_sec`: `sc == 1` e `ltk_present`) quando `isBonded()`. Chama `policy.onEncryptionChange`. Se **autorizar**: grava `_authorizedConn` (atômico), para o advertising, chama `_onConnection(true)` (agora sim `CONN up=1`, `APP_CONNECTED`). Se **recusar**: `server->disconnect(handle, motivo)`; se a política pedir, `NimBLEDevice::deleteBond(idAddr)` antes. Re-cifração numa conexão já autorizada é idempotente. |
| `onDisconnect(server, connInfo, reason)` | Evento `LINK_DOWN`. `policy.onLinkDown`. Se era a autorizada: zera `_authorizedConn` e chama `_onConnection(false)`. Executa as ações (religa advertising). |
| `onWrite(chr, connInfo)` | Se `!isAuthorized(connInfo.getConnHandle())`: evento `WRITE_DROP` e retorna. Senão, como hoje. |
| `onSubscribe(chr, connInfo, subValue)` | Guarda se a conexão autorizada está inscrita no TX do NUS (`_nusSubscribed`). |
| `sendResponse(data, len)` | Se não há conexão autorizada ou ela não está inscrita: descarta. Senão `_txChar->notify(data, len, _authorizedConn)`. |

Motivos de desconexão escolhidos pelo dongle (o app usa para classificar a
falha, spec 04):

| Situação | Motivo HCI | Constante NimBLE |
|---|---|---|
| Conexão recusada (sem bond, bond legado, cifra falhou, *timeout* de autenticação, chave curta, bond não gravado) | `0x05` *Authentication Failure* | `BLE_ERR_AUTH_FAIL` |
| Recusa transitória: já existe sessão autorizada, ou bonds sendo apagados pelo botão | `0x09` *Connection Limit Exceeded* | `BLE_ERR_CONN_LIMIT` |
| Botão: derrubar todas as conexões para novo pareamento | `0x13` *Remote User Terminated* | `BLE_ERR_REM_USER_CONN_TERM` |

O `0x09` existe para o app **não** confundir uma recusa passageira com
"perdi o bond". No lado do dongle, a queda iniciada por ele aparece em
`LINK_DOWN` como `hci=16` (*connection terminated by local host*); o motivo
enviado fica no `CONN_REJECT hci=`.

**Contrato para o SYNC (Fase 1).** Na Fase 1 (spec 04 §7–8) as
características do SYNC são criadas **dentro de `BleConnectivity`** (UUIDs
`SYNC_SERVICE_UUID`/`SYNC_RX_UUID`/`SYNC_TX_UUID`), e o transporte fica em
`BleSyncLink` (`src/connectivity/ble/BleSyncLink.{h,cpp}`, `sync::ISyncLink`),
registrado por `ble.setSyncLink(...)` no `CollectorApp`; `BleConnectivity`
repassa `onWrite` do SYNC RX, `onSubscribe` do SYNC TX, `onConnect`/
`onDisconnect` e o MTU ao `BleSyncLink`, que notifica com janela de créditos.
Esta fase altera esses pontos assim:

1. criar SYNC RX/TX (e NUS RX/TX) com `BleSecurity::kSecureRxProps`/
   `kSecureTxProps` no `BleConnectivity::begin()`;
2. no `onWrite` do SYNC RX (em `BleConnectivity`, antes de repassar ao
   `BleSyncLink`), chamar `isAuthorized(connInfo.getConnHandle())` e
   descartar com `WRITE_DROP chr=sync_rx` se falso;
3. `BleSyncLink` notifica sempre com `notify(data, len, authorizedConn())`, e
   só se `authorizedConn() != BLE_HS_CONN_HANDLE_NONE`;
4. `BleSyncLink::onConnected`/`onDisconnected` (que reiniciam a sessão do
   `SyncEngine`, spec 05 §4) passam a ser chamados na **autorização** e na
   queda da conexão autorizada, não na conexão crua; `APP_CONNECTED` passa a
   significar "sessão autorizada";
5. a Fase 1 não registra `setCustomGapHandler` (confirmado na spec 04); o
   único *listener* de GAP é o `BleSecurity`.

### 5.5 Advertising

**[Decisão] D2 — anunciar sempre que não houver sessão autorizada; não usar
whitelist.** Alternativas avaliadas:

| Opção | Problema |
|---|---|
| Anunciar só durante a janela | O celular com bond não reconecta sozinho (Android/iOS precisam ver o anúncio); o usuário teria de apertar o botão a cada partida — e o botão **apaga** o bond. Inviável. |
| Whitelist de conexão com os endereços dos bonds | Celulares conectam com RPA rotativo; a whitelist do controlador compara o endereço no ar e o ESP32 clássico só tem privacidade no host (§3.1). O celular legítimo seria barrado. |
| Advertising dirigido ao celular | Precisa do endereço atual do peer (RPA) e o iOS não garante resposta a anúncio dirigido. |
| **Anunciar sempre (conectável), decidir no `ENC_CHANGE`** | Escolhida. Estranhos conectam no enlace e caem em ≤ `LINK_AUTH_TIMEOUT_MS` (5 s) ou no `ENC_CHANGE`. |

Regras (executadas pela política, spec 02 §4.3.3):

| Condição | Advertising | Intervalo |
|---|---|---|
| Bonds sendo apagados (botão) | Desligado | — |
| Há conexão autorizada | **Desligado** (uma sessão por vez) | — |
| Sem autorizada, janela aberta, conexões < `CONFIG_BT_NIMBLE_MAX_CONNECTIONS` | Ligado, modo `pairing` | 211,25 ms (`338` × 0,625 ms) |
| Sem autorizada, janela fechada, conexões < máximo | Ligado, modo `normal` | 546,25 ms (`874` × 0,625 ms) |
| Conexões = máximo | Desligado até alguma cair | — |

Intervalos da lista recomendada pela Apple para acessórios ([verificar] a
edição vigente das *Accessory Design Guidelines*), como `constexpr` em
`BleSecurity.h`. Com o dongle alimentado pelo OBD-II, o consumo do anúncio
(~1–2 mA médios a 500 ms) é irrelevante; o ganho é reconexão em ~1–2 s.
Como o `NimBLEServer` religa o anúncio sozinho quando um CONNECT falha
(§3.1), `applyAdvertising` sempre deixa dados e intervalos coerentes com o
estado atual mesmo quando o modo pedido é `Off`; um anúncio religado assim
é inofensivo (a política recusa o que vier).

**Dados de fabricante.** O anúncio ganha um *Manufacturer Specific Data*
para o app saber, **antes** de conectar, se o dongle está pareável:

| Byte | Valor | Significado |
|---|---|---|
| 0–1 | `FF FF` | Company ID `0xFFFF` (reservado pelo SIG para testes/uso interno). |
| 2 | `0x01` | Versão do formato. |
| 3 | `flags` | bit 0 = janela aberta; bit 1 = há bond guardado; bits 2–7 = 0. |

Orçamento do pacote de anúncio (31 B): flags AD 3 B + UUID de 128 bits do
NUS 18 B + fabricante 6 B = 27 B. O nome (`TCCeltaDongle`, 15 B) vai no scan
response — ordem no `begin()`: `enableScanResponse(true)`, depois `setName`
(o código atual faz o inverso). **A Fase 1 não deve pôr o UUID do SYNC no
anúncio** (estouraria o pacote); o app encontra o dongle pelo UUID do NUS. A
cada mudança de janela/bond: `setManufacturerData(...)` +
`refreshAdvertisingData()`.

O campo é **só uma dica de UX**: um atacante pode clonar o anúncio. O app
nunca toma decisão destrutiva só com ele (spec 04, D16).

### 5.6 Armazenamento de bonds: um por vez, recusa fora da janela, sem despejo

- Build flag `-DCONFIG_BT_NIMBLE_MAX_BONDS=1` em `[pairing]` (spec 02 §5.1),
  conferido com `static_assert(MYNEWT_VAL(BLE_STORE_MAX_BONDS) == 1)` em
  `BleSecurity.cpp`.
- `class BondStoreGuard : public NimBLEDeviceCallbacks`,
  `int onStoreStatus(ble_store_status_event* ev, void* arg) override`:

| `event_code` | Condição | Retorno | Efeito | Evento |
|---|---|---|---|---|
| `BLE_STORE_EVENT_FULL` (todo Pairing Request com bond gravado; também conta procedimentos ativos) | janela **fechada** (`!BleSecurity::bondingAllowed()`) | `BLE_HS_ESTORE_CAP` | *Pairing Failed* imediato; nenhum procedimento alocado; `REPEAT_PAIRING` não é alcançado, logo o bond legítimo **não** é apagado nem por quem forja o endereço de identidade do celular | `BOND_FULL event=full action=reject` |
| `BLE_STORE_EVENT_FULL` | janela **aberta** (bonds = 0; o `FULL` só vem de procedimentos de outras conexões) | `0` | Segue o pareamento (devolver ≠ 0 aqui impediria qualquer pareamento: *lockout*) | `BOND_FULL event=full action=allow` |
| `BLE_STORE_EVENT_OVERFLOW` | `obj_type` ∈ {`OUR_SEC`, `PEER_SEC`} | `BLE_HS_ESTORE_CAP` | Não grava o novo e **não** apaga o existente | `BOND_FULL event=overflow action=reject` |
| demais | — | `ble_store_util_status_rr(ev, arg)` | Padrão da biblioteca (ex.: CCCDs) | — |

- **Verificação de gravação.** Depois de autorizar um bond novo, conferir
  `NimBLEDevice::getNumBonds() == 1` e `bondIsLesc(idAddr)` com
  `persisted == true` (`ble_store_read_peer_sec` devolve 0 e `ltk_present`).
  Se falhar (NVS cheia/erro de escrita): evento `BOND_VERIFY ok=0`, recusar a
  conexão (`0x05`) e apagar o que houver; a janela continua aberta (não foi
  consumida). No boot, emitir `NVS free=… used=…` (`nvs_get_stats`) para
  acompanhar o espaço da partição `nvs`, compartilhada com a coleta.
- **Falha ao apagar bonds** (`deleteAllBonds` falhou duas vezes, spec 02):
  `BleSecurity::eraseBondStoreAndRestart()` — `nvs_open("nimble_bond")` +
  `nvs_erase_all` + `nvs_commit` + `esp_restart()`. Evita o *lockout* de uma
  janela que nunca abre; o usuário segura o botão de novo depois do boot.
  Apaga também o IRK local (o dongle usa endereço público, sem efeito
  visível).
- **Higiene no boot** (`BleSecurity::sanitizeBondsAtBoot()`): se
  `getNumBonds() > 1` (resto de firmware antigo) ou se o único bond tem
  `sc == 0`, apagar todos (`BOND_DELETE reason=policy`). Na primeira gravação
  deste firmware, a instrução de bancada também é `pio run -t erase`.
- `deleteAllBonds` continua usando `deleteBond` (que chama `ble_gap_unpair`),
  **não** `ble_store_clear` (que apagaria o IRK local fora do caso de
  recuperação acima).

### 5.7 Listener de GAP

```cpp
// Registrado com NimBLEDevice::setCustomGapHandler(&BleSecurity::gapListener, this).
static int gapListener(ble_gap_event* event, void* arg);
```

Trata só `BLE_GAP_EVENT_ENC_CHANGE` (guarda `event->enc_change.status` por
`conn_handle` numa tabela de 4 posições com `std::atomic<int>`); devolve 0.
Roda na tarefa *host*, antes do `onAuthenticationComplete` da mesma conexão
(`ble_gap.c:8471-8472`). É o único listener do dispositivo (§3.1).

## 6. Bonds na NVS e relação com a coleta e a Fase 1

| Dono | Partição | Namespace | Chaves |
|---|---|---|---|
| NimBLE (bonds, CCCDs, IRK local, registros de peer para resolver RPA) | `nvs` (padrão) | `nimble_bond` | `our_sec_N`, `peer_sec_N`, `cccd_N`, `local_irk`, registros de peer (internas da biblioteca) |
| Coleta | `nvs` | `collector` | `config`, `config_ver`, `boot_id`, `last_seq` |
| Fase 1: `K_auth`, `K_rec`, `DevId`, `k_sig` (spec 01 §4) | `secnvs` (partição própria, 12 KB) | `sec` | `prov_ver`, `dev_id`, `k_auth`, `k_rec`, `k_sig`, `prof` |
| Fase 1: reserva de `seq`, `last_cs`, `wipe_req` (spec 03 §2) | `nvs` | `collector` | `seq_rsv`, `last_cs`, `wipe_req` (+ `last_seq` legado até a migração) |

- Não há colisão: namespaces distintos. A Fase 1 não usa o namespace
  `nimble_bond` nem guarda chaves criptográficas em `collector`.
- Tamanho: um bond ocupa poucas centenas de bytes; a partição `nvs` de 20 KB
  comporta com folga, mas é compartilhada (por isso o evento `NVS` no boot e
  a verificação de gravação do §5.6).
- **Cobertura pela NVS encryption da Fase 1.** A *flash encryption* do ESP32
  **não** cifra partições NVS; elas só ficam cifradas com *NVS encryption*
  (chave na partição `nvs_keys`, protegida pela flash encryption). O store do
  NimBLE abre a partição **padrão** (`nvs_open`, sem `_from_partition`). O
  que a Fase 1 (spec 01 §7) fixou:
  - **perfil de produção** (Arduino como componente do ESP-IDF,
    `CONFIG_NVS_ENCRYPTION=y`, chaves XTS na partição `nvs_key`): a NVS
    encryption cobre a `secnvs` **e** a `nvs` padrão, então os bonds ficam
    cifrados automaticamente;
  - **perfil de desenvolvimento** (Arduino 2.0.17 pré-compilado, sem
    `nvs_flash_secure_init`): nada é cifrado, e **os bonds (LTK/IRK) ficam em
    claro** na `nvs`, assim como as chaves na `secnvs`. Com acesso físico à
    flash (A3), o atacante obtém a LTK do par corrente: decifra capturas desse
    par e pode se passar pelo celular até o próximo botão. Não afeta outros
    dongles. Risco residual só de bancada; ver [Aberto] A2.

## 7. Plano arquivo por arquivo

| Arquivo | Ação | Conteúdo |
|---|---|---|
| `esp32-firmware/src/connectivity/ble/BleSecurity.h` | novo | Namespace `ble_security` com `kSecureRxProps`, `kSecureTxProps`, `kCompanyId = 0xFFFF`, `kAdvFormatVersion = 0x01`, `kAdvItvlPairing = 338`, `kAdvItvlNormal = 874`; `class BleSecurity`; `class BondStoreGuard`. |
| `esp32-firmware/src/connectivity/ble/BleSecurity.cpp` | novo | Implementação; `extern "C" void ble_gatts_set_clt_cfg_perm_flags(uint8_t);`; `static_assert` do `MAX_BONDS`. |
| `esp32-firmware/src/connectivity/ble/BleConnectivity.h` | alterar | Implementa `pairing::ILinkControl` (spec 02); recebe `pairing::PairingController*` via `setPairingController`; novos métodos públicos abaixo; `onSubscribe`; `onAuthenticationComplete`. |
| `esp32-firmware/src/connectivity/ble/BleConnectivity.cpp` | alterar | §5.4; `begin()` chama `BleSecurity::configure()` antes de criar o servidor, usa as novas flags, `advertiseOnDisconnect(false)`, ordem `enableScanResponse` → `setName` → UUID → fabricante; **não** inicia o advertising (quem inicia é `pairing.begin()`); remove o `NimBLEDevice::startAdvertising()` do `onDisconnect`. |
| `esp32-firmware/src/connectivity/ble/BleSyncLink.{h,cpp}` (Fase 1) | alterar | Contrato do §5.4 (itens 3–4): notificar só à conexão autorizada; sessão reiniciada na autorização. |
| `esp32-firmware/src/app/CollectorApp.cpp` | alterar | `onConnection` igual, mas agora chamado só na autorização. Ordem de inicialização na spec 02 §6. |
| `esp32-firmware/platformio.ini` | alterar | Seção `[pairing]` (spec 02 §5.1) com `-DCONFIG_BT_NIMBLE_MAX_BONDS=1` e `-DMYNEWT_VAL_BLE_SM_MAX_PROCS=3`. |
| `docs/seguranca-ble-modelagem-ameacas.md` | alterar (doc) | §3.5: o "SC-only" da NimBLE exige MITM no ATT e não serve a Just Works; §3.8 "Estado atual do firmware": LESC Just Works obrigatório com verificação `sc` no bond, flags, janela; §4 marcada como **histórica** (a camada `connectivity/secure/` foi removida do firmware); cenário C1 da §7 passa a ser o padrão. |

Assinaturas:

```cpp
// BleSecurity.h
namespace ble_security {

class BondStoreGuard : public NimBLEDeviceCallbacks {
public:
    int onStoreStatus(ble_store_status_event* event, void* arg) override;
};

struct BondCheck {
    bool found;      // há registro de peer_sec para o endereço
    bool lesc;       // sc == 1
    bool persisted;  // ltk_present
};

class BleSecurity {
public:
    /** Chamar logo após NimBLEDevice::init() e antes de criar o servidor ou anunciar. */
    static void configure();
    /** Liga/desliga a criação de bond (ble_hs_cfg.sm_bonding), sob seção crítica. */
    static void setBondingAllowed(bool allowed);
    static bool bondingAllowed();
    /** Quantos bonds há no store (NimBLEDevice::getNumBonds). */
    static int bondCount();
    /** Lê o bond do peer no store (ble_store_read_peer_sec). */
    static BondCheck checkBond(const NimBLEAddress& idAddress);
    /** Apaga todos os bonds; devolve ok e o tempo gasto. Não derruba conexões. */
    static bool deleteAllBonds(uint32_t& durMs);
    /** Recuperação: apaga o namespace nimble_bond inteiro e reinicia o ESP32. Não retorna. */
    [[noreturn]] static void eraseBondStoreAndRestart();
    /** Mais de um bond, ou bond legado, no boot => apaga todos. Emite NVS. */
    static void sanitizeBondsAtBoot();
    /** Status do último ENC_CHANGE desta conexão (0 = ok). */
    static int lastEncStatus(uint16_t connHandle);
    /** Monta os 4 bytes de dados de fabricante: FF FF ver flags. */
    static void manufacturerData(bool windowOpen, bool hasBond, uint8_t out[4]);

private:
    static int gapListener(ble_gap_event* event, void* arg);
};

}  // namespace ble_security
```

```cpp
// BleConnectivity.h (acréscimos públicos, além de pairing::ILinkControl)
/** Conexão autorizada (cifrada + bond LESC válido), ou BLE_HS_CONN_HANDLE_NONE. Thread-safe. */
uint16_t authorizedConn() const;
bool     isAuthorized(uint16_t connHandle) const;
void     setPairingController(pairing::PairingController* controller);
```

Estado privado novo: `std::atomic<uint16_t> _authorizedConn{BLE_HS_CONN_HANDLE_NONE}`,
`std::atomic<bool> _nusSubscribed{false}`, `NimBLEServer* _server`,
`pairing::PairingController* _pairing`, tabela `connHandle → idAddress` das
conexões ativas (para `deleteBond` numa recusa).

## 8. Passos ordenados com checkpoints

Pré-requisito: spec 02 passos 1–2 (núcleo puro `PairingPolicy` + testes
nativos verdes).

1. **Seção `[pairing]` no `platformio.ini`** com os flags da spec 02 e
   `-DCONFIG_BT_NIMBLE_MAX_BONDS=1`, `-DMYNEWT_VAL_BLE_SM_MAX_PROCS=3`.
   *Checkpoint:* `pio run -e collector_mock_debug` compila; o `static_assert`
   do `MAX_BONDS` passa; apagar o flag de propósito faz o build falhar;
   log de boot imprime `NimBLEDevice::getVersion()`.
2. **`BleSecurity` sem política** (configure, gate, guard, listener,
   fabricante, verificação de bond). Chamar `configure()` em
   `BleConnectivity::begin()`. *Checkpoint:* no boot, log
   `[BLE] LESC Just Works, bond=0, MAX_BONDS=1`; nRF Connect conecta e, ao
   pedir *bond*, o par cifra mas sem bond (`LINK_ENC up=1 bonded=0`).
3. **Flags das características NUS** e perm flags do CCCD. *Checkpoint
   (bancada B7):* nRF Connect sem parear: escrita com resposta no RX
   devolve erro 0x05; inscrição no TX devolve 0x05; escrita sem resposta não
   chega ao `CMD`.
4. **Autorização e gate de aplicação** no `BleConnectivity` (onConnect →
   `startSecurity` só para peer com bond; onAuthenticationComplete → política;
   onDisconnect, onWrite, onSubscribe, sendResponse com destino).
   `_onConnection` só na autorização. *Checkpoint:* janela fechada e sem
   bond, nenhum comando ELM327 é processado; eventos `CONN_REJECT`; nenhum
   `CONN up=1`.
5. **Advertising controlado pela política** (`advertiseOnDisconnect(false)`,
   ordem do scan response, `applyAdvertising`). *Checkpoint:* com sessão
   autorizada, scanner não vê o dongle; ao cair, o anúncio volta com `flags`
   corretas (nRF Connect mostra `0xFFFF` → `01 02` com bond, `01 01` com
   janela aberta) e o nome aparece no scan response.
6. **Contrato do SYNC** aplicado à classe da Fase 1. *Checkpoint:* testes da
   Fase 1 (handshake/leitura contínua) passam com o app pareado; com cliente
   não pareado, `WRITE_DROP chr=sync_rx`.
7. **Higiene, verificação e recuperação de bonds** e atualização de
   `docs/seguranca-ble-modelagem-ameacas.md`. *Checkpoint:* bancada B1–B16
   da spec 03; com bond gravado, nRF Connect pedindo *bond* fora da janela
   gera `BOND_FULL event=full action=reject` e o bond continua lá.

## 9. Riscos

| # | Risco | Mitigação |
|---|---|---|
| R1 | `ble_gatts_set_clt_cfg_perm_flags` é API privada e pode mudar. | Declaração `extern "C"` isolada em `BleSecurity.cpp`; o gate de aplicação garante o mesmo efeito (nada é notificado a não autorizado). Checkpoint 3 detecta a regressão. |
| R2 | Celular que não distribui IRK ⇒ RPA não resolvido ⇒ reconexão com bond vira pareamento (recusado fora da janela). | A resolução no host foi verificada com endereço público (§3.1); `setSecurityRespKey` pede o IRK. Bancada B8/B9 em Android e iOS. |
| R3 | `REPEAT_PAIRING` apagando o bond legítimo (atacante com o endereço de identidade do celular). | Fechado fora da janela pelo `FULL` (§5.6): o pedido é recusado antes do tratamento de `REPEAT_PAIRING`. Dentro da janela não há bond a perder. |
| R4 | DoS: estranho ocupando o único procedimento de segurança (`BLE_SM_MAX_PROCS = 1`) e deixando o celular pareado sem resposta de LTK; ou conexões repetidas ocupando os 3 slots. | Sem *Security Request* para desconhecidos; `FULL` recusa pareamento de estranho fora da janela antes de alocar procedimento; `MAX_PROCS=3` como reforço; advertising segue ligado com conexões pendentes; *timeout* de 5 s. Bancada B13. Residual: dentro da janela, um pareamento travado segura o SM até 30 s. |
| R5 | Celular com bond "velho" após o botão (dongle sem LTK). | Tratado no app (spec 04: `removeBond` no Android, instruções no iOS). |
| R6 | Divergência de versão da NimBLE (2.5.0 instalada × 2.5.1 pedida). | Passo 1 roda `pio pkg install`; versão no log de boot; reconferir as linhas citadas no §3.1 se a versão mudar. |
| R7 | Escrita concorrente nos *bitfields* `ble_hs_cfg.sm_*`. | Todas as escritas sob o mesmo `portMUX_TYPE`; só o nosso código escreve após o `init`. |
| R8 | NVS cheia: bond marcado como criado mas não gravado ⇒ celular nunca mais reconecta (*lockout*). | Verificação de gravação + recusa (§5.6); evento `NVS` no boot. |

## 10. Decisões e pontos em aberto

- **[Decisão] D1** — LESC-only verificado no bond (`sc == 1`, chave 16 B),
  não pelo `sm_sc_only` da NimBLE (§5.1).
- **[Decisão] D2** — Anunciar sempre sem whitelist; autorização no
  `ENC_CHANGE` (§5.5).
- **[Decisão] D3** — Gate de bond por `sm_bonding` ligado só na janela e, com
  bond gravado, recusa de todo Pairing Request fora da janela via
  `BLE_STORE_EVENT_FULL` (§5.2, §5.6). Alternativa descartada: interceptar
  `store_write_cb` (não sabe a qual conexão a escrita pertence e age tarde
  demais).
- **[Decisão] D4** — Motivos de desconexão `0x05` (recusa), `0x09` (ocupado,
  transitório) e `0x13` (botão) (§5.4).
- **[Decisão] D5** — Uma única sessão autorizada; advertising desligado
  enquanto ela existe (§5.5).
- **[Decisão] D6** — Estado janela/bond nos dados de fabricante, Company ID
  `0xFFFF` (§5.5). Vazamento: um scanner sabe se o dongle tem bond.
- **[Decisão] D21** — *Security Request* só para peer que já tem bond (§5.4):
  preserva o único procedimento de segurança para o celular legítimo e evita
  diálogos de pareamento fora de hora.
- **[Decisão] D22** — Bond confirmado no store antes de valer; falha ao
  apagar bonds leva à recuperação por apagamento do namespace + reinício
  (§5.6).
- **[Aberto] A1** — Manter o bit "há bond" no anúncio? *Recomendação:*
  manter; é ele que permite ao app suspeitar de "dongle perdeu o pareamento"
  sem tentar conectar.
- **[Aberto] A2** — Bonds em claro na NVS se a Fase 1 cifrar só uma partição
  separada (§6). *Recomendação:* aceitar e registrar no texto do TCC como
  risco residual de A3 restrito ao par corrente.
- **[Aberto] A3** — Endereço do dongle: público (atual) ou RPA (privacidade,
  T9)? *Recomendação:* manter público; a reconexão não depende disso (§3.1)
  e o Android identifica o dongle pelo MAC, que o app persiste.
- **[Aberto] A4** — *Fork* da NimBLE-Arduino para tratar `REPEAT_PAIRING`?
  *Recomendação:* não é mais necessário (D3 recusa antes); só registrar no
  texto.

## 11. Critérios de aceite

1. Build de todos os ambientes `collector_*` e `collector_*_debug` sem
   *warnings* novos; `static_assert` de `MAX_BONDS == 1` presente.
2. Cliente sem pareamento: não processa nenhum `CMD`, não recebe nenhuma
   notificação, é desconectado com `0x05` em ≤ 5 s (fora da janela).
3. Pareamento fora da janela: sem bond no dongle, cifra mas não grava bond e
   a conexão cai com `0x05`; com bond no dongle, recebe *Pairing Failed*
   (`BOND_FULL action=reject`) e o bond existente continua intacto
   (`getNumBonds()` e reconexão do celular legítimo inalterados).
4. Pareamento dentro da janela: bond gravado e verificado, `CONN up=1`,
   comandos ELM327 e SYNC funcionam; desligar e religar o dongle e o celular
   reconecta sem novo pareamento (Android e iOS).
5. Pareamento legado (central sem LESC) não resulta em sessão: bond apagado,
   `PAIR_RESULT ok=0 reason=legacy`.
6. Captura com sniffer mostra pareamento LESC (troca de chaves públicas) e
   PDUs de dados cifrados após `LL_ENC_REQ`/`LL_START_ENC`.
