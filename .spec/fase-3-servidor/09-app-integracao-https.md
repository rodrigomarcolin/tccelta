# 09 — Integração HTTPS no app Flutter

> Fase 3 · App. Depende de: 03/04/05/08 (contratos HTTP), **Fase 1** (port do gatilho de handshake, codec TLV/SYNC, buffer e cursor no app). É dependência de: validação ponta a ponta (README).

## Objetivo

Implementar no `mobile-app` a ponte HTTPS entre o SYNC (BLE, Fase 1) e a API:

- pedir desafio, enviar resposta e obter JWT;
- enviar lotes de blocos e receber o ACK;
- obter a última confirmação;
- converter base64 ⇄ bytes nos dois sentidos sem alterar um byte;
- acionar os gatilhos de handshake;
- aplicar a política de reenvio da Tabela 5 (backoff 1 s–5 min, 429 com `Retry-After`, K pela metade, 5xx mantendo o lote no buffer);
- guardar o token com segurança.

A arquitetura obrigatória do app é `datasource → repository → view_model`, com `application/` só quando há orquestração (CLAUDE.md §5.1 e memória do projeto).

## Escopo

Dentro: configuração de ambiente (base URL), cliente `dio`, DTOs, datasource, repository que implementa o port deixado pela Fase 1, use cases de handshake e envio, política de reenvio, falhas, configuração de rede Android/iOS para o certificado de desenvolvimento, testes com `dio` dublado.

Fora (Fase 1): serviço BLE SYNC, TLV/fragmentação, buffer e cursor, erros App ⇄ dongle, tela de parâmetros `T_lat`/`B_max`/`T_GPS`. Esta spec **consome** essas peças pelos ports da Fase 1 e não as redefine.

## Contrato cruzado com a Fase 1 (port do gatilho de handshake)

**[Reconciliado com a Fase 1, spec 09 §2–5 — os nomes e assinaturas abaixo são os da Fase 1 e valem para esta spec.]** A Fase 1 deixa no app o port `SyncServerGateway` (`lib/src/services/sync/sync_server_gateway.dart`) com duas implementações em `infra/sync/` (`UnavailableSyncServerGateway`, stub; `BenchSyncServerGateway`, servidor de bancada em *debug*), o datasource `SyncServerDatasource(SyncServerGateway)`, o repository `SyncServerRepository`/`SyncServerRepositoryImpl` (dono do **token**, com `needsHandshake`/`clearToken`), o `HandshakeRunner` (perna BLE do handshake) e o `SyncCoordinator` (buffer, regra de envio, gatilhos com *backoff*). Esta fase entrega **só** o adapter `HttpsSyncServerGateway` (`lib/src/infra/sync/https_sync_server_gateway.dart`, sobre `dio`) e o troca no provider `syncServerGatewayProvider` (caso `'https'`). Onde esta spec diz `SyncServerRepositoryImpl`, `RealizarHandshakeUseCase`, `EnviarLoteUseCase` ou `SyncDongleRepository`, leia a tabela de mapeamento:

| Nome provisório desta spec | Nome real (Fase 1) | Onde vive |
|---|---|---|
| `pedirDesafio(devId)` | `requestChallenge(DevId) → ChallengeEnvelope{challenge: SyncFrame 0x01, expiresAt}` | port `SyncServerGateway` |
| `enviarResposta(devId, tlv)` | `submitResponse(DevId, SyncFrame response) → HandshakeEnvelope{ack: SyncFrame 0x04, token, tokenExpiresAt, suggestedLatency}` | port |
| `enviarLote(devId, tlvs)` | `postBlocks(String token, List<SyncFrame> blocks) → SyncFrame 0x04` | port |
| `obterConfirmacao(devId)` | `latestConfirmation(DevId, String token) → SyncFrame? 0x04` | port |
| `tokenValidoPor` / `descartarToken` / sessão em memória | `SyncServerRepository.needsHandshake(now, margin)` / `clearToken()` / `SyncToken` | repository da Fase 1 (não é desta fase) |
| `SyncServerRepositoryImpl implements SyncServerGateway` (§Repository abaixo) | `HttpsSyncServerGateway implements SyncServerGateway` | `infra/sync/` (**esta fase**) |
| `SyncApiDatasource` | continua existindo como classe interna do adapter (`infra/sync/sync_api_client.dart`), **não** como datasource da camada `data` (o datasource da Fase 1 já envolve o port) | `infra/sync/` |
| `RealizarHandshakeUseCase` / `EnviarLoteUseCase` | `HandshakeRunner.run(devId, generation:)` e `SyncCoordinator` (`ensureHandshake`, regra de envio, `PoliticaReenvio` ⇒ constantes `kRetryMin`/`kRetryMax` de `sync_protocol.dart`) | `application/sync/` (Fase 1); esta fase só acrescenta os gatilhos que faltam (tabela §Gatilhos) |
| `SyncDongleRepository` | `DongleSyncRepository` (`events`, `sendChallenge`, `sendAck`, `requestBatch`, `sendError`) | `domain/repositories/` (Fase 1) |
| `ResultadoHandshake` / `ResultadoLote` / `AckInfo` | `HandshakeOutcome` / `PostResult` (`BlocksAccepted`, `Unauthorized`, `Forbidden`, `DongleBlocked`, `RateLimited`, `ServerUnavailable`, `MalformedRequest`) / `SyncFrame` + leitura dos campos do ACK em `domain/sync` | Fase 1 |
| Exceções do adapter | tabela de exceções do port da Fase 1 (spec 09 §2: `ServerUnauthorized`, `ServerForbiddenOtherDongle`, `ServerDongleBlocked`, `ServerNoConfirmation`, `ServerChallengeRejected`, `ServerResponseRejected`, `ServerBatchNoProgress(ack)`, `ServerRateLimited(retryAfter)`, `ServerUnavailable(retryAfter?)`, `ServerMalformed(code)`), escolhidas pelo `codigo` do `problem+json` | `services/sync/sync_server_exceptions.dart` (Fase 1) |

O port, como a Fase 1 o define:

```dart
// lib/src/services/sync/sync_server_gateway.dart   (criado pela Fase 1; esta fase não muda a assinatura)
abstract interface class SyncServerGateway {
  String get mode;                                                          // 'stub' | 'bench' | 'https'
  Future<ChallengeEnvelope> requestChallenge(DevId devId);                  // POST /v1/dongles/{id}/desafio → 0x01
  Future<HandshakeEnvelope> submitResponse(DevId devId, SyncFrame response); // POST …/desafio/resposta → 0x04 + token
  Future<SyncFrame> postBlocks(String token, List<SyncFrame> blocks);       // POST /v1/dongles/blocos → 0x04
  Future<SyncFrame?> latestConfirmation(DevId devId, String token);         // GET …/confirmacao → 0x04 | null (404)
}
```

Premissas sobre a Fase 1 (confirmadas na spec 09 da Fase 1):

| Item | Valor |
|---|---|
| Unidade trocada com o servidor | `SyncFrame` (= TLV completo `T(1) \| L(u16 LE) \| V`, idêntico ao do SYNC_RX/SYNC_TX sem o byte de controle; `toTlv()`/`parseTlv()`) |
| `DevId` | 8 bytes, conhecido pelo app a partir do Aviso 0x07 (bytes 5–12); value object `DevId` com `hex` de 16 dígitos (a Fase 1 usa maiúsculos; a URL aceita os dois) |
| Base64 | `Base64Bytes.encode/decode` (RFC 4648 §4 com *padding*; `-`/`_` recusados) |
| Buffer, cursor, regra de envio | `domain/sync` da Fase 1 (spec 07 §2); esta fase não os redefine |
| Token | só em memória, em `SyncServerRepository` (mesma decisão nas duas fases) |

## Design

### Ambiente e base URL (`lib/src/core/config/app_environment.dart`)

**[Decisão] "Flavor" por `--dart-define-from-file`**, sem *product flavors* nativos. O app ainda não tem flavors, e a diferença entre ambientes é só a URL e a permissão de HTTP sem TLS. Isso cabe em *defines*, sem duplicar a configuração Gradle/Xcode.

```dart
enum Flavor { dev, prodlike, producao }

@immutable
class AppEnvironment {
  const AppEnvironment({required this.flavor, required this.apiBaseUrl});
  factory AppEnvironment.fromDefines() => AppEnvironment(
        flavor: Flavor.values.byName(const String.fromEnvironment('FLAVOR', defaultValue: 'dev')),
        apiBaseUrl: Uri.parse(const String.fromEnvironment('API_BASE_URL', defaultValue: 'https://10.0.2.2:8443')),
      );
  final Flavor flavor;
  final Uri apiBaseUrl;
  /// HTTP sem TLS só no flavor dev (emulador apontando para a porta 8080 da API).
  bool get permiteHttpSemTls => flavor == Flavor.dev;
}

final appEnvironmentProvider = Provider<AppEnvironment>((ref) => AppEnvironment.fromDefines());
```

- Arquivos `mobile-app/env/dev.json` (`{"FLAVOR":"dev","API_BASE_URL":"https://192.168.0.10:8443"}`) e `env/prodlike.json`. `env/*.local.json` fica no `.gitignore`.
- Execução: `fvm flutter run --dart-define-from-file=env/dev.json`.
- A URL base é validada no boot: esquema `https`, ou `http` só se `permiteHttpSemTls`. Senão, `AppException`.
- [Aberto] sobrescrever a URL na tela de Configurações (só em build debug).

### Cliente HTTP (`lib/src/core/network/`)

```dart
// dio_client.dart
Dio criarDioSync(AppEnvironment env) => Dio(BaseOptions(
      baseUrl: env.apiBaseUrl.toString(),
      connectTimeout: const Duration(seconds: 10),
      sendTimeout: const Duration(seconds: 30),
      receiveTimeout: const Duration(seconds: 30),
      // O status é tratado pelo datasource: 422 do lote traz o ACK no corpo.
      validateStatus: (s) => s != null && s >= 200 && s < 600,
      responseType: ResponseType.bytes,
    ))
  ..interceptors.addAll([if (kDebugMode) LogHttpSemSegredos()]);

// dio_provider.dart
final syncDioProvider = Provider<Dio>((ref) => criarDioSync(ref.watch(appEnvironmentProvider)));
```

- `LogHttpSemSegredos` (`log_interceptor.dart`): loga método, caminho, status, duração e tamanho; **mascara `Authorization`** e nunca imprime o corpo das respostas de handshake (contêm o token).
- **[Decisão] Sem interceptor de *retry* genérico.** As retentativas dependem da semântica (Tabela 5: 409 pede *novo desafio*, 429 reduz K, 401 refaz o handshake). Por isso elas ficam nos use cases, não no transporte.

### Codec base64 ⇄ bytes (`lib/src/core/utils/bytes_base64.dart`)

```dart
/// Base64 padrão (RFC 4648 §4) com padding. Rejeita o alfabeto URL-safe e
/// entradas sem padding, para que os bytes de ida e volta sejam exatamente os
/// mesmos que o servidor espera.
abstract final class BytesBase64 {
  static String codificar(Uint8List bytes) => base64.encode(bytes);
  static Uint8List decodificar(String texto) {
    if (texto.length % 4 != 0 || !_alfabeto.hasMatch(texto)) {
      throw const FormatException('base64 inválido');
    }
    return base64.decode(texto);
  }
  static final _alfabeto = RegExp(r'^[A-Za-z0-9+/]*={0,2}$');
}
```

Uso nos dois sentidos:

| Sentido | Mensagem | De | Para |
|---|---|---|---|
| servidor → dongle | Desafio 0x01 | JSON `desafio` (base64) | `decodificar` → bytes → SYNC_RX |
| servidor → dongle | ACK 0x04 do handshake | JSON `ack` (base64) | `decodificar` → bytes → SYNC_RX |
| dongle → servidor | Resposta 0x02 | bytes do SYNC_TX | `codificar` → JSON `resposta` |
| dongle → servidor | Blocos 0x03 | bytes do SYNC_TX | binário puro (`octet-stream`), sem base64 |
| servidor → dongle | ACK 0x04 do lote / confirmação | binário puro | bytes → SYNC_RX |

O app **nunca** recalcula, corta ou reordena bytes: o que vai para o SYNC_RX é exatamente o que o servidor mandou, e o que vai ao servidor é exatamente o TLV remontado.

### DTOs (`lib/src/data/dtos/`, freezed + json_serializable)

```dart
@freezed
abstract class DesafioDto with _$DesafioDto {
  const factory DesafioDto({
    required String desafio,
    @JsonKey(name: 'expira_em') required DateTime expiraEm,
  }) = _DesafioDto;
  factory DesafioDto.fromJson(Map<String, dynamic> json) => _$DesafioDtoFromJson(json);
}

@freezed
abstract class RespostaDesafioDto with _$RespostaDesafioDto {   // corpo do POST /desafio/resposta
  const factory RespostaDesafioDto({required String resposta}) = _RespostaDesafioDto;
  factory RespostaDesafioDto.fromJson(Map<String, dynamic> json) => _$RespostaDesafioDtoFromJson(json);
}

@freezed
abstract class HandshakeDto with _$HandshakeDto {
  const factory HandshakeDto({
    required String ack,
    required String token,
    @JsonKey(name: 'token_expira_em') required DateTime tokenExpiraEm,
    @JsonKey(name: 'token_validade_s') required int tokenValidadeS,
    @JsonKey(name: 't_lat_s') int? tLatS,
  }) = _HandshakeDto;
  factory HandshakeDto.fromJson(Map<String, dynamic> json) => _$HandshakeDtoFromJson(json);
}

@freezed
abstract class ProblemaDto with _$ProblemaDto {                // application/problem+json
  const factory ProblemaDto({required int status, required String codigo, String? title, String? detail}) = _ProblemaDto;
  factory ProblemaDto.fromJson(Map<String, dynamic> json) => _$ProblemaDtoFromJson(json);
}
```

Gerar com `fvm dart run build_runner build --delete-conflicting-outputs`.

### Datasource (`lib/src/data/datasources/sync_api_datasource.dart`)

Só faz HTTP. Não conhece domínio, token guardado nem retentativas.

```dart
class SyncApiDatasource {
  const SyncApiDatasource(this._dio);
  final Dio _dio;

  /// POST /v1/dongles/{devIdHex}/desafio (corpo vazio).
  Future<DesafioDto> pedirDesafio(String devIdHex);

  /// POST /v1/dongles/{devIdHex}/desafio/resposta, JSON {"resposta": b64}.
  Future<HandshakeDto> enviarResposta(String devIdHex, String respostaB64);

  /// POST /v1/dongles/blocos, octet-stream = concatenação dos TLVs; Bearer [token].
  /// Devolve a resposta crua (200 ou 422 trazem o ACK no corpo).
  Future<RespostaBinaria> enviarLote(Uint8List corpo, String token);

  /// GET /v1/dongles/{devIdHex}/confirmacao; Bearer [token].
  Future<RespostaBinaria> obterConfirmacao(String devIdHex, String token);
}

/// Resposta binária crua: status, corpo e os headers relevantes.
@immutable
class RespostaBinaria {
  const RespostaBinaria(this.status, this.corpo, this.headers);
  final int status;
  final Uint8List corpo;
  final Map<String, String> headers;    // chaves em minúsculas
}

/// Status ≠ 2xx esperado: carrega o problem+json (se houver) e o Retry-After.
class SyncApiException implements Exception {
  const SyncApiException(this.status, {this.codigo, this.retryAfter, this.corpo});
  final int status;
  final String? codigo;
  final Duration? retryAfter;
  final Uint8List? corpo;
}
```

Headers enviados:

- JSON: `Content-Type: application/json`, `Accept: application/json`.
- Lote: `Content-Type: application/octet-stream`, `Accept: application/octet-stream`, `Authorization: Bearer …`.

`Retry-After` é lido em segundos inteiros.

### Falhas (`lib/src/core/errors/sync_server_failure.dart`)

```dart
sealed class SyncServerFailure extends Failure {
  const SyncServerFailure(super.message, {this.retryAfter});
  final Duration? retryAfter;
}
final class TokenRecusadoFailure extends SyncServerFailure { ... }        // 401
final class TokenOutroDongleFailure extends SyncServerFailure { ... }     // 403 token_outro_dongle
final class DongleBloqueadoFailure extends SyncServerFailure { ... }      // 403 dongle_inativo / 404 dongle_desconhecido / perfil_nao_permitido
final class DesafioRecusadoFailure extends SyncServerFailure { ... }      // 409
final class RespostaRecusadaFailure extends SyncServerFailure { ... }     // 422 tag_invalida / resposta_inconsistente
final class LimiteTaxaFailure extends SyncServerFailure { ... }           // 429 (retryAfter obrigatório)
final class ServidorIndisponivelFailure extends SyncServerFailure { ... } // 5xx, 503, timeout, sem rede
final class RequisicaoInvalidaFailure extends SyncServerFailure { ... }   // 400/413/415 (bug do app)
final class SemConfirmacaoFailure extends SyncServerFailure { ... }       // 404 sem_confirmacao
```

### Domínio (`lib/src/domain/sync/`; reaproveitar o que a Fase 1 já criou)

```dart
@immutable class ResultadoHandshake {
  const ResultadoHandshake({required this.ackTlv, required this.ack, required this.tLatSugerido});
  final Uint8List ackTlv;            // vai como está para o SYNC_RX
  final AckInfo ack;                  // leitura dos campos, sem verificar a tag
  final Duration? tLatSugerido;
  bool get ancoraFalhou => ack.flags & 0x01 != 0;
}

enum StatusLote { completo, interrompido }
enum AcaoLote { nenhuma, reler, aguardar }

@immutable class ResultadoLote {
  const ResultadoLote({required this.ackTlv, required this.ack, required this.status,
                       required this.acao, this.motivo, this.semProgresso = false});
  final Uint8List ackTlv;
  final AckInfo ack;
  final StatusLote status;
  final AcaoLote acao;
  final String? motivo;              // X-Lote-Motivo, para log
  final bool semProgresso;           // status 422
}

/// Campos do ACK 0x04 lidos do TLV (offsets: T=0, L=1..2, ACK_seq=3..6 LE, c_conf=7..10 LE, flags=11).
@immutable class AckInfo {
  factory AckInfo.deTlv(Uint8List tlv);   // valida T=0x04 e L=25; senão FormatException
  final int ackSeq; final int cConf; final int flags;
}
```

### Adapter HTTPS (`lib/src/infra/sync/https_sync_server_gateway.dart`)

**[Reconciliado]** O que esta seção chama de `SyncServerRepositoryImpl` é, nos nomes da Fase 1, o adapter `HttpsSyncServerGateway implements SyncServerGateway` (camada `infra/`), sem estado de sessão: o token vive no `SyncServerRepository` da Fase 1 e chega como argumento em `postBlocks`/`latestConfirmation`. O mapa `_sessoes` e `tokenValidoPor`/`descartarToken` abaixo **não** são implementados aqui (já existem na Fase 1 como `needsHandshake`/`clearToken`).

```dart
class SyncServerRepositoryImpl implements SyncServerGateway {   // nome real: HttpsSyncServerGateway
  SyncServerRepositoryImpl(this._ds, {DateTime Function()? relogio, Stopwatch? cronometro});
  final SyncApiDatasource _ds;
  final _sessoes = <DevId, _Sessao>{};   // token em memória
  ...
}

class _Sessao {
  _Sessao(this.token, this.recebidoEm, this.validade);  // recebidoEm = Stopwatch monotônico
  final String token; final Duration recebidoEm; final Duration validade;
}
```

Responsabilidades:

- `pedirDesafio`: `ds.pedirDesafio(dev.hex)` → `BytesBase64.decodificar(dto.desafio)`. Valida só a forma (`T=0x01`, `L ∈ {45, 47}`); senão `RequisicaoInvalidaFailure`.
- `enviarResposta`: `codificar(respostaTlv)` → `ds.enviarResposta` → guarda `_Sessao(token, agoraMonotonico, Duration(seconds: dto.tokenValidadeS))` → `ResultadoHandshake`.
- `enviarLote`:
  - sem sessão: lança `TokenRecusadoFailure`, que dispara o handshake;
  - concatena os TLVs num `BytesBuilder(copy: false)` → `ds.enviarLote`;
  - corpo com `Content-Type: application/octet-stream` (200, ou 422 `lote_sem_progresso`): `AckInfo.deTlv(corpo)` + headers `x-lote-*` → `ResultadoLote`. A regra é **"o corpo é ACK ⇔ content-type `octet-stream`"**, nunca o status: os outros 422 da API são `problem+json`;
  - demais status: lança a falha mapeada.
- `tokenValidoPor(dev, margem)`: `restante = validade − (agoraMonotonico − recebidoEm)`; devolve `restante > margem`.
- Erros crus (`DioException` de rede/timeout, `SyncApiException`, `FormatException`) viram `SyncServerFailure`. O status 401 também descarta a sessão.

**[Decisão] Token só em memória, não no `flutter_secure_storage`.**

- O dongle liga o bit 0 do aviso 0x07 ("sem handshake nesta conexão") a cada conexão BLE nova, e o buffer de blocos do app é em memória. Logo, depois de reiniciar o app, **sempre** há um handshake antes do primeiro lote, e um token persistido nunca seria reutilizado.
- Guardar em disco só aumentaria a superfície (backup do aparelho, root) sem benefício.
- "Armazenamento seguro" aqui significa: não persistir, não logar (o interceptor mascara), descartar em 401/403 e ao trocar de dongle (`ConnectionGuard`/"Esquecer dispositivo" chamam `descartarToken`).
- Se uma fase futura persistir o buffer em disco, a mesma interface ganha uma implementação com `flutter_secure_storage` ([Aberto]).

### Use cases (`lib/src/application/sync/`)

Eles combinam o port do dongle (Fase 1) com o `SyncServerGateway`, que são dois repositórios, por isso entram em `application/` (CLAUDE.md §5.1).

```dart
/// Handshake completo: servidor → dongle → servidor → dongle. Single-flight por DevId.
class RealizarHandshakeUseCase {
  RealizarHandshakeUseCase(this._dongle, this._servidor, this._espera, {this.maxTentativas = 3});
  Future<ResultadoHandshake> call(DevId dev);
}

/// Um POST de lote com a política da Tabela 5; devolve o que fazer em seguida.
class EnviarLoteUseCase {
  EnviarLoteUseCase(this._dongle, this._servidor, this._buffer, this._handshake, this._politica);
  Future<ResultadoEnvio> call(DevId dev);
}

/// Política de reenvio pura (testável): backoff exponencial com jitter total.
class PoliticaReenvio {
  PoliticaReenvio({this.minimo = const Duration(seconds: 1), this.maximo = const Duration(minutes: 5), Random? aleatorio});
  Duration proximaEspera();          // min(máximo, mínimo·2^n) · U(0,1], com piso de 1 s
  void sucesso();                    // zera n
}

/// K adaptativo (blocos por POST): começa em 32; 429 e 5xx ⇒ metade (≥ 1); sucesso ⇒ dobro (≤ 32).
class LoteAdaptativo { int get k; void reduzir(); void sucesso(); }
```

#### Fluxo do handshake (`RealizarHandshakeUseCase`)

```
1. tlv01 := servidor.pedirDesafio(dev)
     DesafioRecusado/LimiteTaxa/Indisponível → backoff (Retry-After se houver) e repetir até maxTentativas
2. dongle.escreverRx(tlv01)                         ← PRIORIDADE: nada de fila atrás de outros envios BLE
3. tlv02 := dongle.aguardarMensagem(0x02, timeout: 5 s)
     Erro 0x01 (tag inválida) ou 0x02 (c_S antigo) do dongle → voltar ao passo 1 (novo desafio)
4. r := servidor.enviarResposta(dev, tlv02)         ← imediatamente, sem esperar T_lat
     409 → passo 1; 422 → backoff e passo 1; 429/5xx → backoff e REENVIAR a mesma resposta enquanto o desafio vale (60 s)
5. dongle.escreverRx(r.ackTlv)
6. se r.ancoraFalhou: agendar novo handshake em 2 s (no máximo 3 seguidos; depois só no próximo gatilho natural)
```

**Orçamento de latência da âncora (S2 − S1 ≤ 2 s).** Os passos 1 → 4 incluem duas idas e voltas HTTPS e uma BLE. Com 4G (~100–300 ms de RTT) cabe com folga, desde que o app **não** enfileire o desafio atrás de leituras de lote no BLE nem espere `T_lat`. Os use cases pausam o envio de lotes durante o handshake. O tempo entre o recebimento do desafio e o envio da resposta é medido e logado (`sync.handshake_rtt_app_ms`).

#### Fluxo do lote (`EnviarLoteUseCase`)

```
0. se !servidor.tokenValidoPor(dev, 5 min) → handshake.call(dev)        ← gatilho "token ausente/expirando"
1. blocos := buffer.primeiros(lote.k); se vazio → nada
2. r := servidor.enviarLote(dev, blocos)
3. sucesso (200, ou 422 sem progresso):
     buffer.removerAte(r.ack.ackSeq); dongle.escreverRx(r.ackTlv); politica.sucesso(); lote.sucesso()
     se r.acao == reler    → buffer.descartarAcimaDe(r.ack.ackSeq); dongle.enviarErro(0x07)
     se r.acao == aguardar → manter o buffer; próxima tentativa após politica.proximaEspera()
     se r.motivo == marcador_invalido 3× com o mesmo ackSeq → handshake.call(dev)
4. falhas:
     TokenRecusado (401)        → handshake.call(dev) e repetir o MESMO lote uma vez
     TokenOutroDongle (403)     → servidor.descartarToken(dev); handshake.call(dev); repetir
     DongleBloqueado (403/404)  → parar o envio e avisar o usuário (estado de erro no view_model)
     LimiteTaxa (429)           → esperar Retry-After; lote.reduzir()
     ServidorIndisponível (5xx/503/rede) → esperar max(Retry-After, politica.proximaEspera()); lote.reduzir(); lote fica no buffer
     RequisicaoInvalida (400)   → bug: log de erro; buffer.descartarAcimaDe(ACK conhecido); dongle.enviarErro(0x07);
                                  400 repetido 3× com os mesmos bytes relidos ⇒ estado bloqueado ("bloco malformado no dongle";
                                  o operador resolve com `tccelta admin saltar`, spec 05)
```

Nada é apagado do buffer sem um ACK, e o dongle só apaga a flash com o ACK assinado. Uma falha nunca perde dados, só atrasa.

#### Gatilhos de handshake (preenchem o "molde" da Fase 1)

| Gatilho | Origem | Ação |
|---|---|---|
| Token ausente ou com menos de 5 min | antes de cada POST | `RealizarHandshakeUseCase` |
| Aviso 0x07 com bit 0 ou bit 1 ligado | `dongle.avisos` | handshake antes de qualquer pedido de lote |
| Erro 0x05 do dongle (sem sessão) | resposta a um pedido de lote | reescrever o último ACK de abertura se ainda estiver na sessão; senão handshake |
| Erro 0x01/0x02 do dongle após um desafio | passo 3 do handshake | novo desafio |
| 401 / 403 `token_outro_dongle` | `enviarLote` | handshake e reenvio |
| 409 | `enviarResposta` | novo desafio |
| `flags_a` bit 0 no ACK | resultado do handshake | novo handshake em 2 s (máx. 3 seguidos) |
| `marcador_invalido` 3× | `enviarLote` | handshake |

Coordenação: o handshake é *single-flight* por DevId (um `Completer` compartilhado: gatilhos simultâneos esperam o mesmo resultado). O envio de lotes fica suspenso enquanto há handshake em curso. O ponto que hoje é o "molde" da Fase 1 (um view_model ou serviço de sessão SYNC) passa a chamar esses use cases, e a UI só observa o estado (`SyncEstado`: `semToken`, `handshakeEmCurso`, `enviando`, `aguardandoReenvio(ate)`, `bloqueado(motivo)`).

### Providers (`lib/src/ui/sync/sync_providers.dart` ou o arquivo de providers que a Fase 1 criar)

```dart
// Fase 1, spec 09 §8: o provider já seleciona por --dart-define=SYNC_SERVER; esta fase acrescenta o caso 'https'
// e o torna o padrão (hoje o padrão em release é UnavailableSyncServerGateway).
final Provider<SyncServerGateway> syncServerGatewayProvider = Provider((ref) {
  const mode = String.fromEnvironment('SYNC_SERVER', defaultValue: 'https');
  return switch (mode) {
    'bench' when !kReleaseMode => BenchSyncServerGateway(...),          // Fase 1
    'stub'  when !kReleaseMode => const UnavailableSyncServerGateway(), // Fase 1
    _ => HttpsSyncServerGateway(ref.watch(syncDioProvider), ref.watch(appEnvironmentProvider)),
  };
});
// HandshakeRunner, SyncCoordinator e seus providers são da Fase 1 e não mudam;
// os gatilhos novos (tabela acima) entram no SyncCoordinator.
```

### Rede no Android/iOS (certificado do Caddy em desenvolvimento)

- `android/app/src/debug/res/xml/network_security_config.xml`:

  ```xml
  <network-security-config>
    <debug-overrides>
      <trust-anchors>
        <certificates src="system" />
        <certificates src="user" />   <!-- CA local do Caddy instalada no aparelho -->
      </trust-anchors>
    </debug-overrides>
    <domain-config cleartextTrafficPermitted="true">
      <domain includeSubdomains="false">10.0.2.2</domain>   <!-- emulador → API :8080 sem TLS -->
    </domain-config>
  </network-security-config>
  ```

  Ele é referenciado só no `android/app/src/debug/AndroidManifest.xml` (`android:networkSecurityConfig="@xml/network_security_config"`). O release confia só nas CAs do sistema e não permite HTTP.
- Instalar a CA: `make ca-exportar` (spec 01) → copiar `saida/caddy-root.crt` para o aparelho → Configurações → Segurança → Instalar certificado CA.
- iOS: instalar o `.crt` como perfil e habilitar em Ajustes → Geral → Sobre → Ajustes de Confiança de Certificados.
- `API_HOST` do Caddy = IP do PC na LAN; `env/dev.json` aponta `https://<IP>:8443`.

## Plano arquivo por arquivo

| Arquivo | Ação |
|---|---|
| `mobile-app/env/dev.json`, `env/prodlike.json`; `.gitignore` (`env/*.local.json`) | criar |
| `lib/src/core/config/app_environment.dart` | criar |
| `lib/src/core/network/dio_client.dart`, `dio_provider.dart`, `log_interceptor.dart` | criar |
| `lib/src/core/utils/bytes_base64.dart` | criar |
| `lib/src/core/errors/sync_server_failure.dart` | criar |
| `lib/src/data/dtos/{desafio,resposta_desafio,handshake,problema}_dto.dart` (+ `.freezed.dart`/`.g.dart` gerados) | criar |
| `lib/src/data/datasources/sync_api_datasource.dart` | criar |
| `lib/src/data/repositories/sync_server_repository_impl.dart` | criar (implementa o port da Fase 1) |
| `lib/src/domain/sync/{resultado_handshake,resultado_lote,ack_info}.dart` | criar, ou estender se a Fase 1 já tiver `AckInfo` |
| `lib/src/application/sync/{realizar_handshake,enviar_lote}_use_case.dart`, `politica_reenvio.dart`, `lote_adaptativo.dart` | criar |
| providers da feature sync (arquivo da Fase 1) | trocar o stub do gateway pela impl real; registrar os use cases |
| view_model de sessão SYNC (Fase 1) | ligar os gatilhos aos use cases |
| `android/app/src/debug/res/xml/network_security_config.xml` + manifest debug | criar/editar |
| `test/support/fake_http_adapter.dart` | criar (`HttpClientAdapter` roteirizado; sem dependência nova) |
| `test/support/fake_sync_dongle.dart`, `fake_sync_server_gateway.dart` | criar (ou reutilizar os da Fase 1) |
| `test/fixtures/protocolo_vetores.json` | cópia de `backend/internal/protocolo/testdata/vetores.json` (`make vetores` copia) |
| testes listados abaixo | criar |

`pubspec.yaml`: **nenhuma dependência nova** (`dio`, `freezed`, `json_serializable` e `mocktail` já existem).

## Testes (`fvm flutter test`; dublê de `dio` via `FakeHttpAdapter`)

1. `bytes_base64_test.dart`: ida e volta dos TLVs dos vetores; rejeita `-`/`_`, falta de padding e espaços; bytes de saída idênticos aos de entrada (incluindo `0x00` e `0xFF`).
2. `sync_api_datasource_test.dart`:
   - `pedirDesafio` faz `POST /v1/dongles/0011223344556677/desafio` com corpo vazio;
   - `enviarResposta` envia JSON `{"resposta": "<b64>"}` com `Content-Type` correto;
   - `enviarLote` envia `octet-stream` = concatenação exata e `Authorization: Bearer t`;
   - lê os headers `X-Lote-*` em minúsculas;
   - `Retry-After: 7` vira `Duration(seconds: 7)`.
3. `sync_server_repository_impl_test.dart`:
   - mapeamento de cada status ⇒ falha (tabela da spec 03);
   - token guardado e `tokenValidoPor` com cronômetro falso (59 min ⇒ `false` para margem de 5 min);
   - 401 descarta a sessão;
   - 200 ⇒ `ResultadoLote` com `AckInfo` correto;
   - 422 com `octet-stream` ⇒ `semProgresso = true` e ACK lido do corpo; 422 com `problem+json` ⇒ falha mapeada (nunca tenta ler ACK);
   - TLV de desafio com `L = 46` ⇒ `RequisicaoInvalidaFailure`.
4. `realizar_handshake_use_case_test.dart`:
   - feliz (o dongle recebe exatamente os bytes do servidor; o servidor recebe exatamente os bytes do dongle);
   - Erro 0x02 do dongle ⇒ novo desafio;
   - 409 ⇒ novo desafio;
   - 503 na resposta ⇒ reenvia a mesma resposta;
   - `flags_a` ⇒ reagenda (máx. 3);
   - dois gatilhos simultâneos ⇒ um único pedido de desafio (*single-flight*).
5. `enviar_lote_use_case_test.dart`:
   - token a 4 min do fim ⇒ handshake antes do POST;
   - 200 completo ⇒ buffer reduzido e ACK escrito no dongle;
   - `reler` ⇒ `descartarAcimaDe` + Erro 0x07;
   - `aguardar` ⇒ buffer intacto;
   - 429 com `Retry-After: 3` ⇒ espera ≥ 3 s e `k` 32 → 16;
   - 503 ⇒ backoff ≥ 1 s, `k` pela metade, lote mantido;
   - 401 ⇒ handshake + reenvio do mesmo lote;
   - 403 `dongle_inativo` ⇒ estado bloqueado, sem retentativa.
6. `politica_reenvio_test.dart`: as esperas crescem de 1 s até no máximo 5 min; `sucesso()` zera; o jitter fica dentro dos limites (`Random` com semente).

Antes de dar a tarefa por pronta: `fvm dart format .` → `fvm flutter analyze` ("No issues found!") → `fvm flutter test` (regra do CLAUDE.md).

## Passos ordenados com checkpoints

1. `AppEnvironment`, `dio_client`, `BytesBase64`, falhas. **Checkpoint:** testes 1 e 6 verdes; `flutter analyze` limpo.
2. DTOs + datasource + `FakeHttpAdapter`. **Checkpoint:** teste 2.
3. Repository implementando o port da Fase 1. **Checkpoint:** teste 3; troca do provider do stub pela implementação real.
4. Use cases + gatilhos no view_model da Fase 1. **Checkpoint:** testes 4 e 5.
5. Configuração de rede Android + CA do Caddy. **Checkpoint:** num aparelho real na LAN, `GET /saude/pronta` via `dio` (botão de debug temporário ou teste manual) responde 200 em HTTPS.
6. Ponta a ponta com o simulador de dongle *no lugar do BLE* (port do dongle dublado, servidor real) e depois com o dongle real. **Checkpoint:** roteiro do README.

## Critérios de aceite

- [ ] A cadeia `datasource → repository → (use case) → view_model` é respeitada; nenhum view_model fala com o datasource.
- [ ] Os bytes que chegam ao SYNC_RX são idênticos aos enviados pelo servidor, e os que chegam ao servidor são idênticos aos do SYNC_TX (testes com vetores).
- [ ] Todos os gatilhos da tabela disparam o handshake, e o handshake é *single-flight*.
- [ ] 429/5xx/sem rede nunca descartam blocos do buffer; o backoff respeita 1 s–5 min e o `Retry-After`; K cai pela metade.
- [ ] O token nunca é persistido nem logado.
- [ ] `format`, `analyze` e `test` limpos.

## Riscos

| Risco | Mitigação |
|---|---|
| Nomes e assinaturas dos ports da Fase 1 diferentes do suposto | o contrato está registrado no README; a adaptação é mecânica e a semântica está fixada aqui |
| RTT do celular > 2 s ⇒ âncoras sempre falhando | prioridade do desafio no BLE; reenvio imediato; métrica no app e no servidor (`tccelta_ancora_rtt_segundos`); sem âncora o handshake ainda vale |
| CA do Caddy não aceita em algum aparelho | túnel com certificado público ([Aberto] da spec 01) |
| Relógio do celular errado | expiração calculada pelo cronômetro monotônico + `token_validade_s` |

## [Aberto]

- **[Aberto] Sobrescrever a URL base em Configurações** (só em build debug, persistida no `SettingsService`, no padrão de `psk_hex`). Recomendação: fazer se trocar o IP da LAN virar atrito no teste de campo.
- **[Aberto] `flutter_secure_storage`** para o token, caso o buffer passe a ser persistido em disco.
- **[Aberto] Usar `t_lat_s` sugerido pelo servidor.** Recomendação: o app só registra o valor nesta fase; a configuração local de `T_lat` (Fase 1) continua valendo, como o doc define para o TCC.
