# Relatório: o firmware não está preparado para múltiplas ECUs

> Aprofundamento de um ponto já levantado em `HANDOFF_MULTI_ECU_ISOTP.md`
> (que documenta o bug de dreno de fila já corrigido). Este relatório foca
> no problema estrutural que **fica de pé mesmo depois daquele fix**: o
> firmware nunca soube endereçar uma ECU específica, só broadcast. As
> recomendações abaixo se apoiam em ISO 15765-4 (endereçamento CAN do
> OBD-II) e no conjunto de comandos AT do ELM327 — fontes citadas ao
> final; nenhum parâmetro/comando é inventado.

## 1. Qual é o problema

O OBD-II sobre CAN (ISO 15765-4) define dois modos de endereçamento:

- **Funcional (broadcast)** — ID `0x7DF`. O tester pergunta "pra quem
  souber responder isso" e **qualquer** ECU relevante à emissão responde,
  cada uma no seu próprio ID de resposta física (`0x7E8`–`0x7EF`, 11-bit).
  Isso não é uma falha de protocolo nem uma colisão indesejada — o
  próprio padrão **espera** múltiplas respostas a uma requisição
  funcional (CSS Electronics: *"In this mode, expect several modules to
  respond to each request"*).
- **Física (direcionada)** — o tester manda direto pro ID de requisição
  de UMA ECU (`0x7E0`–`0x7E7`), e só ela responde, no seu ID de resposta
  correspondente (requisição + `0x08`).

O firmware deste dongle (`Obd2Can`, tanto o `readPid()` antigo quanto o
`IsoTpClient` novo) **só implementa o lado emissor do modo funcional**:
toda requisição de qualquer serviço (Modo 01/02/03/07/0A) é sempre
mandada pro broadcast `0x7DF`, nunca pro ID físico de uma ECU específica.
E do lado receptor, ambas as implementações só sabem **capturar uma
resposta por requisição** — a primeira que bater os critérios de match —
e descartam qualquer outra que chegue depois (o `IsoTpClient`, depois do
fix de dreno, descarta de forma limpa; o `readPid()` antigo simplesmente
ignora com `continue` e deixa sobrar na fila). Em nenhum lugar da pilha
(`IObd2`, `Elm327`, `Obd2Can`) existe o conceito de "qual ECU", "quantas
ECUs existem no barramento" ou "coletar todas as respostas a este
pedido".

Ou seja: o firmware trata um barramento que estruturalmente pode ter
2+ respondentes como se só pudesse ter 1 — não por engano de
implementação isolado, mas porque a arquitetura inteira (interface
`IObd2`, formato de retorno de cada método) foi desenhada em torno de
"uma pergunta, uma resposta".

## 2. O que isso causa na prática

### No dongle

- **Nunca entrega a união de dados de várias ECUs** — só o resultado de
  quem respondeu primeiro a cada requisição. Isso já foi demonstrado
  contra o simulador (cenário `multi_ecu` do `hil_dongle_dtc.py`): DTC
  confirmado na ECM e outro na TCM ao mesmo tempo → só o da ECM aparece.
- **A "vencedora" não é uma escolha, é um acidente de timing.** No
  simulador ela é determinística (o laço do `handleCANMessage` processa
  ECM antes de TCM, sempre, na mesma chamada) — mas isso é uma
  particularidade da implementação em software do simulador, não uma
  garantia do protocolo. Num barramento real, cada ECU decide sozinha
  quando responder (latência interna própria), então não há garantia
  de que será sempre a mesma a "vencer" a cada nova sessão do dongle,
  nem estabilidade entre plataformas.
- **Para dados genuinamente por-módulo** (listas de DTC — Modo
  03/07/0A —, freeze frame — Modo 02 —, monitor status — PID 0x01 do
  Modo 01), isso significa perda **silenciosa**: nenhum erro, nenhum
  aviso, o dongle simplesmente nunca chega a ver a resposta da(s)
  outra(s) ECU(s).

### No app

- **Diagnóstico incompleto sem indicação disso.** Um cenário realista:
  MIL acesa por um DTC de motor **e** outro de transmissão ao mesmo
  tempo (é exatamente pra isso que existe P0700 "Transmission Control
  System Malfunction", já no catálogo do simulador) — o app mostraria só
  um dos dois, e a tela de Diagnóstico não tem hoje nenhum jeito de
  saber ou sinalizar "faltou consultar outro módulo".
- **`milOn` pode estar errado** se cada módulo tiver seu próprio bit de
  MIL (PID 0x01 é por-ECU, ver `HANDOFF_MULTI_ECU_ISOTP.md`) e o dongle
  só ler de um.
- **Não há como rotular a origem de um DTC.** Mesmo se o dongle
  passasse a coletar todas as respostas, o domínio do app
  (`DtcComponent`) hoje deriva o componente do **prefixo do código**
  (P/C/B/U), não de qual ECU física respondeu — não tem onde guardar
  "isto veio do módulo em `0x7E9`" mesmo que essa informação chegasse.

### Conectando num carro real

O padrão (SAE J1979/ISO 15031) só exige resposta de módulos
**relevantes à emissão** — tipicamente a ECM (motor) sempre, e a TCM
(transmissão) quando ela afeta emissão (a maioria dos veículos
automáticos modernos) — o espaço de endereços permite até 8 respondentes
(`0x7E8`–`0x7EF`), mas quantos módulos de fato respondem varia por
veículo. Ou seja, o cenário mais provável num carro real é **exatamente
o que o simulador já modela** (1 ou 2 respondentes, ECM e opcionalmente
TCM) — não dezenas de módulos [ABS, airbag, infotainment etc. tipicamente
ficam fora do escopo do Modo 01-0A padrão, em barramentos/serviços de
diagnóstico separados (UDS/ISO 14229), que este dongle não fala]. Isso
não torna o problema menos real: é justamente a combinação mais comum
(ECM + TCM) que o dongle hoje não sabe tratar direito. Riscos concretos:

- Um veículo com TCM respondendo (comum) já é suficiente pra reproduzir
  a perda de dado descrita acima — não é um caso de borda raro.
- Sem controle sobre "qual ECU vence", o comportamento pode **variar
  entre veículos diferentes** (dependendo de qual módulo responde mais
  rápido em cada um) — o mesmo dongle poderia parecer "funcionar
  perfeitamente" num carro e "perder DTC da transmissão" noutro, sem
  nenhuma mudança de código.

### 2.1 E o painel de telemetria (os 43 PIDs do Modo 01)? Funciona num carro real sem indicar a ECU?

Pergunta separada dos riscos acima, e importante não confundir as duas:
**sim, a leitura atual do painel (os 43 PIDs de `Obd2Pid`, todos Modo 01)
funcionaria normalmente num carro real sem precisar endereçar uma ECU
específica** — mas não pelo motivo que uma leitura apressada do §4.2
sugere.

Não é que "ECU não importa" para esses PIDs, como se o protocolo
garantisse isso. É que, na prática, **só a ECM implementa esses PIDs
específicos** (RPM, temperatura, MAF, avanço de ignição etc. — todos de
domínio motor/combustível/emissões). Cada ECU só responde aos PIDs que
ela mesma anuncia como suportados (via o bitmap do PID `0x00`/`0x20`/
`0x40`/.../`0xC0` — mecanismo do próprio padrão, já usado por
`Obd2Datasource.readSupportedPids()`). Numa requisição broadcast
(`0x7DF`) pedindo, por exemplo, PID `0x0C` (RPM):

- Num carro real, a TCM (se existir) **não tem esse dado e não responde
  a esse PID** — ela fica em silêncio, não é um "empate" de duas
  respostas. Só a ECM responde, porque só ela suporta aquele PID.
- É por isso que capturar "a primeira resposta que chegar" funciona bem
  aqui: estruturalmente só existe uma resposta esperada pra esses PIDs
  específicos, num veículo real.

**Isso é diferente de "a TCM responderia idêntico"**, frase usada mais
abaixo (§4.2/§4.4) pra descrever o comportamento do **simulador atual**
— e que só vale pro simulador. O simulador guarda todo PID de valor num
único array global (`PID_Value_Map`, `ECUSim.ino`), compartilhado por
todas as ECUs simuladas, então nele — só nele — pedir esses PIDs pra
`0x7E1` (TCM simulada) devolve os mesmos bytes que pedir pra `0x7E0`
(ECM simulada). Isso não é o que um dongle real veria: numa TCM real, a
resposta não seria "idêntica", seria **ausência de resposta** (a TCM
real não suporta esses PIDs). O resultado prático — dá pra ler sem
indicar ECU — é o mesmo nos dois casos, mas por razões diferentes; tratar
o comportamento do simulador como se fosse a garantia do protocolo seria
um engano fácil de cometer lendo só a tabela abaixo.

**Onde isso deixa de valer, e o dongle já tem o problema real:** PID
`0x01`/`0x41` (monitor status/MIL) e os Modos 02/03/07/0A (freeze frame,
DTCs) **são** PIDs/serviços que uma ECM **e** uma TCM reais podem ambos
suportar e responder de verdade, cada uma com seu próprio dado — é
exatamente aí que a captura de "só a primeira resposta" perde dado
silenciosamente num carro real (ver §2 acima e §4.3). O painel de
telemetria de hoje não sofre disso porque nenhum dos 43 PIDs que ele lê
cai nessa categoria — mas qualquer PID novo adicionado ao painel no
futuro precisa ser checado individualmente contra essa mesma pergunta
("é plausível uma TCM/outro módulo implementar este PID também?") antes
de assumir que broadcast + primeira resposta continua seguro.

**Ressalva de teste**: essa conclusão é analítica (baseada em como o
padrão define suporte a PID e em como scan tools/TCMs reais se
comportam), não algo verificado contra o simulador — o simulador atual
não modela "TCM não suporta este PID" (ele responde tudo do array
global), então o cenário de silêncio da TCM real nunca foi de fato
exercitado em `hil_dongle_dtc.py` nem em nenhum teste deste repositório.

## 3. Como resolver, seguindo o padrão OBD-II

Duas capacidades do protocolo, usadas juntas, resolvem isto — nenhuma
das duas é invenção deste relatório, são como scan tools genéricos
("selecionar ECU" nos menus) já operam:

### 3.1 Descoberta de ECUs — funcional, mas coletando TODAS as respostas

Hoje o `IsoTpClient::request()` retorna assim que a **primeira** resposta
válida chega. Uma requisição funcional de descoberta (ex.: Modo 01 PID
`0x00`, "quais PIDs são suportados" — já usado por
`Obd2Datasource.readSupportedPids()` no app) deveria, em vez disso,
continuar ouvindo até o timeout, **coletando cada resposta distinta por
ID de origem**. Cada ID diferente visto em `0x7E8`–`0x7EF` dentro dessa
janela é uma ECU presente no barramento — é assim que ISO 15765-4 já
espera que o tester se comporte ("expect several modules to respond to
each request"). O mapeamento ID→módulo mais comum (`0x7E8`=ECM,
`0x7E9`=TCM) já é convenção usada no próprio simulador
(`simulador/ECUSim/ECUSim.h`), não precisa ser inventado.

Isso exige uma função nova ao lado de `IsoTp::request()` — algo como
`IsoTp::requestAll()` — que devolve uma lista de `(idOrigem, payload)` em
vez de um único payload, reaproveitando a mesma máquina de recepção SF/FF
já existente, só sem parar no primeiro match.

### 3.2 Endereçamento físico — pra ler uma ECU específica sob demanda

Depois de descobrir os IDs presentes, ler dados **daquela ECU
especificamente** é endereçamento físico puro: mandar a requisição pro
ID de requisição física dela (`idResposta - 0x08` — já é a mesma conta
que `IsoTpClient::physicalRequestIdFor()` usa hoje só pra endereçar o
Flow Control, nunca a requisição inicial) e aceitar resposta só daquele
ID específico (não da faixa inteira `0x7E8`–`0x7EF`).

Isso não exige nada novo no `IsoTpClient` — `IsoTp::request()` já aceita
`reqId`/`respIdMin`/`respIdMax` como parâmetros; hoje `Obd2Can` sempre
passa `reqId=0x7DF, respIdMin=0x7E8, respIdMax=0x7EF`. Pra endereçamento
físico bastaria passar `reqId=0x7E0, respIdMin=respIdMax=0x7E8` (ex.:
só ECM). A mudança real é na camada acima: `IObd2`/`Obd2Can` precisam de
um jeito de expressar "qual ECU" — ou um parâmetro em cada método, ou um
"alvo atual" persistente (ver 3.3).

### 3.3 Superfície ELM327 — `AT SH` (Set Header), já é o padrão de fato

Scan tools reais não inventam uma sintaxe própria pra "escolher ECU" —
usam o comando ELM327 já consagrado **`AT SH <header>`** ("Set Header"),
que define o CAN ID usado nas próximas requisições até ser trocado de
novo (ex.: `AT SH 7E0` → próximas requisições viram físicas, endereçadas
à ECM; `AT SH 7DF`, ou reset via `AT D`/`AT Z`, volta ao funcional). Há
também `AT SR <hh>` ("Set Receive") pra filtrar de qual ID aceitar
resposta, independente do header de envio.

O `Elm327::processAt()` deste dongle **não implementa `AT SH`/`AT SR`
hoje** (lista atual: `ATZ/ATI/ATE0-1/ATH0-1/ATL0-1/ATS0-1/ATSP.../ATDP/
ATPC/ATRV/ATD/ATAT.../ATST.../ATAR/ATAL/ATM0-1/ATCAF0-1`). Adicionar
esses dois comandos é o que fecha a ponta-a-ponta: o app (ou qualquer
app OBD2 genérico, sem precisar conhecer nada específico deste TCC)
ganharia a mesma capacidade de "selecionar ECU" que qualquer scan tool
comercial já oferece, usando exatamente o protocolo que já fala.

### 3.4 Resumo da mudança de arquitetura necessária

1. `IsoTpClient`: nova função de coleta múltipla (`requestAll`), ao lado
   da `request()` existente (que continua servindo pra quando só uma
   resposta é esperada/suficiente).
2. `Obd2Can`/`IObd2`: cada método (`readPid`, `readDtc`,
   `readFreezeFramePid`) precisa saber o alvo atual (ECU específica ou
   "todas") — ou como parâmetro explícito, ou como estado interno
   (`selectEcu(id)`) espelhando como `AT SH` funciona de verdade no
   protocolo (estado que persiste até ser trocado, não parâmetro por
   chamada) — essa segunda opção é a que fica mais alinhada ao padrão.
3. `Elm327`: implementar `AT SH`/`AT SR`, roteando pro `selectEcu()`
   de `IObd2`.
4. Domínio (opcional, só se o app for expor "qual módulo" na UI):
   `DtcComponent`/`DtcActiveEntry` ganhariam um jeito de carregar a ECU
   de origem, hoje inexistente.

Nada disso foi implementado — fica como recomendação, complementar ao
que já está registrado em `HANDOFF_MULTI_ECU_ISOTP.md` (que resolveu só
a instabilidade/dado obsoleto, não a ausência de endereçamento).

## 4. Referência: DTCs, PIDs e MIL por categoria/ECU, conforme o padrão

Pedido explícito: mapear "todos os DTCs, PIDs e MILs que podem ter em
todas as ECUs". Duas ressalvas importantes antes da tabela, pra não
prometer uma precisão que o próprio padrão não tem:

- **DTC não é uma lista fechada e enumerável.** O padrão (SAE J2012)
  define uma **estrutura** de código (letra + dígito genérico/fabricante
  + dígito de subsistema + 2 dígitos específicos) — não uma lista fixa
  de "todos os códigos possíveis". A maior parte do espaço de códigos é
  **específica de fabricante** (não documentada publicamente pela SAE).
  O que dá pra mapear com precisão, e é o que a tabela abaixo faz, é a
  **estrutura** e as **faixas reservadas por subsistema**.
- **PID não pertence a uma ECU por definição do padrão.** SAE J1979 define
  o que cada PID **significa**, não quem deve respondê-lo — cada ECU
  anuncia dinamicamente quais PIDs suporta (bitmask do PID `0x00`/`0x20`/
  `0x40`/`0x60`/`0x80`/`0xA0`/`0xC0`, já usado por
  `Obd2Datasource.readSupportedPids()` no app). "Qual ECU responde qual
  PID" é descoberto por veículo, em runtime — não é uma tabela fixa da
  norma. O que É fixo é o **significado** de cada PID, e por convenção
  (não por exigência normativa) a imensa maioria dos PIDs de Modo 01 é
  respondida pela ECM, por serem grandezas do motor/emissões.

### 4.1 DTC — estrutura do código (SAE J2012)

| Posição | Valores | Significado |
| --- | --- | --- |
| 1º caractere | `P` `C` `B` `U` | Powertrain (motor/transmissão) · Chassis (freios/suspensão/direção) · Body (carroceria/conforto) · Network (comunicação entre módulos) |
| 2º caractere (dígito) | `0`–`3` | `0`/`2` = código genérico SAE (definição comum a todos os fabricantes) · `1`/`3` = específico de fabricante |
| 3º caractere (só p/ `P`) | `0`–`9` | Subsistema — ver faixas abaixo |
| 4º-5º caracteres | `00`–`99` | Falha específica dentro do subsistema |

**Faixas de subsistema dos códigos `P0xxx` (genéricos SAE)** — únicas
com repartição numérica padronizada e publicamente documentada:

| Faixa | Subsistema | ECU típica (convenção, não exigência normativa) |
| --- | --- | --- |
| `P0100`–`P0199` | Medição de ar/combustível (MAF, MAP, TPS) | ECM |
| `P0200`–`P0299` | Circuito dos injetores | ECM |
| `P0300`–`P0399` | Ignição/falha de combustão (misfire) | ECM |
| `P0400`–`P0499` | Controle adicional de emissões (EGR, EVAP, catalisador) | ECM |
| `P0500`–`P0599` | Velocidade do veículo / controle de marcha lenta | ECM |
| `P0600`–`P0699` | Módulo de computador / circuitos de saída | ECM |
| `P0700`–`P0999` | Transmissão | TCM |

`C`, `B` e `U` têm bem menos códigos genéricos SAE publicamente
definidos que `P` — a maior parte do espaço `C`/`B` é de fabricante, e
por isso não são tipicamente expostas nos Modos 03/07/0A de OBD-II
"clássico" (ver §2 — ABS/airbag/carroceria costumam ficar num
diagnóstico separado, UDS/ISO 14229, fora do que este dongle fala).
Códigos `U` (rede) são o caso onde qualquer módulo pode reportar,
tipicamente por perda de comunicação com outro módulo específico — é
por isso que `U0100`/`U0121` (já no catálogo do app,
`dtc_catalog.dart`) fazem sentido como "ECM"/"ABS" mesmo sendo rede.

### 4.2 PID (Modo 01) — o que já está verificado neste repositório

A lista completa oficial (SAE J1979/J1979-2) passa de 150 PIDs e segue
crescendo a cada revisão — reproduzi-la inteira aqui arriscaria copiar
nomes que eu não consegui confirmar por uma fonte confiável (o pedido
explícito é não inventar). Em vez disso, a tabela abaixo é o **conjunto
já implementado e testado** no simulador
(`simulador/ECUSim/PIDMap_Definition.h`, que define o comprimento em
bytes de cada PID de `0x00` a `0xFF` — `0` = não suportado) e espelhado
no app (`mobile-app/lib/src/domain/obd2/obd2_pid.dart`) — é um
subconjunto real do padrão, não uma lista à parte:

Todo PID abaixo é exatamente o que existe hoje em `Obd2Pid`
(`obd2_pid.dart`) — conferido linha a linha no arquivo, nenhum
adicionado ou removido. A coluna "ECU a pedir" não é opinião: é o que o
**código-fonte do simulador** (`PIDMessageBuilder.ino::fillValueBytes()`)
faz de fato — ele lê todo PID de valor do array **global**
`PID_Value_Map`, ignorando qual `ecuState` foi passado. Ou seja, pedir
qualquer um desses PIDs pra TCM (`0x7E1`) devolveria, no simulador atual,
**o mesmo byte a byte** que pedir pra ECM (`0x7E0`) — comprovável lendo
o próprio código, não é suposição. **Isso é uma particularidade deste
simulador** (array global), não um comportamento de carro real — ver
§2.1 para a explicação de por que a leitura funciona sem endereçar ECU
também num veículo real, mas por um motivo diferente (só a ECM suporta
esses PIDs, a TCM real não responderia "idêntico", responderia nada).

| PID | Nome (`Obd2Pid`) | ECU a pedir (comprovado no simulador) |
| --- | --- | --- |
| `0x04` | Carga do motor | ECM (TCM responderia idêntico — `PID_Value_Map` global) |
| `0x05` | Temp. líquido de arrefecimento | ECM (idem) |
| `0x06` | Ajuste combustível curto prazo (B1) | ECM (idem) |
| `0x07` | Ajuste combustível longo prazo (B1) | ECM (idem) |
| `0x0A` | Pressão de combustível (gauge) | ECM (idem) |
| `0x0B` | Pressão absoluta coletor admissão | ECM (idem) |
| `0x0C` | Rotação do motor (RPM) | ECM (idem) |
| `0x0D` | Velocidade do veículo | ECM (idem) |
| `0x0E` | Avanço de ignição | ECM (idem) |
| `0x0F` | Temp. ar de admissão | ECM (idem) |
| `0x10` | Fluxo de massa de ar (MAF) | ECM (idem) |
| `0x11` | Posição do acelerador | ECM (idem) |
| `0x14` | Sonda lambda 1 (tensão) | ECM (idem) |
| `0x15` | Sonda lambda 2 pós-catalisador (tensão) | ECM (idem) |
| `0x1F` | Tempo de funcionamento do motor | ECM (idem) |
| `0x21` | Distância percorrida com MIL acesa | ECM (idem) |
| `0x2C` | EGR comandada | ECM (idem) |
| `0x2D` | Erro de EGR | ECM (idem) |
| `0x2E` | Purga evaporativa comandada | ECM (idem) |
| `0x2F` | Nível do tanque de combustível | ECM (idem) |
| `0x30` | Ciclos de aquecimento desde reset | ECM (idem) |
| `0x31` | Distância desde reset de códigos | ECM (idem) |
| `0x32` | Pressão de vapor evaporativo | ECM (idem) |
| `0x33` | Pressão barométrica absoluta | ECM (idem) |
| `0x3C` | Temp. catalisador (B1S1) | ECM (idem) |
| `0x42` | Tensão do módulo de controle | ECM (idem) |
| `0x43` | Carga absoluta do motor | ECM (idem) |
| `0x44` | Razão ar-combustível comandada (λ) | ECM (idem) |
| `0x45` | Posição relativa do acelerador | ECM (idem) |
| `0x46` | Temp. ar ambiente | ECM (idem) |
| `0x47` | Posição da borboleta B | ECM (idem) |
| `0x49` | Posição do pedal do acelerador D | ECM (idem) |
| `0x4C` | Atuador de borboleta comandado | ECM (idem) |
| `0x4D` | Tempo com MIL acesa | ECM (idem) |
| `0x4E` | Tempo desde reset de códigos | ECM (idem) |
| `0x52` | Percentual de etanol no combustível | ECM (idem) |
| `0x5A` | Posição relativa do pedal do acelerador | ECM (idem) |
| `0x5C` | Temp. óleo do motor | ECM (idem) |
| `0x5D` | Temporização de injeção de combustível | ECM (idem) |
| `0x5E` | Taxa de consumo de combustível | ECM (idem) |
| `0x61` | Torque demandado pelo motorista | ECM (idem) |
| `0x62` | Torque real do motor | ECM (idem) |
| `0x63` | Torque de referência do motor | ECM (idem) |

**Nenhum PID de `Obd2Pid` precisa ser pedido da TCM hoje** — os 43 são,
sem exceção, PIDs de domínio motor/combustível/emissões (bate com as
faixas de subsistema do §4.1: fuel/air, ignição, emissões, velocidade,
computador — todas "powertrain-motor", não "powertrain-transmissão").
Isso é consistente com o padrão em geral, não só uma coincidência do
simulador: transmissões reais expõem pouquíssimos PIDs do conjunto
genérico SAE — a maioria do dado de câmbio, quando existe, é PID
específico de fabricante, fora do que `Obd2Pid` cobre.

**PID fora de `Obd2Pid` que passaria a ser necessário** — `0x01`
(monitor status) e seu equivalente `0x41` (mesmo formato, "neste ciclo
de condução") **são os únicos PIDs genuinamente por-ECU** no simulador
(via `computeMonitorStatusPID()`, que usa `ecuState.mil`/`confirmedCount`
— não o array global). Pra implementar MIL corretamente (ver §4.3), o
app precisaria ler PID `0x01` de **ambas** as ECUs (ECM e TCM),
fisicamente endereçadas, e combinar os resultados — nunca vai bastar
pedir de uma só.

### 4.3 MIL — um bit por ECU, uma lâmpada só no painel

MIL ("Malfunction Indicator Lamp", a luz de injeção) **não é um dado
único e global no protocolo** — é um bit dentro da resposta do PID `0x01`
(e `0x41`) de **cada** ECU relevante à emissão, junto com a contagem de
DTCs confirmados daquela ECU (é exatamente o que
`computeMonitorStatusPID()` do simulador já modela, por-ECU:
`ecmState.mil`/`tcmState.mil` são campos independentes). O veículo tem
**uma** lâmpada física no painel, que se acende se **qualquer** ECU
relevante à emissão estiver comandando-a — ou seja, o "MIL global" que o
app expõe (`DtcSnapshot.milOn`) é, de fato, uma união lógica de "MIL de
cada ECU", não um valor que uma única leitura consiga capturar sem
consultar todas as ECUs (reforça o problema do §2: hoje só se lê o PID
`0x01` de quem responder primeiro, então `milOn` também sofre da mesma
lacuna).

### 4.4 Tabela de decisão — pronta pra guiar a implementação por-ECU

Resumo direto de tudo acima, no formato que serve de checklist:

| O que o app lê | De qual(is) ECU(s) pedir | Como combinar |
| --- | --- | --- |
| Qualquer um dos 43 PIDs listados em §4.2 (todo o painel de telemetria hoje) | Só **ECM** (`0x7E0`/`0x7E8`) | Não precisa combinar — num carro real, só a ECM suporta/responde esses PIDs (TCM fica em silêncio); no simulador atual, a TCM devolveria os mesmos bytes por causa do array global `PID_Value_Map` (ver §2.1) — comportamento do simulador, não do protocolo |
| PID `0x01`/`0x41` (monitor status/MIL) — **não implementado ainda**, ver §5 do handoff do app | **ECM e TCM**, uma requisição física pra cada | `milOn` = OR lógico do bit de MIL de cada uma; contagem de confirmados = soma, se for exibida |
| Modo 03/07/0A (listas de DTC) | **ECM e TCM**, uma requisição por serviço por ECU (6 requisições no total pra ler os 3 modos das 2) | União das listas — um DTC de cada ECU é um `DtcActiveEntry` próprio, nenhum se sobrepõe |
| Modo 02 (freeze frame) | **ECM e TCM**, cada uma tem o seu (`readFreezeFramePid` já é por-ECU por natureza — só falta endereçar fisicamente em vez de broadcast) | Não é união — cada freeze frame fica associado ao DTC de origem daquela ECU especificamente (mesma regra do §4 do handoff do app, só que agora aplicada 2× — uma vez por ECU) |

Isso pressupõe o endereçamento físico do §3 implementado (`reqId=0x7E0`
pra ECM, `reqId=0x7E1` pra TCM, `respIdMin=respIdMax` = a resposta
correspondente) — sem isso, "pedir da ECM" e "pedir da TCM" não são
operações distintas, é sempre o mesmo broadcast pra ambas.

## 5. PIDs do Modo 01 fora do que `Obd2Pid` mapeia hoje, que podem vir de OUTRA ECU numa implementação real

Pergunta diferente da §4.2: lá o assunto era "dos 43 PIDs que o app já
lê, qual ECU pedir" (resposta: todos ECM, comprovado no simulador atual).
Aqui o assunto é o oposto — **dentre os PIDs do Modo 01 que o app ainda
não mapeia**, quais têm nome/definição oficial (SAE J1979/J1979-2)
indicando que vêm de um módulo **diferente** do motor, pra você já saber
quais mapear quando a implementação de múltiplas ECUs de verdade
acontecer (hardware + software). Fonte: tabela de PIDs da Wikipedia
(mirror comum da SAE J1979/J1979-2, ver Fontes) cruzada com
`PIDMap_Definition.h` (`_PID_BYTE_LENGTH`, que já reserva esses PIDs como
"definidos" no comprimento em bytes, mesmo sem fórmula implementada).

### 5.1 Confirmados como não-motor — candidatos reais pra outra ECU

| PID | Nome oficial | Módulo de origem (convenção) |
| --- | --- | --- |
| `0x5B` | Hybrid battery pack remaining life (%) | Controlador de bateria híbrida/EV |
| `0x9A` | Hybrid/EV Vehicle System Data, Battery, Voltage | Controlador de bateria híbrida/EV |
| `0xA4` | **Transmission Actual Gear** | **TCM** — este é nomeado explicitamente como transmissão pelo próprio padrão, não é inferência |
| `0xA9` | ABS Disable Switch State | Módulo de ABS/chassi — nem é powertrain, então nem apareceria via um Modo 01 broadcast em muitos veículos (ABS costuma ficar fora do escopo OBD-II clássico, ver §2) |

Se o seu hardware real tiver um veículo híbrido/elétrico ou quiser expor
a marcha engatada, são esses 4 os PIDs a mapear em `Obd2Pid` — e, pela
mesma lógica do §3, precisam ser pedidos fisicamente do módulo
correspondente (não dá pra saber se vêm da TCM/bateria por broadcast sem
antes descobrir os IDs presentes, §3.1).

### 5.2 PIDs "não mapeados" mas que continuam sendo domínio motor (não são candidatos a outra ECU)

A maior parte do intervalo `0x67`–`0x93` (temperatura de admissão dupla,
EGR/turbo/DPF/NOx/aftertreatment diesel, temperatura de escapamento por
banco, etc.) segue sendo dado de **motor/emissões**, só que de sensores
que o simulador/mock atual não implementa — não são candidatos a "outra
ECU" pela definição oficial. Única exceção parcial: em caminhões/veículos
pesados, alguns desses PIDs de pós-tratamento diesel (DPF/SCR/NOx,
`0x7A`-`0x8B`, `0x9B`) podem estar num controlador de aftertreatment
separado do ECM principal — mas isso é uma particularidade de aplicações
pesadas, não do escopo automotivo leve deste TCC, então não deveria
entrar no seu mapeamento sem uma necessidade concreta.

### 5.3 Divergência encontrada, não resolvida — não inventei uma resposta

A busca na Wikipedia não retornou nome pra `0xAA`–`0xBF` (relatou "não
existem" nessa tabela) — só que `PIDMap_Definition.h` reserva esse
intervalo inteiro (`0xA4`–`0xBF`) como PIDs de 4 bytes **definidos**,
contíguo ao `0xA4` (Transmission Actual Gear) que acabei de confirmar.
As duas fontes não batem, e não vou inventar nomes pra fechar essa
lacuna — antes de mapear qualquer coisa nesse intervalo no app, valeria
conferir a norma primária (SAE J1979/J1979-2, não um mirror de
terceiros) ou, mais barato, ler o que o `_PID_BYTE_LENGTH` do próprio
simulador já reserva e decidir por implementação própria, já que ele foi
a fonte usada pra construir o resto deste projeto.

### 5.4 Pergunta explícita: duas ECUs podem responder o mesmo número de PID com dados totalmente diferentes?

Sim — mas só fora da lista oficialmente publicada pela SAE. A resposta
não é uniforme, e vale separar os dois regimes:

- **Dentro do catálogo oficial (SAE J1979/J1979-2 — a mesma fonte usada
  em todo este relatório, §4.2 e §6):** o padrão define **um** significado
  global por número de PID, não um significado "por ECU". Um PID
  catalogado nunca deveria ter dois significados diferentes dependendo
  de quem responde — mesmo no único caso genuinamente por-ECU deste
  relatório (PID `0x01`/`0x41`, monitor status, §4.3), a **estrutura**
  (bit de MIL + contagem de confirmados) é idêntica em qualquer ECU que
  a reporte; muda o valor, não o significado dos bytes. Então, pros PIDs
  já mapeados neste relatório, colisão de significado **não é esperada**.
- **Fora do catálogo oficial — PIDs específicos de fabricante — sim, é
  um risco real, documentado, não hipotético.** A grande maioria dos PIDs
  que existem de fato num veículo moderno nem está na lista SAE: segundo
  a Wikipedia (mesma fonte já citada), *"the majority of all OBD-II PIDs
  in use are non-standard [...] there is relatively minor overlap between
  vehicle manufacturers for these non-standard PIDs"* — ou seja,
  **cada fabricante define seus próprios PIDs de forma independente**, e
  o "overlap" entre eles é descrito como "relativamente pequeno", não
  como "zero". Isso significa que o mesmo byte de PID pode, sim, ser
  usado por fabricantes diferentes (e, dentro do mesmo fabricante, por
  módulos diferentes cujo firmware nem sempre é desenvolvido de forma
  centralizada) pra representar dados completamente não relacionados.
  Não existe uma autoridade central coordenando esse espaço fora do que a
  SAE publica.
- **Exemplo concreto já encontrado nesta própria pesquisa, não
  hipotético:** o intervalo `0xAA`–`0xBF` (§5.3 acima) é exatamente essa
  zona cinzenta — reservado no simulador deste projeto, mas sem nome
  oficial na fonte SAE consultada. Se, num veículo real, duas ECUs
  diferentes respondessem a um PID nessa faixa, **não há garantia
  nenhuma de que signifiquem a mesma coisa** — isso teria que ser
  verificado por veículo, contra a norma primária ou a documentação do
  fabricante, nunca assumido.

**Implicação prática pra este projeto:** a suposição "ler um PID
funciona sem endereçar ECU" (§2.1) só vale com segurança pros PIDs
efetivamente catalogados pela SAE — é o que sustenta a tabela do §6.
Se a descoberta multi-ECU do §3.1, quando implementada, revelar o
**mesmo** número de PID como suportado por duas ECUs diferentes **fora**
do intervalo já catalogado (§4.2/§6), isso deveria ser tratado como um
alerta a investigar por veículo — nunca unificado silenciosamente como
se fosse o mesmo dado, e nunca deveria ganhar uma fórmula de decodificação
assumida sem confirmação por veículo.

## 6. Tabela única — todos os PIDs de Modo 01 "pegáveis" pro painel, simulador × app

Pedido explícito: uma única tabela com todos os PIDs candidatos ao
painel, destacando o que falta catalogar no simulador e/ou no app. Ela
consolida (e substitui, como referência única) o que estava espalhado
entre §4.2 e §5.

**Critério de entrada** — o mesmo que `Obd2Pid` já documenta no próprio
código (`obd2_pid.dart`, doc do enum): só entra PID que decodifica pra
**um único valor físico contínuo** (`decode: List<int> -> double`).
Ficam de fora, por definição — não é lacuna, é o contrato do painel:
PIDs de bitmap (`0x00`/`0x20`/`0x40`/`0x60`/`0x80`/`0xA0`/`0xC0` —
"quais PIDs são suportados", e `0x13`/`0x1D` — sondas presentes), PIDs de
status/categóricos (`0x01`/`0x41` monitor status, `0x02` DTC do freeze
frame, `0x03` status do sistema de combustível, `0x12` ar secundário,
`0x1C` padrão OBD, `0x1E` status de entrada auxiliar, `0x51` tipo de
combustível, `0x5F` requisito de emissão, `0x65`/`0x92` bitmaps/controle),
tetos de calibração (`0x4F`/`0x50`, máximos de fundo de escala) e
estruturas compostas/multi-valor (`0x64` dado de torque composto, `0x66`
MAF multi-sensor, e a faixa `0x67`–`0x93` de diesel/aftertreatment já
discutida no §5.2 — nenhuma delas é um valor físico único). Essas ficam
de fora da tabela abaixo de propósito, não por esquecimento.

**Fonte da coluna "Nome oficial"**: tabela de PIDs do Modo 01 da
Wikipedia (mirror de SAE J1979/J1979-2, ver Fontes), a mesma já usada no
§5. **Fonte da coluna "Simulador"**: `_PID_BYTE_LENGTH[]` em
`PIDMap_Definition.h` (comprimento > 0 = simulador declara o PID como
suportado no bitmap `0x00`/`0x20`/etc.; note que isso é diferente de "o
simulador gera um valor fisicamente realista" — o array `PID_Value_Map`
que guarda o valor é zerado por padrão e só é alimentado onde há lógica
de ciclo de condução implementada, o que hoje cobre exatamente os PIDs
já marcados "✅ App" abaixo). **Fonte da coluna "App"**: `Obd2Pid`
(`obd2_pid.dart`), linha a linha.

| PID | Nome oficial | Bytes | Domínio (convenção) | Simulador | App | Status |
| --- | --- | --- | --- | --- | --- | --- |
| `0x04` | Calculated engine load | 1 | ECM | ✅ | ✅ | Completo |
| `0x05` | Engine coolant temperature | 1 | ECM | ✅ | ✅ | Completo |
| `0x06` | Short term fuel trim — Bank 1 | 1 | ECM | ✅ | ✅ | Completo |
| `0x07` | Long term fuel trim — Bank 1 | 1 | ECM | ✅ | ✅ | Completo |
| `0x08` | Short term fuel trim — Bank 2 | 1 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x09` | Long term fuel trim — Bank 2 | 1 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x0A` | Fuel pressure (gauge) | 1 | ECM | ✅ | ✅ | Completo |
| `0x0B` | Intake manifold absolute pressure | 1 | ECM | ✅ | ✅ | Completo |
| `0x0C` | Engine speed (RPM) | 2 | ECM | ✅ | ✅ | Completo |
| `0x0D` | Vehicle speed | 1 | ECM | ✅ | ✅ | Completo |
| `0x0E` | Timing advance | 1 | ECM | ✅ | ✅ | Completo |
| `0x0F` | Intake air temperature | 1 | ECM | ✅ | ✅ | Completo |
| `0x10` | MAF air flow rate | 2 | ECM | ✅ | ✅ | Completo |
| `0x11` | Throttle position | 1 | ECM | ✅ | ✅ | Completo |
| `0x14` | Oxygen Sensor 1 (tensão + ajuste) | 2 | ECM | ✅ | ✅ | Completo |
| `0x15` | Oxygen Sensor 2 (tensão + ajuste) | 2 | ECM | ✅ | ✅ | Completo |
| `0x16` | Oxygen Sensor 3 | 2 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x17` | Oxygen Sensor 4 | 2 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x18` | Oxygen Sensor 5 | 2 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x19` | Oxygen Sensor 6 | 2 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x1A` | Oxygen Sensor 7 | 2 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x1B` | Oxygen Sensor 8 | 2 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x1F` | Run time since engine start | 2 | ECM | ✅ | ✅ | Completo |
| `0x21` | Distance traveled with MIL on | 2 | ECM | ✅ | ✅ | Completo |
| `0x22` | Fuel Rail Pressure (rel. manifold vacuum) | 2 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x23` | Fuel Rail Gauge Pressure (diesel/GDI) | 2 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x24`–`0x2B` | Oxygen Sensor 1–8 (wideband, λ+tensão — formato alternativo a `0x14`-`0x1B`) | 4 cada | ECM | ✅ | ❌ | **Gap: só no simulador** (8 PIDs) |
| `0x2C` | Commanded EGR | 1 | ECM | ✅ | ✅ | Completo |
| `0x2D` | EGR Error | 1 | ECM | ✅ | ✅ | Completo |
| `0x2E` | Commanded evaporative purge | 1 | ECM | ✅ | ✅ | Completo |
| `0x2F` | Fuel Tank Level Input | 1 | ECM | ✅ | ✅ | Completo |
| `0x30` | Warm-ups since codes cleared | 1 | ECM | ✅ | ✅ | Completo |
| `0x31` | Distance traveled since codes cleared | 2 | ECM | ✅ | ✅ | Completo |
| `0x32` | Evap. System Vapor Pressure | 2 | ECM | ✅ | ✅ | Completo |
| `0x33` | Absolute Barometric Pressure | 1 | ECM | ✅ | ✅ | Completo |
| `0x34`–`0x3B` | Oxygen Sensor 1–8 (wideband, λ+corrente — formato alternativo) | 4 cada | ECM | ✅ | ❌ | **Gap: só no simulador** (8 PIDs) |
| `0x3C` | Catalyst Temperature — Bank 1, Sensor 1 | 2 | ECM | ✅ | ✅ | Completo |
| `0x3D` | Catalyst Temperature — Bank 2, Sensor 1 | 2 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x3E` | Catalyst Temperature — Bank 1, Sensor 2 | 2 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x3F` | Catalyst Temperature — Bank 2, Sensor 2 | 2 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x42` | Control module voltage | 2 | ECM | ✅ | ✅ | Completo |
| `0x43` | Absolute load value | 2 | ECM | ✅ | ✅ | Completo |
| `0x44` | Commanded Air-Fuel Equivalence Ratio (λ) | 2 | ECM | ✅ | ✅ | Completo |
| `0x45` | Relative throttle position | 1 | ECM | ✅ | ✅ | Completo |
| `0x46` | Ambient air temperature | 1 | ECM | ✅ | ✅ | Completo |
| `0x47` | Absolute throttle position B | 1 | ECM | ✅ | ✅ | Completo |
| `0x48` | Absolute throttle position C | 1 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x49` | Accelerator pedal position D | 1 | ECM | ✅ | ✅ | Completo |
| `0x4A` | Accelerator pedal position E | 1 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x4B` | Accelerator pedal position F | 1 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x4C` | Commanded throttle actuator | 1 | ECM | ✅ | ✅ | Completo |
| `0x4D` | Time run with MIL on | 2 | ECM | ✅ | ✅ | Completo |
| `0x4E` | Time since trouble codes cleared | 2 | ECM | ✅ | ✅ | Completo |
| `0x52` | Ethanol fuel % | 1 | ECM | ✅ | ✅ | Completo |
| `0x53` | Absolute Evap system Vapor Pressure | 2 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x54` | Evap system vapor pressure (sinalizado) | 2 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x55` | STFT secundário O2, banco 1/3 | 2 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x56` | LTFT secundário O2, banco 1/3 | 2 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x57` | STFT secundário O2, banco 2/4 | 2 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x58` | LTFT secundário O2, banco 2/4 | 2 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x59` | Fuel rail absolute pressure | 2 | ECM | ✅ | ❌ | **Gap: só no simulador** |
| `0x5A` | Relative accelerator pedal position | 1 | ECM | ✅ | ✅ | Completo |
| `0x5B` | Hybrid battery pack remaining life (%) | 1 | Bateria híbrida/EV | ✅ | ❌ | **Gap — e não é ECM** (ver §5.1) |
| `0x5C` | Engine oil temperature | 1 | ECM | ✅ | ✅ | Completo |
| `0x5D` | Fuel injection timing | 2 | ECM | ✅ | ✅ | Completo |
| `0x5E` | Engine fuel rate | 2 | ECM | ✅ | ✅ | Completo |
| `0x61` | Driver's demand engine torque (%) | 1 | ECM | ✅ | ✅ | Completo |
| `0x62` | Actual engine torque (%) | 1 | ECM | ✅ | ✅ | Completo |
| `0x63` | Engine reference torque | 2 | ECM | ✅ | ✅ | Completo |
| `0x9A` | Hybrid/EV System Data — Battery Voltage | 6 | Bateria híbrida/EV | ❌ | ❌ | **Gap total — nem reservado no simulador** |
| `0xA4` | Transmission Actual Gear | 4 | **TCM** | ✅ | ❌ | **Gap — e não é ECM** (ver §5.1) |
| `0xA6` | Odometer | 4 | Não especificado pela norma — tipicamente instrumentação/BCM, fora do domínio motor | ✅ | ❌ | **Gap — achado novo, não estava no §5 original** |
| `0xA9` | ABS Disable Switch State | 4 | Módulo ABS/chassi (fora do escopo OBD-II clássico na maioria dos veículos) | ✅ | ❌ | **Gap — e não é ECM** (ver §5.1) |

**Leitura da tabela**: 43 PIDs já estão completos (simulador + app, é o
conjunto inteiro de `Obd2Pid` hoje). 39 PIDs (contando `0x24`-`0x2B` e
`0x34`-`0x3B` como 8 cada, e `0x16`-`0x1B` como 6) já têm comprimento
reservado no simulador — logo já apareceriam no bitmap de PIDs suportados
e responderiam a uma requisição — mas não têm implementação real de dado
por trás nem lugar no app: são o próximo lote óbvio a mapear, todos ainda
domínio motor/ECM, sem precisar de endereçamento físico (ver §2.1). Os 5
restantes (`0x5B`, `0x9A`, `0xA4`, `0xA6`, `0xA9`) são os únicos
candidatos confirmados de **outra ECU/módulo** — esses só fazem sentido
depois do endereçamento físico do §3, e `0x9A` precisa primeiro ganhar
comprimento no simulador (hoje é o único da lista que nem aparece no
bitmap de PIDs suportados).

### 6.1 Na prática, como se leria um desses 5 PIDs? (`0x5B`, `0x9A`, `0xA4`, `0xA6`, `0xA9`)

Vale separar duas perguntas que parecem uma só: **descobrir** que o PID
existe vs. **ler** o valor dele. O firmware hoje falha na primeira e é
incerto na segunda — nenhuma das duas funciona de ponta a ponta sem o
que falta implementar no §3.

- **Descoberta está quebrada hoje, especificamente pra esses PIDs.** O
  app descobre PIDs suportados mandando Mode 01 PID `0x00`/`0x20`/etc.
  em broadcast e lendo o bitmap devolvido
  (`Obd2Datasource.readSupportedPids()`). Mas `IsoTpClient::request()`
  para na primeira resposta (§3.1) — num carro real com ECM+TCM, o app só
  vê o bitmap de quem respondeu primeiro (tipicamente a ECM), que não
  marca `0xA4` como suportado, porque quem suporta é a TCM. **O app nunca
  fica sabendo que o PID existe**, mesmo que o veículo o suporte. Isso só
  se resolve com `requestAll()` (§3.1) coletando o bitmap de cada ECU
  separadamente.
- **Leitura do valor, uma vez sabendo que existe, é mais sutil.** Como
  cada um desses 5 PIDs é tipicamente implementado por um único módulo
  (marcha só na TCM, % de bateria só no controlador híbrido/EV), pela
  mesma lógica do §2.1 uma leitura em broadcast **provavelmente**
  funcionaria sem endereçar ECU — só quem suporta responderia. Mas essa
  convenção é mais fraca aqui do que pros PIDs de motor: "motor = ECM" é
  praticamente universal; "marcha = TCM" e as demais já dependem mais do
  fabricante, e o §5.3 já registrou faixas de PID com definição incerta
  na fonte consultada. Não dá pra generalizar a garantia do §2.1 pra
  qualquer PID novo sem checar.
- **Por isso a resposta prática é sim, você precisaria endereçar a ECU
  explicitamente** — não necessariamente porque a leitura em broadcast
  falharia, mas porque (a) a descoberta não funciona sem isso, e (b) é o
  único jeito de ter certeza de qual módulo respondeu, em vez de confiar
  numa convenção de nomenclatura que o padrão não garante. Isso é
  exatamente o que falta implementar no §3.2/§3.3: `reqId=0x7E1`
  (physical request pra TCM) em vez de `0x7DF`, e a superfície
  `AT SH 7E1` no `Elm327` — nenhum dos dois existe hoje, então, com o
  firmware atual, **nenhum desses 5 PIDs é lido de forma confiável em um
  carro real**, mesmo que o app já soubesse decodificá-los.

## Fontes consultadas

- [OBD2 Explained - A Simple Intro — CSS Electronics](https://www.csselectronics.com/pages/obd2-explained-simple-intro) — endereçamento funcional/físico, múltiplas respostas esperadas a requisição funcional, offset `+0x08`.
- [ISO 15765-4 CAN 11-Bit Identifier Guide — GarageGuide.blog](https://garageguide.blog/how-iso-15765-4-uses-can-11-bit-identifier) — faixas de ID e norma aplicável.
- [ELM327 AT Command Set (GitHub wiki)](https://github.com/deshi-basara/libreXC/wiki/ELM327-AT-Command-Set) — sintaxe de `AT SH`/`AT SR`.
- Busca sobre SAE J1979/ISO 15031 (módulos relevantes à emissão como escopo do Modo 01-0A) — confirma que o cenário ECM(+TCM) já modelado no simulador é o caso comum, não um caso extremo.
- [OBD-II PIDs — Wikipedia](https://en.wikipedia.org/wiki/OBD-II_PIDs) — tabela oficial de PIDs do Modo 01 e estrutura de decodificação de DTC (letra de categoria + dígito genérico/fabricante).
- Busca sobre faixas de subsistema dos códigos `P0xxx` (SAE J2012) — confirma a repartição por dígito de subsistema (`P01xx`/`P02xx` combustível-ar, `P03xx` ignição/misfire, `P04xx` emissões, `P05xx` velocidade/marcha lenta, `P06xx` computador/saídas, `P07xx`-`P08xx` transmissão) e o significado do dígito genérico (`0`/`2`) vs. fabricante (`1`/`3`).
- `simulador/ECUSim/PIDMap_Definition.h` e `mobile-app/lib/src/domain/obd2/obd2_pid.dart` — conjunto de PIDs já implementado e testado neste repositório (base da tabela §4.2, não uma lista à parte inventada).
