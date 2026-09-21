# Handoff: leitura real de DTC no app (mobile-app)

> Escrito ao final da sessão que implementou e **validou em hardware real**
> (Arduino/ECUSim + ESP32 rodando `-e twai`) a leitura de DTC no dongle
> (`esp32-firmware`, branch `feature/dongle-dtc`). Este documento é o ponto
> de partida para quem for trocar `FakeDtcRepositoryImpl` pela leitura real.
> Não presuma que quem lê isso tem o contexto da conversa que gerou o
> dongle — está tudo resumido abaixo. Complementa (não substitui)
> `esp32-firmware/HANDOFF_DTC.md`, que documenta o lado do firmware em
> detalhe — aqui só entram as partes relevantes pro app, mais o que mudou
> desde então.

## 1. Contexto: o que o dongle já fala hoje (validado em hardware real)

O dongle (`esp32-firmware`, perfis `-e twai`/`-e mcp2515`) responde aos
Modos **03** (confirmados), **07** (pendentes) e **0A** (permanentes) —
comando ELM327 de 2 hex chars, sem PID (`"03"`, `"07"`, `"0A"`), sobre o
mesmo transporte BLE (Nordic UART) + texto ELM327 já documentado em
`CLAUDE.md`. Testado ponta-a-ponta contra o simulador
(`test-scripts/hil_dongle_dtc.py`), inclusive o caso de 3+ DTCs que força
fragmentação ISO-TP (First Frame + Consecutive Frame) — não é mais uma
implementação só analisada estaticamente, foi exercitada em barramento CAN
real.

**Atualização — Modo 02 (freeze frame) também implementado.** O bloqueio
estrutural descrito originalmente na seção 4 (interface `readPid` só
mandava 2 dos 3 bytes exigidos) foi resolvido: o dongle agora expõe
`IObd2::readFreezeFramePid(pid, buf, maxLen)`, e o comando ELM327
`"02"+PID` (ex.: `"0202"`, `"020C"`) já responde de verdade. A seção 4
deixa de ser "especificação bloqueada" e passa a ser o desenho pronto pra
implementar no app.

**Formato da resposta** (confirmado por hardware, não é só o SAE J1979 no
papel): `<SID+0x40> <count> <DTC_hi> <DTC_lo> ...` pro Modo 03/07/0A, e
`<SID+0x40> <PID> <frame#> <dados...>` pro Modo 02 — sempre hex cru, sem
separador "Pxxxx" nenhum, conversão é responsabilidade do app (ver seção
3). Exemplos reais capturados via BLE:

```
"03"    -> "43 00"                      (0 DTCs confirmados)
"03"    -> "43 01 03 01"                (1 DTC: 0x0301 = P0301)
"03"    -> "43 03 03 01 01 71 01 33"    (3 DTCs: P0301, P0171, P0133)
"07"    -> "47 00"
"0A"    -> "4A 00"
"0202"  -> "42 02 00 03 01"             (freeze frame armazenado, origem = P0301)
"0204"  -> "42 04 00 66"                (freeze frame — carga do motor)
"0204"  -> "NO DATA"                    (sem freeze frame armazenado)
```

O `<frame#>` do Modo 02 é sempre `00` — é o único frame que existe (nem o
simulador nem o dongle guardam histórico de frames antigos), então o app
não precisa (e não deve) tratar isso como algo consultável/variável.

**Atenção ao eco do comando**: o `Elm327` do dongle tem `_echo = true` por
padrão (não é `ATE0` por default) — a notificação BLE de resposta vem com
o comando ecoado embutido, ex.: pedir `"03"` devolve literalmente
`"03 43 00 \r>"` no fio (o `"03"` inicial é o eco, não faz parte da
resposta). Isso é **exatamente** o mesmo problema que
`Obd2Datasource._readServiceBytes` já resolve pra PID (`compact.indexOf(header)`
em vez de assumir posição 0) — reaproveitar essa técnica é obrigatório
para o datasource de DTC, não opcional.

**Duas limitações reais e conhecidas, não é bug a corrigir no app**:

- **Sem endereçamento por ECU.** O barramento tem 2 ECUs simuladas (ECM
  `0x7E0`/`0x7E8`, TCM `0x7E1`/`0x7E9`) respondendo ao mesmo broadcast
  funcional `0x7DF`. O dongle aceita a **primeira** resposta que casar
  (mesma simplificação que já existe pra leitura de PID), então se ECM e
  TCM tiverem DTCs confirmados ao mesmo tempo, só os da que respondeu
  primeiro aparecem — a leitura nunca é a união das duas. Isso já foi
  testado (cenário `multi_ecu` do `hil_dongle_dtc.py`) e é o comportamento
  esperado hoje, não uma falha intermitente para investigar.
- **Qualquer erro vira `"NO DATA"`, nunca um NRC explícito.** Timeout,
  resposta negativa (`7F <SID> <NRC>`) e mismatch de SID (transação
  antiga não drenada) são todos colapsados em `-1` no `Obd2Can::readDtc()`
  e formatados como `"NO DATA"` pelo `Elm327`. O datasource do app **nunca**
  vai ver um `"7F 03 11"` chegar — só precisa tratar `"43/47/4A ..."` (sucesso,
  inclusive `count=0`) ou `"NO DATA"`/timeout (falha).

## 2. Estado atual do app: o que já existe e o que mudou desde o handoff do firmware

O handoff do firmware (`esp32-firmware/HANDOFF_DTC.md`, seção 3) já mapeou
que **toda a UI e o domínio de DTC já existem e estão prontos**
(`DtcCode`, `DtcActiveEntry`, `DtcSnapshot`, `DtcStatus`, tela de
diagnóstico) — só falta o datasource/repository real, espelhando
`obd2_repository_impl.dart`/`elm327_client.dart`/`obd2_datasource.dart`
(Fase 2, telemetria). Isso continua verdade. O que esta sessão confirmou a
mais, checando o código de fato (não é mais suposição):

- **A decodificação "Pxxxx" NÃO existe em lugar nenhum do app hoje.**
  `dtc_catalog.dart` é só uma lista estática de `DtcDefinition` — não tem
  nenhuma função `int -> String` para converter o valor cru de 16 bits do
  protocolo (ex.: `0x0301`) no formato "P0301". Vai ser preciso implementar
  isso do zero (regra SAE J2012: os 2 bits mais altos do byte alto
  escolhem a letra — `00`=P poder­train, `01`=C chassis, `10`=B body,
  `11`=U network —, os 2 bits seguintes escolhem o primeiro dígito
  (0–3), e os 3 nibbles restantes são hex literal — ex.: `0x0301` → bits
  `00 00 0011 00000001` → `P` + `0` + `301` = "P0301"). Sugestão de lugar:
  uma função pura em `dtc_catalog.dart` ou um novo `dtc_code_codec.dart`
  no domínio (mesmo espírito de `Obd2Pid.decode` — regra pura, testável,
  sem tocar em BLE).
- **Códigos ativos fora do catálogo curado ficam invisíveis.** `DtcViewModel._load()`
  (`dtc_view_model.dart`) monta a lista exibida iterando **`dtcCatalog`**
  (a lista curada de ~26 códigos) e decorando com o que estiver em
  `snapshot.active` — nunca itera `snapshot.active` diretamente. Ou seja:
  um DTC real do veículo que não estiver na curadoria de `dtc_catalog.dart`
  **nunca aparece na tela**, mesmo estando confirmado/pendente. Isso é uma
  limitação pré-existente do design da vista "catálogo decorado", não algo
  que esta sessão mudou — mas quem for plugar a leitura real deve estar
  ciente: testar contra o simulador com um DTC fora da curadoria (o
  simulador tem `P0133`/`P0700`, por exemplo — `P0700` já está no catálogo,
  mas `P0133` não) vai parecer "leitura incompleta" quando na verdade é
  esse comportamento existente. Vale decidir (fora do escopo deste
  handoff) se isso é aceitável para o TCC ou se `DtcViewModel` deveria
  passar a iterar os ativos e cair para uma `DtcDefinition` "genérica"
  quando o código não estiver curado.
- **`DtcStatus` ainda só tem `confirmed`/`pending`** (`dtc_status.dart`) —
  precisa de um terceiro valor pro Modo 0A (permanente) antes da leitura
  real chegar lá. Widgets que fazem `switch` exaustivo sobre `DtcStatus`
  (ex.: `dtcStatusBadge` em algum lugar de `dtc_severity_color.dart`/`dtc_row.dart`
  — checar) vão quebrar a compilação até serem atualizados, o que é bom:
  o analyzer força a decisão de como badge/cor de "permanente" deve ficar.

## 3. Plano sugerido, camada por camada

Mesmo padrão da feature `telemetry` (`mobile-app/CLAUDE.md` §Arquitetura):
`data/datasources` (bytes) → `data/repositories` (domínio + erro) →
`ui/view_model` (inalterado) → `ui/view` (inalterado).

### `data/datasources/dtc_datasource.dart` (novo)

Espelha `Obd2Datasource`: recebe um `Elm327Client` já ligado à conexão, e é
o único lugar que conhece o formato de fio do Modo 03/07/0A/02. Devolve
sempre dado cru (nunca "Pxxxx", nunca string formatada pra exibição) —
essa é a régua para decidir o que é datasource vs repository daqui pra
frente.

```dart
class DtcDatasource {
  DtcDatasource(this._elm);
  final Elm327Client _elm;

  /// Lê a lista de DTCs de um serviço sem PID (0x03/0x07/0x0A). Devolve os
  /// códigos crus de 16 bits (ex.: 0x0301 para P0301) — lista VAZIA (não
  /// null) quando a leitura funcionou e não há nenhum DTC (`"43 00"`), e
  /// `null` em NO DATA/timeout/resposta malformada (falha de leitura).
  /// Essa distinção importa: 0 DTCs é sucesso, sem resposta é erro.
  Future<List<int>?> readDtcListRaw(int service) async { ... }
}
```

Parsing: mesma técnica de `Obd2Datasource._readServiceBytes` — uppercase,
checar `"NO DATA"`, `compact = raw.replaceAll(RegExp('[^0-9A-F]'), '')`,
localizar o header `hex(service + 0x40)` com `compact.indexOf(header)`
(**não** assumir posição 0 — ver eco na seção 1), ler os 2 hex chars
seguintes como `count`, depois `count` pares de 2 bytes (4 hex chars cada)
como os DTCs.

### `data/repositories/dtc_repository_impl.dart` (troca do `FakeDtcRepositoryImpl`)

Espelha `Obd2RepositoryImpl`: obtém a `BleConnection` via
`DongleRepository`, monta `Elm327Client` + `DtcDatasource` sob demanda
(mesmo padrão `_ensureDatasource()`/teardown proativo em
`connectionPhase`), mapeia erro cru → `DtcReadFailure`.

```dart
class DtcRepositoryImpl implements DtcRepository {
  DtcRepositoryImpl(this._dongle) { /* mesma escuta de fase que Obd2RepositoryImpl */ }
  final DongleRepository _dongle;
  // ... _ensureDatasource/_teardown iguais ao padrão de Obd2RepositoryImpl ...

  @override
  Future<DtcSnapshot> read() async {
    final ds = _ensureDatasource();
    if (ds == null) throw const DtcReadFailure('Sem conexão BLE pronta');

    final confirmedRaw = await ds.readDtcListRaw(0x03) ?? [];
    final pendingRaw   = await ds.readDtcListRaw(0x07) ?? [];
    final permanentRaw = await ds.readDtcListRaw(0x0A) ?? [];
    // (permanentRaw só é útil quando DtcStatus ganhar um 3º valor — ver §2)

    final active = [
      for (final raw in confirmedRaw)
        DtcActiveEntry(code: dtcCodeFromRaw(raw), status: DtcStatus.confirmed, detectedLabel: ''),
      for (final raw in pendingRaw)
        DtcActiveEntry(code: dtcCodeFromRaw(raw), status: DtcStatus.pending, detectedLabel: ''),
    ];

    // Freeze frame: só o DTC confirmado que de fato tem congelamento (ver §4).
    final withFreezeFrame = await _attachFreezeFrame(ds, active);

    return DtcSnapshot(
      active: withFreezeFrame,
      milOn: confirmedRaw.isNotEmpty, // ou ler PID 0x01 (monitor status) separado — ver nota abaixo
    );
  }
}
```

Nota sobre `milOn` (**implementado**): a inferência via
`confirmedRaw.isNotEmpty` mostrada acima era só o esboço mais simples do
handoff original. A versão real usa o PID 0x01 (Modo 01, status de
monitoramento) lido separadamente, como o handoff do firmware sempre
sugeriu — `Obd2RepositoryImpl._readMilFromEcus` consulta
`Obd2Datasource.readMonitorStatusResponses()` (`010101`... na prática só
`0101`) e considera o MIL aceso se **qualquer uma** das ECUs de motor (ECM,
`0x7E8`) ou câmbio (TCM, `0x7E9`) — ver `domain/obd2/ecu_role.dart` —
reportar o bit 7 do byte A ligado (`domain/obd2/monitor_status.dart`). É
uma leitura best-effort, independente da varredura de DTCs (Modos
03/07/0A): falha ou ausência de resposta de uma ECU não derruba o
diagnóstico, só deixa aquela ECU de fora da checagem do MIL.

### DI (`ui/diagnostics/diagnostics_providers.dart`)

Troca de uma linha, igual ao padrão de `obd2_repository_impl.dart`:

```dart
final Provider<DtcRepository> dtcRepositoryProvider = Provider<DtcRepository>((ref) {
  final repo = DtcRepositoryImpl(ref.read(dongleRepositoryProvider));
  ref.onDispose(repo.dispose);
  return repo;
});
```

## 4. Freeze frame (Modo 02) — pode ser implementado agora

O bloqueio estrutural que existia no dongle (`readPid` só mandava 2 dos 3
bytes que o Modo 02 real exige) **já foi resolvido** — ver seção 1. O
comando `"02"+PID` já responde de verdade contra o simulador. Pode
implementar contra hardware real, seguindo exatamente o desenho abaixo
(é o mesmo que já estava especificado aqui, nenhuma mudança de design —
só deixou de estar bloqueado).

**O ajuste que importa manter, resumido**: a leitura de freeze frame vive
num método **separado** do datasource (não misturado com
`readDtcListRaw`), devolvendo PID + valor cru no mesmo formato de uma
leitura de sensor normal (`Obd2Reading`) — nunca label/unidade já
formatados. É o **repository** quem agrega esses PIDs no DTC certo,
abstraindo o formato de fio de quem consome (`view_model`/`view`, que não
mudam nada). **UI e domínio permanecem exatamente como estão hoje**
(`DtcCode.freezeFrame`, `DtcActiveEntry.freezeFrame`, `dtc_detail_sheet.dart`)
— só o repository muda. E, como o protocolo real só guarda **um** freeze
frame por ECU, só **um único DTC** (o que efetivamente originou aquele
freeze frame, descoberto via PID `0x02`) vai sair com `freezeFrame`
preenchido — todo o resto dos confirmados fica com `freezeFrame: []`,
mesmo que o mock de hoje mostre vários preenchidos ao mesmo tempo (ver
`esp32-firmware/HANDOFF_DTC.md` item 3 pro porquê disso ser a resposta
honesta ao hardware, não uma limitação a contornar).

O desenho detalhado, camada por camada:

### No datasource: método(s) dedicado(s), formato igual ao de leitura de sensor

**Não** misture a leitura de freeze frame com `readDtcListRaw`. São dois
métodos novos e específicos, devolvendo dado cru no **mesmo formato de uma
leitura de PID normal** (`List<int>?` de data bytes, igual
`Obd2Datasource.readPidRaw`) — nenhuma formatação de label/unidade aqui:

```dart
class DtcDatasource {
  // ... readDtcListRaw acima ...

  /// Descobre qual DTC (se algum) tem freeze frame de verdade, perguntando
  /// [0x02, 0x02, 0x00] — PID 0x02 é tratado à parte pelo protocolo (devolve
  /// o DTC de origem, 2 bytes, não um valor de PID comum). Devolve `null`
  /// quando não há freeze frame válido armazenado (NRC 0x31 -> "NO DATA").
  Future<int?> readFreezeFrameOriginDtc() async { ... }

  /// Lê um PID do freeze frame (Modo 02, frame 0) — MESMO formato de
  /// `readPidRaw`: devolve os data bytes crus do [pid], já congelados no
  /// instante da falha, sem nenhuma conversão/formatação. `null` se o PID
  /// não estiver na lista fixa congelada pelo veículo, ou NO DATA.
  Future<List<int>?> readFreezeFramePidRaw(Obd2Pid pid) async { ... }
}
```

A lista fixa de PIDs congelados pelo simulador/hardware (documentada no
handoff do firmware) é `{0x04, 0x05, 0x0C, 0x0D, 0x0F, 0x11}` —
**coincide exatamente** com PIDs que já existem em `Obd2Pid`
(`engineLoad`, `coolantTemp`, `rpm`, `speed`, `intakeAirTemp`, `throttle`).
Não existe descoberta via PID `0x00` no Modo 02 (ver handoff do firmware,
item 1) — a lista é hardcoded no lado de quem lê, igual está hardcoded no
simulador.

### No repository: reaproveita `Obd2Reading`, formata, e associa ao DTC certo

O passo de "virar `Obd2Reading`" é *idêntico* ao de uma leitura normal —
não crie um tipo novo para isso:

```dart
Future<List<DtcActiveEntry>> _attachFreezeFrame(
  DtcDatasource ds,
  List<DtcActiveEntry> active,
) async {
  final originRaw = await ds.readFreezeFrameOriginDtc();
  if (originRaw == null) return active; // nenhum freeze frame armazenado

  final originCode = dtcCodeFromRaw(originRaw);
  final freezeFrame = <DtcFreezeFrameEntry>[];
  for (final pid in const [
    Obd2Pid.engineLoad, Obd2Pid.coolantTemp, Obd2Pid.rpm,
    Obd2Pid.speed, Obd2Pid.intakeAirTemp, Obd2Pid.throttle,
  ]) {
    final data = await ds.readFreezeFramePidRaw(pid);
    if (data == null) continue;
    final reading = Obd2Reading(pid: pid, value: pid.decode(data)); // igual à leitura ao vivo
    freezeFrame.add(_formatFreezeFrameEntry(reading)); // formatação SÓ aqui
  }

  // Só o DTC de origem recebe o freeze frame; os demais confirmados ficam
  // com freezeFrame: [] — é a resposta honesta ao protocolo real (só existe
  // 1 freeze frame por ECU), mesmo que o mock hoje mostre mais de um
  // preenchido (ver esp32-firmware/HANDOFF_DTC.md item 3 para o porquê).
  return [
    for (final entry in active)
      entry.code == originCode ? entry.copyWith(freezeFrame: freezeFrame) : entry,
  ];
}

DtcFreezeFrameEntry _formatFreezeFrameEntry(Obd2Reading reading) => DtcFreezeFrameEntry(
  label: reading.pid.shortLabel, // ou .label — decidir na hora, olhando o layout do StatCard no sheet
  value: '${_formatNumber(reading.value)} ${reading.pid.unit}'.trim(),
);
```

(`DtcActiveEntry` vai precisar de um `copyWith` — hoje não tem, só
`==`/`hashCode`/`toString`; adicionar segue o mesmo padrão de
`DtcState.copyWith`.) `_formatNumber` é responsabilidade nova do
repository — não existe hoje um formatador compartilhado de "número +
separador de milhar pt-BR" no app (`pubspec.yaml` não tem `intl`); ou
adiciona `intl` ou escreve algo pequeno o suficiente para não precisar.

**O ponto central, que é a resposta direta ao pedido desta sessão**: o
*datasource* nunca sabe o que é "Rotação" ou "2.480 rpm" — ele só sabe
"PID 0x0C, bytes `[0x26, 0x18]`", exatamente como para uma leitura normal
de painel. Toda a tradução pra rótulo/unidade/DTC-dono vive no
*repository*, que é quem conhece tanto o domínio de PID quanto o de DTC —
abstraindo os dois lados um do outro, igual ao resto do projeto já faz
entre camadas.

## 5. `detectedLabel`: remove da leitura real, nunca aparece no datasource

`detectedLabel` (`DtcActiveEntry`/`DtcCode`) é um rótulo de exibição tipo
"há 2 dias · 3 ciclos" — o próprio domínio já documenta que **não é um
timestamp real** ("o firmware não guarda histórico entre sessões"). Não
existe protocolo OBD-II nenhum (Modo 03/07/0A/02) que devolva "há quanto
tempo"/"quantos ciclos" — é dado curado do mock, sem contraparte real.

Para a implementação real:

- **O `DtcDatasource` não expõe nada relacionado a isso** — nenhum método,
  nenhum parsing, nenhuma tentativa de inferir tempo/ciclos a partir de
  qualquer resposta. Não é um "TODO: implementar depois" — é
  estruturalmente impossível com os Modos 03/07/0A/02, então nem entra na
  superfície da classe.
- **O `DtcRepositoryImpl` sempre passa `detectedLabel: ''`** (string
  vazia, não tenta formatar nada) ao construir `DtcActiveEntry` — ver
  exemplo de código na seção 3.
- **Ajuste necessário na UI, senão aparece uma linha vazia**:
  `dtc_detail_sheet.dart` hoje checa `if (dtc.detectedLabel != null)` para
  decidir se renderiza `"detectado ${dtc.detectedLabel}"`. Com string
  vazia (não `null`), essa condição continua `true` e renderiza
  `"detectado "` com nada depois — um bug visual. Trocar a condição para
  `if (dtc.detectedLabel != null && dtc.detectedLabel!.isNotEmpty)` (ou
  equivalente) faz parte deste trabalho, não é opcional.
- **Não precisa mexer no `FakeDtcRepositoryImpl`** — o mock continua
  populando `detectedLabel` com texto de curadoria pra demonstrar a UI
  "bonita"; é só a implementação real que fica sem essa informação.
  `DtcCode`/`DtcActiveEntry` mantêm o campo (evita mexer em domain/UI além
  do necessário) — só a fonte real nunca o preenche de verdade.

## 6. Como testar

- **Sem hardware**: `FakeDtcRepositoryImpl` continua sendo o dev default
  enquanto a implementação real não estiver plugada no
  `dtcRepositoryProvider` — nada muda aqui até a troca final.
- **Com hardware real**: grave `esp32-firmware` no perfil `-e twai` (sem
  PSK — mais simples pra testar; ver `esp32-firmware/README.md` para os
  perfis `_psk`/`_handshake` quando quiser testar com criptografia depois),
  conecte ao `simulador/ECUSim` via CAN físico, e use
  `test-scripts/hil_dongle_dtc.py --com <porta> --scenario <nome>` para
  montar os cenários (`limpo`, `falha_unica`, `multiframe`, `multi_ecu`)
  **antes** de abrir o app e ler pela tela de Diagnóstico — o script isola
  o setup do simulador da leitura em si, útil pra montar cenários
  determinísticos sem precisar de um carro de verdade. Por padrão o script
  já consulta o freeze frame também (PID de origem + os 6 PIDs congelados)
  depois da lista de DTCs — útil pra conferir o formato de fio do Modo 02
  antes de implementar o datasource. Ver `test-scripts/README.md` para os
  pré-requisitos.
