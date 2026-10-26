# Fase 2 — Firmware + App: pareamento BLE LESC Just Works

## Objetivo

Trocar a camada de enlace entre app e dongle por **BLE LE Secure Connections
(LESC) com bond**, como fixa o documento de segurança
(`latex/aux-docs/seguranca-dongle-server/segunraca-dongle-server.tex`, seção
3.1 e início da seção 5): o dongle aceita **apenas** enlaces LESC; o método é
**Just Works** (o dongle não tem E/S); pareamento **só** numa janela aberta
pelo botão físico; **um bond por vez**; apertar o botão **derruba todos** os
pareamentos e conexões. Isso fecha A1.1/A1.2/A1.4 e reduz A1.3 (MITM) e A1.5
(DoS) do modelo de ameaças.

## Entregas

1. **Firmware** — Security Manager do NimBLE em LESC-only com bond; NUS e
   SYNC exigindo enlace cifrado e sessão autorizada; nada é lido, escrito
   ou notificado sem bond válido; advertising com o estado da janela.
2. **Firmware** — botão BOOT (GPIO0) segurado 5 s ⇒ derruba conexões,
   apaga bonds e abre a janela de 45 s; tempos por build flag
   (`PAIR_BUTTON_HOLD_MS`, `PAIR_WINDOW_MS`); fora da janela nenhum bond é
   criado.
3. **Firmware** — eventos de depuração de toda transição (`PAIR_BTN`,
   `PAIR_WINDOW`, `BOND_DELETE`, `PAIR_RESULT`, `CONN_REJECT`, `LINK_ENC`, …)
   e testes nativos da política com relógio falso.
4. **App** — pareamento fim a fim conduzido pelo SO (Android: `createBond`;
   iOS: alerta do sistema), telas para "segure o botão", janela expirada,
   pareamento recusado, bond perdido (Android remove e pareia de novo; iOS
   orienta "Esquecer este dispositivo"), reconexão automática com bond; a
   camada PSK antiga sai do caminho de conexão.

## Specs (ordem de leitura)

| # | Arquivo | Conteúdo |
|---|---|---|
| 01 | `01-firmware-lesc-nimble.md` | Configuração LESC no NimBLE, gate de bond, flags de características e CCCD, autorização por conexão, advertising, bonds na NVS. |
| 02 | `02-firmware-botao-e-janela-de-pareamento.md` | Detector do botão, máquina de estados da janela e das conexões (`PairingPolicy`), tarefa, timers, prioridades, build flags. |
| 03 | `03-firmware-eventos-e-testes.md` | Catálogo de eventos para `docs/debug-events.md`, testes nativos, roteiro de bancada B1–B16 e métricas. |
| 04 | `04-app-pareamento-just-works.md` | Port/adapter BLE, use cases, `ConnectingViewModel`, `PairingScreen`, remoção da camada PSK, testes com `FakeBleService`. |

### Ordem de execução

1. Spec 02 passos 1–2 + spec 03 passos 1–3: `PairButton` e `PairingPolicy`
   puros com testes nativos (sem hardware).
2. Spec 01 passos 1–7 junto com a spec 02 passos 3–6 (integração NimBLE,
   tarefa, GPIO).
3. Spec 03 passos 4–5: documentação dos eventos e bancada.
4. Spec 04: pode começar em paralelo ao item 2 (contrato fixado nas specs
   01/02; desenvolvimento com `FakeBleService`); os passos 3, 5 e 6 dela
   precisam do firmware dos itens 1–2.

## Dependências e fronteira com a Fase 1

- **Pré-requisito recomendado:** Fase 1 implementada (ordem do
  `.spec/README.md`: Fase 1 → Fase 3 → Fase 2).
- O serviço **SYNC** (`7A3C0001-9B1E-4F2A-8C5D-2E6F1A0B3C4D`, RX `…0002`,
  TX `…0003`) e o **NUS** continuam com o mesmo protocolo. Esta fase só muda
  a camada de enlace/segurança da conexão: flags `WRITE_ENC`/`READ_ENC` nas
  características, descarte de escrita e notificação para conexão não
  autorizada, e o significado de `APP_CONNECTED`/`CONN up` ("sessão
  autorizada"). Contrato detalhado: spec 01 §5.4.
- A Fase 1 não deve anunciar o UUID do SYNC (o pacote de anúncio não
  comporta com os dados de fabricante; spec 01 §5.5) nem usar o namespace
  NVS `nimble_bond` (spec 01 §6).
- Tela de configurações: a rota `/settings` e os parâmetros T_lat/B_max/T_GPS
  são da Fase 1; a Fase 2 remove apenas o conteúdo de PSK (spec 04 §5.7).
- Camada PSK do app: a Fase 1 (spec 07 §1) só a desliga por padrão atrás de
  `--dart-define=LEGACY_PSK_TRANSPORT` e **não apaga arquivo nenhum**; esse
  é o estado intermediário. A remoção (ou isolamento) é desta fase (spec 04
  §5.7, A8), que também retira a flag.
- NVS encryption da Fase 1 (spec 01 §7): no perfil de **produção** cifra a
  `nvs` padrão além da `secnvs`, cobrindo os bonds do NimBLE; no perfil de
  **desenvolvimento** (Arduino puro) nada é cifrado e os bonds ficam em claro
  (risco residual aceito, spec 01 §6, A2).

## Critérios de aceite da fase

1. Nenhum comando ELM327 é processado e nenhuma mensagem SYNC/NUS é
   notificada para um cliente sem bond válido (bancada B6, B7).
2. Fora da janela, nenhum bond é criado nem sobrescrito; dentro dela, o
   primeiro celular a parear fica com o único bond e a janela fecha
   (B4, B6, POL-07/09/14/25).
3. Segurar o BOOT 5 s derruba conexões, apaga bonds e abre a janela de 45 s;
   tempos mudam por build flag (B3, B5, spec 02 §10).
4. Celular pareado reconecta sozinho após reinício do dongle ou do app, sem
   novo diálogo, em Android e iOS (B8, B9).
5. App conduz o usuário em todos os desvios: janela fechada/expirada,
   recusa, bond perdido em Android (automático) e iOS (instruções) (B5,
   B10, B11, B15).
6. Captura com sniffer mostra pareamento LESC e tráfego cifrado; o firmware
   anterior à fase mostra os comandos em claro (B12).
7. `pio test -e native` e a CI do app (`format`, `analyze`, `test`) verdes;
   todos os eventos novos documentados em `docs/debug-events.md`.

## Roteiro de validação em bancada (resumo)

Material: ESP32 DevKit (botão BOOT/GPIO0), firmware `collector_*_debug` com a
serial gravada em arquivo, Android ≥ 10 e iPhone com o app, nRF Connect e
LightBlue; opcional nRF52840 Dongle + nRF Sniffer no Wireshark e `crackle`.
Antes da primeira gravação: `pio run -t erase` e esquecer o dongle nos
celulares. Cenários completos, eventos esperados e métricas (tempo de
pareamento, de restauração e até a sessão, N = 10 por SO) na spec 03 §4:

B1 boot sem bond · B2 toque curto · B3 abrir janela · B4 parear Android ·
B5 janela expira · B6 estranho fora da janela · B7 acesso sem pareamento ·
B8 reconexão Android · B9 parear e reconectar iOS · B10 botão com Android
conectado · B11 botão com iPhone conectado · B12 sniffer · B13 DoS de
conexões · B14 falta de energia na janela · B15 usuário cancela · B16
central sem LESC (opcional).

## Índice de decisões e pontos em aberto

| Id | Spec | Resumo |
|---|---|---|
| D1 | 01 | LESC-only verificado no bond (`sc == 1`, 16 B), **não** pelo `sm_sc_only` da NimBLE (que quebraria Just Works no ATT). |
| D2 | 01 | Anunciar sempre (sem whitelist: celulares usam RPA); autorizar no `ENC_CHANGE`. |
| D3 | 01 | Gate de bond por `ble_hs_cfg.sm_bonding`, ligado só na janela; com bond gravado, todo Pairing Request fora da janela é recusado no `BLE_STORE_EVENT_FULL`. |
| D4 | 01 | Motivos de desconexão `0x05` recusa, `0x09` ocupado, `0x13` botão. |
| D5 | 01 | Uma sessão autorizada; advertising desligado enquanto ela existe. |
| D6 | 01 | Estado janela/bond em *Manufacturer Data* (`0xFFFF`). |
| D21 | 01 | *Security Request* só para peer que já tem bond. |
| D22 | 01 | Bond confirmado no store antes de valer; falha ao apagar bonds ⇒ apagar o namespace e reiniciar. |
| D7 | 02 | Botão amostrado a 20 ms em tarefa, sem ISR. |
| D8 | 02 | Janela estrita; pareamento já iniciado termina (`late=1`). |
| D9 | 02 | Sem janela automática no boot. |
| D10 | 02 | `PairingTask` no núcleo 0, prioridade 2. |
| D11 | 02 | Tempos em seção `[pairing]` do `.ini`, não em variáveis do SO. |
| D12 | 02 | Botão pressionado no boot bloqueia o detector até soltar. |
| D13–D15 | 03 | `FORMAT v` inalterado; `CONN` = sessão autorizada; `LINK_UP` registra o RPA. |
| D16–D20 | 04 | Anúncio só como dica; pareamento no `ConnectingViewModel`; plano automático; `createBond` explícito no Android; sem novas tentativas após erro de segurança. |
| A1 | 01 | Manter o bit "há bond" no anúncio? Rec.: manter. |
| A2 | 01 | Bonds em claro só no perfil de desenvolvimento da Fase 1 (Arduino puro); o perfil de produção cifra a `nvs` padrão. Rec.: aceitar e documentar o risco residual do perfil dev. |
| A3 | 01 | Endereço do dongle público × RPA. Rec.: público nesta fase. |
| A4 | 01 | *Fork* da NimBLE para recusar `REPEAT_PAIRING`. Rec.: desnecessário (D3 recusa antes). |
| A5 | 02 | Janela automática no primeiro boot. Rec.: não. |
| A6 | 02 | LED no GPIO2 por padrão. Rec.: `-1` no repositório. |
| A7 | 03 | Artefatos de bancada no repositório. Rec.: só logs de texto. |
| A8 | 04 | Camada PSK: remover (com tag) ou isolar como legado. Rec.: remover. |
| A9 | 04 | Card "Segurança do enlace" no "Mais". Rec.: não. |

## Revisão crítica (advisor)

O rascunho foi revisado por um subagente especialista em BLE/NimBLE e
Flutter, que conferiu as afirmações contra o código da NimBLE-Arduino 2.5.0
instalada e do `flutter_blue_plus` 2.3.10 (plugins Android/iOS 9.0.3). Os
três achados mais graves foram reconferidos no código antes de incorporar.
O que mudou:

| # | Achado | Mudança |
|---|---|---|
| 1 | **Bloqueante:** `ble_hs_cfg.sm_sc_only=1` faz o ATT exigir enlace *autenticado* (MITM) em todo atributo com `*_ENC` (`ble_att_svr.c:329-343`); Just Works nunca é autenticado ⇒ nem o celular pareado conseguiria escrever no RX ou inscrever no CCCD. | `sm_sc_only` fica 0. "LESC-only" passa a ser verificado no bond (`sc == 1`, 16 B) e bond legado é apagado (`PAIR_RESULT reason=legacy`). D1 reescrita; B16 e critério 5 da spec 01 ajustados. |
| 2 | Com `BLE_SM_MAX_PROCS = 1`, o *Security Request* mandado a todo `onConnect` ocupa o único procedimento de segurança; um estranho segura o slot e o pedido de LTK do celular pareado fica sem resposta. | *Security Request* só para peer que já tem bond (D21; a identidade já vem resolvida no `onConnect`); `-DMYNEWT_VAL_BLE_SM_MAX_PROCS=3` como reforço [verificar]. Elimina também o diálogo duplicado do Android e o teto de 30 s do SMP antes do toque no iOS. |
| 3 | Pareamento concluído depois da recusa por *timeout* gravaria bond de conexão morta. | Linha nova na tabela de `Rejecting`: bond criado ⇒ apagado (`BOND_DELETE reason=policy`); `PAIR_AUTH_TIMEOUT_MS` documentado como o prazo real; app usa 30 s (< 35 s). |
| 4 | O evento que dispara em todo Pairing Request com o store cheio é `BLE_STORE_EVENT_FULL`, **antes** do `REPEAT_PAIRING` e da alocação do procedimento (`ble_sm.c:1094-1110,1903`). | `BondStoreGuard` recusa `FULL` com a janela fechada (permite com ela aberta, para não haver *lockout*). Fecha o DoS por `REPEAT_PAIRING` com identidade forjada (R3) e o estranho ocupando o SM fora da janela; A4 deixa de ser necessário. `BOND_FULL` ganha `event`/`action`. |
| 5 | Restauração sem LTK no dongle não gera `ENC_CHANGE` (`ble_sm.c:1409-1415`). | Removido `LINK_ENC reason=no_ltk`; POL-17, B10 e B11 reescritos (o dongle vê só *timeout*; quem detecta é o app). |
| 6 | Estranho sem cifra recebe ATT 0x05, não 0x0F (`ble_att_svr.c:345-363`). | Tabelas e B7 corrigidos. |
| 7 | Conexão aceita durante o apagamento (`Wiping`) pareava com bond e era tomada por restauração. | `onLinkUp` em `Wiping` ⇒ recusa `0x09`; pendentes viram "admitidas na janela" ao abrir; POL-27. |
| 8 | Lacunas no app: `notAuthorized` na janela aberta, exceção `deviceIsDisconnected` durante `createBond`, `removeBond` com ACL ativo, limites do iOS. | Linhas novas na máquina de estados e no classificador; `disconnect()` antes do `removeBond`; nota de que no iOS `busy`/`remoteReset` não se distinguem de `notAuthorized`. |
| 9 | NVS cheia: `bonded=1` sem LTK gravada ⇒ *lockout* silencioso. | Verificação do bond no store (`BOND_VERIFY`, `PAIR_RESULT reason=not_persisted`), evento `NVS` no boot, recuperação por `nvs_erase_all("nimble_bond")` + reinício quando `deleteAllBonds` falha (D22). |
| 10–18 | Menores: `LINK_DOWN` local mostra `hci=16`; resolução de RPA funciona com endereço público (plano B de RPA próprio descartado); ordem `enableScanResponse` → `setName`; perm flags do CCCD antes de qualquer `start()`; a biblioteca religa o anúncio em CONNECT falho; listener de GAP único (contrato com a Fase 1); herança de prioridade não transitiva; `READ_ENC` na TX não é gatilho. | Texto correspondente nas specs 01–03. |

Mantido por ter sido confirmado: decisão no `ENC_CHANGE`; gate por
`sm_bonding` (não há caminho para `bonded=1` sem LTK prévia); destino
explícito nas notificações; `advertiseOnDisconnect(false)`; `deleteBond` em
vez de `ble_store_clear`; whitelist inútil com RPA; intervalos de anúncio;
mutex com ações fora do lock; anúncio só como dica no app; sem novas
tentativas após erro de segurança; remoção da PSK do caminho de conexão.
