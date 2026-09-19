# Handoff: suporte a DTC no dongle (esp32-firmware)

> Escrito ao final de uma sessão que implementou leitura de DTC no **simulador**
> (`simulador/ECUSim`, branch `feature/simulador`, commit `d4e6036`). Este
> documento é o ponto de partida para quem for implementar o lado
> correspondente aqui no dongle. Não presuma que quem lê isso tem o contexto
> da conversa que gerou o simulador — está tudo resumido abaixo.

## 1. Contexto: o que já existe do lado do simulador

`simulador/ECUSim` é um sketch Arduino (Uno + MCP2515) que simula uma ECU real
respondendo a requisições OBD-II sobre CAN. Ele já respondia só Modo 01
(dados atuais); agora também responde:

- **Modo 03/07/0A** — lista de DTCs confirmados/pendentes/permanentes.
  Resposta: `<SID+0x40> <count> <DTC_hi> <DTC_lo> ...` (2 bytes por DTC,
  formato padrão SAE J1979 — o valor de 16 bits *é* literalmente "Pxxxx": por
  ex. `0x0301` = P0301).
- **Modo 02** — freeze frame (PIDs fixos capturados no momento do DTC).
  Requisição: `[0x02, PID, frame#]` (`frame#` sempre `0` hoje — não há
  histórico de frames antigos; qualquer outro valor → NRC `0x31`, assim como
  quando não há freeze frame válido armazenado). Resposta:
  `[0x42, PID, frame#, dados...]`.
  **Como saber qual DTC originou o freeze frame**: `PID 0x02` é
  tratado à parte (é o único caso especial, igual o PID 0x01 no Modo 01) —
  em vez de vir do snapshot, ele devolve os 2 bytes do DTC que disparou a
  captura (`ecuState.freezeFrame.originDtc`). Ou seja: primeiro pergunta-se
  `[0x02, 0x02, 0x00]` para descobrir o DTC; as demais leituras (`[0x02, PID,
  0x00]`) trazem o valor congelado daquele PID — exatamente como uma leitura
  de Modo 01, mas fixo no instante da falha.
  **Quais PIDs foram congelados é fixo e não é descobrível hoje**: o
  simulador captura sempre o mesmo conjunto — `FREEZE_FRAME_PIDS` em
  `DTCMap_Definition.h` = `{0x04 (carga do motor), 0x05 (temp. arrefecimento),
  0x0C (RPM), 0x0D (velocidade), 0x0F (temp. ar admissão), 0x11 (posição
  borboleta)}`. Não existe um Modo 02 com `PID 0x00` (bitmap de PIDs
  suportados) como existe no Modo 01 — pedir qualquer PID fora dessa lista
  fixa (inclusive `0x00`) devolve NRC `0x31`. Um cliente real (dongle
  incluso) precisa conhecer essa lista de antemão em vez de descobri-la; se
  isso for uma limitação relevante para o TCC, é mais barato resolver no
  simulador (adicionar suporte a PID 0x00 no Modo 02) do que compensar no
  dongle.
- **Qualquer outro serviço (04+)** → resposta negativa `7F <SID> 0x11`
  (serviceNotSupported) — antes o simulador simplesmente ficava em silêncio.
- **2 ECUs simuladas**: ECM (`0x7E0`/`0x7E8`) e TCM (`0x7E1`/`0x7E9`), ambas
  respondendo ao broadcast `0x7DF` (fan-out — cada uma responde
  independentemente).
- PID 0x01 (monitor status) agora reflete o estado real de MIL/contagem de
  DTC confirmados, em vez de ser um valor cru setável.
- Canal serial paralelo (`DTCUpdateSerialControl.ino`, 9 bytes, prefixo
  `S/R/Z/L/Q` — nunca é hex válido, então não colide com o protocolo de PID
  de 11 bytes) para injetar/remover DTC, forçar MIL e capturar freeze frame
  manualmente, útil para montar cenários de teste determinísticos.

Ver `simulador/README.md` e o diff do commit `d4e6036` (branch
`feature/simulador`) para o detalhamento completo. Setup de build:
`simulador/platformio.ini`, env `uno` (já compila e roda em hardware real).

**Importante — o que NÃO foi validado**: a fragmentação ISO-TP multiframe
(First Frame + Consecutive Frames) do `isotp.send()` (lib `altelch/iso-tp`)
não foi testada em barramento real com um tester que fale ISO-TP de verdade,
porque — motivo do item 2 abaixo — o dongle atual não é capaz de ser esse
tester. A leitura do código-fonte da lib (`iso-tp.cpp`) indica que ela
fragmenta corretamente e tem timeout de 250ms esperando Flow Control (não
trava o `loop()`), mas isso é análise estática, não evidência de bus real.

## 2. Estado atual do dongle: por que ele não fala DTC hoje

Três limitações, todas no caminho `Elm327 → Obd2Can`, impedem qualquer
leitura de DTC hoje — nenhuma delas é sobre CAN físico, todas são de
protocolo/parsing:

1. **`Obd2Can::readPid()` (`src/obd2/real/Obd2Can.cpp`) é Single-Frame-only
   por design.** O próprio comentário do arquivo diz: *"Multi-frame (ISO-TP
   segmented) responses não estão implementados"*. Ele lê exatamente 1 frame
   CAN e trata `data[0] & 0x0F` como tamanho de Single Frame — nunca envia
   Flow Control, então se o simulador mandasse um First Frame (resposta >7
   bytes), o dongle simplesmente ignoraria o frame (não bate o filtro de
   `resp.data[1] == service|0x40`) e daria timeout.
2. **A interface `IObd2::readPid(service, pid, ...)` não tem forma para
   Mode 03/07/0A**, que não têm PID nenhum. Não dá para expressar "leia os
   DTCs confirmados" nessa assinatura sem uma mudança de interface.
3. **`Elm327::process()` (`src/elm327/Elm327.cpp`) exige no mínimo 4 hex
   chars** (`service`+`pid` compactados). Um comando ELM327 real de leitura
   de DTC (`"03"`, sozinho, sem PID) é rejeitado como `?` antes de tentar
   qualquer coisa no CAN.

Ou seja: **nenhuma quantidade de teste no barramento CAN resolve isso** — é
preciso estender essas três camadas primeiro. Isso também significa que a
suíte de cenários do simulador (`LIMPO`/`FALHA_UNICA`/`MULTIFRAME`/etc.) só
poderá ser validada ponta-a-ponta depois dessa extensão.

## 3. O que o mobile-app já espera (não é um chute — já está no código)

`mobile-app/lib/src/data/repositories/dtc_repository_impl.dart` hoje é
`FakeDtcRepositoryImpl`, com este comentário explícito:

> "Placeholder para a leitura real. Quando o datasource ELM327 existir, ele
> lerá os Modos 03 (confirmados)/07 (pendentes)/0A (permanentes) para os
> códigos e o Modo 02 para o congelamento — a troca é só esta classe (...),
> sem tocar em domain/view_model/view."

Ou seja: **toda a UI e o domínio de DTC no app já existem e estão prontos**
(`DtcCode`, `DtcActiveEntry`, `DtcSnapshot`, `DtcStatus`, tela de
diagnóstico) — só falta o datasource real que fale com o dongle via BLE,
espelhando como `obd2_repository_impl.dart`/`elm327_client.dart` já fazem
para PIDs. Duas coisas a notar para quem for além do dongle:

- **`DtcStatus` hoje só tem `confirmed` e `pending`** (`dtc_status.dart`) —
  não tem um caso para Modo 0A (permanente), embora o contrato do
  repository já cite os três modos no docstring. Vai precisar de um terceiro
  valor no enum quando a leitura real chegar lá.
- O contrato de transporte (de `mobile-app/CLAUDE.md`) é **BLE Nordic UART
  + texto ELM327 puro**: RX (`WRITE`) recebe comandos tipo `"03\r"`, TX
  (`NOTIFY`) devolve texto tipo `"43 02 03 01 07 71\r>"`. O formato "Pxxxx"
  aparece só no domínio do app — o dongle deve devolver hex cru no estilo
  ELM327 (é isso que os apps OBD2 reais fazem; a decodificação para
  "P0301" deve acontecer no app, provavelmente em `dtc_catalog.dart` — não
  confirmei se essa lógica já existe lá, vale checar antes de duplicá-la no
  firmware).
- **Atualização (freeze frame voltou a ser modelado no app — checar de novo
  antes de implementar Modo 02 no dongle):** `DtcActiveEntry` e `DtcCode`
  agora têm um campo `freezeFrame: List<DtcFreezeFrameEntry>`
  (`dtc_active_entry.dart`, `dtc_code.dart`), e
  `dtc_freeze_frame_entry.dart` define `DtcFreezeFrameEntry(label, value)`
  — **`label`/`value` já vêm formatados para exibição** (ex.: `('Rotação',
  '2.480 rpm')`), não bytes crus nem grandeza física a converter. Isso é
  exibido em `dtc_detail_sheet.dart`, seção "CONGELAMENTO NO MOMENTO DA
  FALHA" (grid de `StatCard.value`), com o texto vazio-esperado *"o veículo
  só grava o quadro da falha quando o código é confirmado"* — bate com a
  regra do simulador (`captureFreezeFrame` só roda no comando `S`+`ListChar
  'C'`, nunca em pendente).
  **Descompasso real a resolver antes de implementar**: o mock em
  `FakeDtcRepositoryImpl` dá **um freeze frame diferente para cada DTC
  confirmado simultaneamente** (P0301 e P0420 têm listas próprias e
  distintas de grandezas; o DTC de rede `U0121` mostra "Tensão" no lugar de
  "Carga"). O simulador (e o Modo 02 real, na maioria dos veículos) só
  guarda **um** freeze frame por ECU, ligado a um único `originDtc` — se dois
  DTCs estiverem confirmados ao mesmo tempo, só um deles (o de maior
  prioridade, regra misfire/fuel) tem freeze frame de verdade; os outros
  devem vir com `freezeFrame: []`, mesmo estando confirmados. O dado
  variado do mock parece curadoria de design (mostrar a tela "bonita"), não
  uma garantia de protocolo — não dá para fabricar um freeze frame por DTC
  que o hardware não tem. Vale alinhar com quem mantém o app se a tela
  aceita bem "a maioria dos confirmados aparece sem congelamento" antes de
  implementar o Modo 02 no dongle.
  **PIDs do freeze frame também variam no mock** (Rotação/Velocidade/Temp.
  arrefec./Carga para DTCs de motor; Tensão/Rotação/Velocidade/Temp. arrefec.
  para o DTC de rede) — o simulador usa um conjunto **fixo** de 6 PIDs
  independente do DTC (ver item 1). Reforça o ponto acima: o mock é
  ilustrativo, não espelha o que o Modo 02 real consegue entregar hoje.

## 4. Plano sugerido, camada por camada

Seguindo a mesma arquitetura em camadas do projeto (`prompt.md`,
`ICanBus → IObd2 → Elm327 → IConnectivity`), sem inventar uma estrutura nova:

### `can/` — sem mudança
`ICanBus`/`TwaiCan`/`Mcp2515Can` já são só envio/recebimento de frame cru.
Não precisam saber nada de ISO-TP.

### `obd2/real/` — precisa de um cliente ISO-TP de recepção
Hoje `Obd2Can` não faz ISO-TP nenhum (nem para enviar — a requisição já é
Single Frame — nem para receber). Para ler DTC (resposta quase sempre >7
bytes com 2+ códigos) é preciso implementar o lado *receptor* de ISO-TP:
enviar SF de requisição, receber SF ou First Frame; se FF, mandar Flow
Control (`30 00 00 ...`, CTS/BS=0/STmin=0 é suficiente para começar) e
reassemblar os Consecutive Frames até completar, com timeout (sugestão:
mesma ordem de grandeza do `TIMEOUT_FC`/`TIMEOUT_CF` de 250ms usados no
`altelch/iso-tp` do simulador, para simetria).

Sugestão concreta: um `IsoTpClient` novo em `obd2/real/` (não dá para
reusar `altelch/iso-tp` direto — o construtor dele é acoplado a
`MCP_CAN*`, não a `ICanBus`), com uma API mínima tipo:
```cpp
// Envia req (<=7 bytes) e devolve os bytes de payload OBD (SID+dados),
// já reassemblados se tiver vindo em multiframe.
int isoTpRequest(ICanBus* can, uint32_t reqId, uint32_t respId,
                  const uint8_t* req, uint8_t reqLen,
                  uint8_t* outBuf, size_t maxLen, uint32_t timeoutMs);
```

E extensão da interface `IObd2` para expressar um pedido sem PID:
```cpp
// service = 0x03/0x07/0x0A. Devolve os DTCs decodificados (2 bytes cada,
// já no formato de 16 bits — não precisa converter pra "Pxxxx" aqui).
virtual int readDtc(uint8_t service, uint16_t* dtcCodes, size_t maxCount) = 0;
```
(precisa de stub em `Obd2Mock` também, para não quebrar o profile `mock`.)

### `elm327/` — aceitar comando sem PID e formatar resposta multi-valor
`Elm327::process()` precisa reconhecer um comando de 2 hex chars (`"03"`,
`"07"`, `"0A"`) como válido além do caso atual de 4 chars, rotear para
`readDtc()`, e formatar a resposta no estilo ELM327 real: `SID+0x40`,
contagem, DTCs em hex — sem cabeçalho `7E8` a menos que `ATH1` esteja ativo
(mesma lógica condicional que já existe em `formatDataBytes` para Modo 01).

### `connectivity/` — sem mudança
Já é agnóstico ao conteúdo do texto que trafega.

### Modo 02 (freeze frame) — o app já tem UI pronta pra isso (ver item 3)
Confirmado: `DtcActiveEntry.freezeFrame`/`DtcCode.freezeFrame`
(`List<DtcFreezeFrameEntry>`) e a tela `dtc_detail_sheet.dart` já existem e
esperam essa leitura — não é mais "se entrar", é um requisito real do app.
Fluxo do lado do dongle: `readDtc()` devolve os DTCs confirmados → para
descobrir **qual deles** tem freeze frame de verdade, perguntar
`[0x02, 0x02, 0x00]` (devolve o `originDtc` — ver item 1) e comparar com a
lista; **só esse DTC** deve popular `freezeFrame` no domínio do app, os
demais confirmados ficam com `freezeFrame: []` (é a resposta honesta ao
protocolo real, mesmo que a UI do mock hoje mostre mais de um preenchido —
ver ressalva no item 3). Depois, `[0x02, PID, 0x00]` para cada PID de
`{0x04, 0x05, 0x0C, 0x0D, 0x0F, 0x11}` (lista hardcoded no simulador hoje,
sem descoberta via PID 0x00 — replicar esses PIDs aqui, ver item 1) e
formatar cada valor com **label + unidade em pt-BR** (`DtcFreezeFrameEntry`
espera string já pronta, ex.: `"2.480 rpm"`) — reaproveitar as fórmulas e
rótulos de `mobile-app/lib/src/domain/obd2/obd2_pid.dart`
(`Obd2Pid.decode`), já usadas pro painel ao vivo, já que um valor de freeze
frame é exatamente um valor de PID normal, só que congelado no tempo.

## 5. Como testar contra o simulador já pronto

O simulador já expõe um canal de controle serial para montar cenários sem
precisar de um carro de verdade — ver `simulador/ECUSim/DTCUpdateSerialControl.ino`
e `simulador/send_payload.py` (aceita payloads hex crus além dos comandos de
PID por nome). Formato do comando DTC (9 bytes + `\n`):

```
[0] S/R/Z/L/Q   (Set/Remove/Zerar lista/Lamp MIL/força freeze frame)
[1] E/T         (ECM/TCM)
[2] C/P/M/-     (Confirmed/Pending/perManent, '-' quando não se aplica)
[3..6]          DTC em 4 hex chars (ex.: 0301 = P0301)
[7]             '-' (filler)
[8]             '\n'
```

Exemplo: `SEC0301--\n` injeta P0301 como confirmado na ECM. Depois disso,
uma requisição Mode 03 real (`0x7DF` ou `0x7E0`, payload `[01, 03]`) deve
disparar a resposta `43 01 03 01`.

Cenários que valem a pena automatizar como teste de integração dongle↔simulador
(mesma nomenclatura do plano original do simulador): `LIMPO` (0 DTCs → `43
00`), `FALHA_UNICA`/`LIMITE_SF` (1-2 DTCs, ainda cabe em Single Frame),
`MULTIFRAME` (3+ DTCs, força FF/CF — é o teste que hoje não dá pra rodar sem
os dois lados prontos), `MULTI_ECU` (comparar resposta de `0x7E8` vs
`0x7E9` no mesmo broadcast).

## 6. Pendência em aberto que fica para depois (não é bloqueio deste plano)

A validação de multiframe do **lado transmissor** (simulador) com um tester
real de ISO-TP não foi feita nesta sessão — precisamente porque o dongle,
antes deste trabalho, não conseguia ser esse tester. Depois de implementar
o `IsoTpClient` do item 4, essa mesma implementação passa a ser o tester
real que faltava: rodar o cenário `MULTIFRAME` contra o simulador com os
dois lados atualizados valida as duas pontas de uma vez (D2 do plano do
simulador).
