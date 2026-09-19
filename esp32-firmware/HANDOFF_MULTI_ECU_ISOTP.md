# Handoff: múltiplas ECUs e unificação de leitura em `IsoTpClient`

> Escrito durante a sessão de implementação de DTC (Modo 03/07/0A/02) no
> dongle (`esp32-firmware/HANDOFF_DTC.md`), a partir de descobertas feitas
> testando contra hardware real (Arduino/ECUSim + ESP32 `-e twai`). Cobre
> dois assuntos relacionados mas distintos: (1) o que se sabe hoje sobre o
> comportamento com múltiplas ECUs respondendo, e (2) uma recomendação de
> unificar `Obd2Can::readPid()` (Modo 01/09) sobre `IsoTpClient` — **não
> implementada ainda**, fica como próximo passo.

## 1. Múltiplas ECUs respondendo ao mesmo broadcast

### O que já era assim antes desta sessão (não é regressão)

`simulador/ECUSim/CANMessageHandle.ino::handleCANMessage()` despacha
**qualquer** requisição que chegue em `0x7DF` (broadcast funcional) pra
**todas** as ECUs configuradas (`ECU_CONFIGS`, hoje ECM `0x7E0`/`0x7E8` e
TCM `0x7E1`/`0x7E9`) — não é algo específico de DTC, vale pra Modo 01
também. O dongle (`Obd2Can`, tanto o `readPid()` antigo quanto o
`IsoTp::request()` novo) sempre manda pro broadcast — nunca usa
endereçamento físico — e sempre aceita a **primeira** resposta que bater
os critérios, seja qual ECU for. Isso já era assim desde antes desta
sessão para o Modo 01; a sessão de DTC só herdou o mesmo padrão pros
modos novos.

### Achado: para PIDs de valor, "qual ECU respondeu" não importa hoje

Fui conferir o simulador de verdade em vez de supor: `PID_Value_Map`
(`simulador/ECUSim/ECUSim.ino`) é um **único array global**, não faz
parte de `EcuState_t`. `fillValueBytes()` (`PIDMessageBuilder.ino`) lê
direto desse array global pra qualquer PID de valor (0x04, 0x05, 0x0C,
0x0D, 0x0F, 0x11, etc.) — o parâmetro `ecuState` que `dispatchOBDRequest`
recebe e repassa **nunca é usado** para esses PIDs. Ou seja: ECM e TCM
sempre devolvem o **mesmo** valor pra RPM, temperatura, velocidade etc.,
por construção do simulador — não há ambiguidade de "qual ECU está
certa" pra esses PIDs, mesmo que ambas respondam.

A exceção real é **por-ECU de verdade**: PID 0x01 (monitor status, via
`computeMonitorStatusPID`, que lê `ecuState.mil`/`ecuState.confirmedCount`)
e tudo que envolve `EcuState_t` — as listas de DTC (Modo 03/07/0A) e o
freeze frame (Modo 02), cada ECU com o seu. O painel de telemetria do app
(`mobile-app/lib/src/domain/obd2/obd2_pid.dart`, enum `Obd2Pid`) exclui
PID 0x01 do que lê, então hoje ele nunca toca nessa parte genuinamente
por-ECU.

### Bug real encontrado e corrigido: filtro por SID não basta quando duas requisições esperam o mesmo SID

Ao testar o freeze frame em hardware, toda consulta depois da primeira
(`"0202"` funcionou, `"0204"` em diante vinham `"NO DATA"`) revelou um bug
no `IsoTpClient` (`src/obd2/real/IsoTpClient.cpp`): o filtro que eu tinha
posto (`expectedReplySid = req[0] | 0x40`) usa só o SID da resposta pra
decidir se aquele frame é "a resposta desta transação". Isso funciona
quando transações vizinhas pedem serviços diferentes (Modo 03 → 07 → 0A,
cada um com SID de resposta diferente: 0x43/0x47/0x4A), mas **não**
funciona quando duas requisições seguidas esperam o **mesmo** SID — que é
exatamente o caso do freeze frame, onde toda consulta de PID usa SID
0x02/0x42 independente do PID. Sequência do bug: ECM responde positivo a
`"0202"` (aceito), TCM responde negativo (`7F 02 31`, ela nunca tem
freeze frame na TCM) um instante depois — mas essa resposta da TCM já não
está mais sendo esperada, fica na fila. A próxima consulta (`"0204"`) lê
essa sobra (SID bate — é `0x7F`, tratado incondicionalmente como negative
response da transação atual) e devolve `-1` antes mesmo de mandar
qualquer coisa nova pro barramento — e o ciclo se repete pra cada consulta
seguinte, numa cascata de "um passo atrasado".

**Fix aplicado**: `IsoTp::request()` agora drena (`while
(can->receive(stale)) {}`) qualquer frame já pendente na fila **antes**
de mandar uma nova requisição — não só quando o SID muda. Como o uso
deste cliente é sempre síncrono (uma transação de cada vez, nunca duas em
voo), qualquer coisa na fila neste ponto é garantidamente sobra de uma
transação anterior. Isso resolve tanto o caso "SID diferente" (que o
filtro já cobria) quanto o caso "mesmo SID" (que só o dreno resolve).
Avaliação de efeitos colaterais (nenhum encontrado, dado o uso atual —
acesso sempre síncrono, sem nenhuma feature de escuta passiva de
barramento no firmware hoje) está registrada nos commits/histórico desta
sessão; vale reavaliar se um dia existir leitura passiva de CAN
independente de request/response.

### O que continua sem solução (limitação conhecida, não corrigida)

**"Primeiro que responde vence" nunca vira "união das duas ECUs".** Isso
é uma limitação de design, não um bug: o dongle não tem endereçamento por
ECU. Testado explicitamente (cenário `multi_ecu` do
`test-scripts/hil_dongle_dtc.py`): DTC confirmado na ECM e outro na TCM
ao mesmo tempo → só o da ECM aparece (ela responde primeiro, de forma
consistente, porque o loop do simulador processa ECM antes de TCM
sincronamente na mesma chamada). Isso vale igualmente pra Modo 01, 02,
03, 07 e 0A — em qualquer um deles, se as duas ECUs tivessem respostas
*genuinamente diferentes* pro mesmo pedido, só uma seria vista.

**Isso faz parte do protocolo OBD-II de verdade, não é invenção deste
TCC**: ISO 15765-4 define tanto endereçamento funcional (broadcast,
`0x7DF`) quanto físico (`0x7E0`–`0x7E7` de requisição, resposta em
`+0x08` → `0x7E8`–`0x7EF`) — os próprios IDs já usados no simulador
(`ECM_CAN_ID = 0x7E0`/`ECM_CAN_RESPONSE_ID = 0x7E8` etc., em
`simulador/ECUSim/ECUSim.h`) são literalmente esse range padrão. O que
falta é o dongle **usar** a parte física do protocolo (mandar direto pra
`0x7E0` quando quiser só a ECM) em vez de sempre fazer broadcast — isso
resolveria a ambiguidade de vez, mas é uma mudança de escopo maior
(precisaria de um jeito de escolher "qual ECU" na interface `IObd2`, que
hoje não existe) e fica fora do escopo desta sessão.

**`Obd2Can::readPid()` (Modo 01/09) não tem o fix de dreno.** Ele tem seu
próprio loop, separado de `IsoTpClient` (ver seção 2) — carrega a mesma
classe de bug (uma resposta atrasada de uma ECU pode ser lida como
resposta a uma leitura de PID diferente logo em seguida), só que hoje
isso fica mascarado porque, pro conjunto de PIDs que o app lê, as duas
ECUs sempre respondem o mesmo valor (ver achado acima) — então mesmo que
o dongle leia a resposta "errada" (da ECU errada), o dado está certo por
coincidência do simulador, não porque o código está correto. Isso pararia
de ser inofensivo se: (a) o app passasse a ler PID 0x01 (monitor status,
genuinamente por-ECU), ou (b) o simulador evoluísse pra ECUs com valores
de fato distintos, ou (c) isto rodasse contra um veículo real (onde
módulos diferentes têm sensores fisicamente diferentes).

## 2. Migração de `readPid()` pra `IsoTp::request()` — recomendada, não feita

### Por que hoje existem dois caminhos

`Obd2Can::readPid()` é código anterior a esta sessão (arquivo já tinha o
comentário `"TODO: Rever implementação. Este arquivo foi gerado com IA"`).
Ele é Single-Frame-only por design. Quando implementei Modo 03/07/0A,
precisei de um cliente ISO-TP de verdade (multiframe) — critério
explícito: não dava pra fazer isso dentro do loop do `readPid()` sem
reescrevê-lo, e Modo 01 nunca gera resposta >7 bytes na prática, então a
decisão deliberada foi **não tocar** em `readPid()` e criar
`IsoTpClient` como caminho novo e paralelo, só para os modos novos
(depois reaproveitado pelo Modo 02 também).

### Por que isso não é ideal

`IsoTp::request()` é hoje estritamente mais correto que o loop de
`readPid()`: tem o fix de dreno (seção 1), trata negative response de
forma genérica, e cobre multiframe. Ter dois códigos fazendo a mesma
coisa básica ("manda Single Frame, espera resposta que bata dentro de um
timeout") é duplicação — e o efeito colateral já apareceu na prática: o
fix de dreno só foi parar num dos dois lugares, então o Modo 01 carrega
uma classe de bug que o Modo 02/03/07/0A não carrega mais.

### Por que não é necessário manter separado

`IsoTp::request()` é um superconjunto do que `readPid()` faz — cobre
Single Frame, cobre negative response, cobre o dreno. A única coisa que
falta pro caso de `readPid()` é validar o **PID** da resposta (não só o
SID) — mas isso já é feito na camada de `Obd2Can`, não dentro do
`IsoTpClient` (mesmo padrão que `readDtc()`/`readFreezeFramePid()` já
usam: checar o byte relevante em `out[]` depois de chamar `request()`, e
devolver `-1` se não bater). Não requer mudança nenhuma no
`IsoTpClient` para isso.

### O obstáculo real encontrado: timeout fixo

`IsoTp::request()` usa uma constante fixa (`N_BS_MS = 1000ms`) pra
esperar a primeira resposta — não há parâmetro de timeout na assinatura
hoje. `Obd2Can::readPid()`, por outro lado, usa `_timeoutMs`
(configurável no construtor, default **200ms**). Isso importa porque a
telemetria (`Obd2Datasource.readSupportedPids()`/`readAll()`/`readMany()`)
lê dezenas de PIDs por ciclo e frequentemente recebe "NO DATA" de
propósito (PID não suportado pelo veículo, ou ainda não descoberto) — se
`readPid()` passasse a usar `IsoTp::request()` do jeito que está hoje,
cada "NO DATA" levaria até 1000ms pra desistir em vez de 200ms (5×), uma
regressão real e perceptível de responsividade no painel ao vivo — não
uma preocupação teórica.

### Plano recomendado (nesta ordem)

1. Adicionar um parâmetro de timeout em `IsoTp::request()` (ex.:
   `uint32_t timeoutMs = N_BS_MS`, preservando o comportamento atual do
   Modo 02/03/07/0A por default).
2. Reescrever `Obd2Can::readPid()` pra montar a requisição de 2 bytes
   (`service`+`pid`) e chamar `IsoTp::request(..., _timeoutMs)`,
   validando `out[0]==service|0x40 && out[1]==pid` antes de extrair os
   dados — apagando o loop manual antigo.
3. **Revalidar contra hardware real** (não só compilar) — especificamente
   o painel de telemetria (`mobile-app`, tela `/painel`), já que é o
   caminho mais usado do dongle e esta sessão já demonstrou duas vezes
   que bugs deste tipo só aparecem em teste com hardware de verdade, não
   em revisão de código nem nos testes nativos do `IsoTpClient`.

Nada disso foi implementado ainda — fica como próximo passo, junto com o
endereçamento físico por ECU (seção 1) como a correção "de verdade" da
ambiguidade multi-ECU, caso vire prioridade pro TCC.
