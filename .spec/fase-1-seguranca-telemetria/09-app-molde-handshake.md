# 09 — App: molde do handshake e do servidor (port para a Fase 3, stub e servidor de bancada)

## Objetivo

Deixar o app pronto para o handshake da Fig. 5 e para o envio de lotes sem
ainda falar HTTPS: (1) definir o **port** `SyncServerGateway` com o contrato
exato que a Fase 3 implementará sobre `dio` (pedido de desafio, envio da
resposta, POST de lotes, confirmação), com tipos binários (`SyncFrame`),
conversão base64 ⇄ bytes e o mapeamento dos erros 401/403/404/409/422/429/5xx;
(2) implementar a **perna BLE** do handshake (`HandshakeRunner`: repassa o
Desafio 0x01 ao dongle, coleta a Resposta 0x02, repassa ao servidor, escreve
o ACK 0x04 no dongle); (3) entregar duas implementações do port: **stub**
(sem servidor; o app fica em "handshake pendente" com espera exponencial) e
**servidor de bancada** em memória, com as chaves de desenvolvimento, que
permite validar ponta a ponta a leitura contínua (Fig. 15) e o handshake
(Fig. 5) com o dongle real.

## Escopo

**Dentro:** port, *envelopes* e exceções; `SyncServerDatasource`;
`SyncServerRepository` (token, mapeamento para resultados); `HandshakeRunner`
e `HandshakeTrigger`; `UnavailableSyncServerGateway`; `BenchSyncServer`
(lógica pura) + `BenchSyncServerGateway`; utilitário base64; seleção por
`--dart-define`; testes; contrato para a Fase 3.

**Fora:** `HttpsSyncServerGateway`, `dio`, JWT real, KMS (Fase 3); regras de
quando disparar o handshake (spec 07 — coordenador); verificação de
assinatura ECDSA no servidor de bancada (ver Riscos).

## Estado atual (verificado no código)

| Peça | Onde | Hoje |
|---|---|---|
| HTTP | — | Nenhuma camada HTTP; `dio ^5.10.0` no `pubspec.yaml` sem uso. |
| Criptografia | `lib/src/core/crypto/hkdf.dart` | `pointycastle` com `HMac(SHA256Digest(), 64)` já em uso (camada PSK legada). |
| Contrato HTTP da Fase 3 | `.spec/fase-3-servidor/03-api-go-arquitetura.md` (§HTTP) | `POST /v1/dongles/{id}/desafio` → `200 JSON {"desafio":"<b64 TLV 0x01>","expira_em":"<RFC3339>"}`; `POST /v1/dongles/{id}/desafio/resposta` com `{"resposta":"<b64 TLV 0x02>"}` → `{"ack":"<b64 TLV 0x04>","token":"<JWT>","token_expira_em":"<RFC3339>","t_lat_s":60}`; `POST /v1/dongles/blocos` (`Bearer`, `application/octet-stream`, 1–32 TLVs 0x03 concatenados) → `200 octet-stream` (TLV 0x04); `GET /v1/dongles/{id}/confirmacao` → TLV 0x04; erros `application/problem+json` com `codigo`; base64 RFC 4648 §4 com *padding*; `{id}` = DevId em 16 hex. |

## Design

### 1. Camadas

```
SyncCoordinator / HandshakeRunner  (application/sync)
        │
SyncServerRepository  (domain/repositories + data/repositories)   ← token, resultados, Failure
        │
SyncServerDatasource  (data/datasources)                           ← valida tipos/tamanhos dos TLVs
        │
SyncServerGateway  (services/sync — PORT)
        ├── UnavailableSyncServerGateway  (infra/sync — stub)
        ├── BenchSyncServerGateway        (infra/sync/bench — debug)
        └── HttpsSyncServerGateway        (infra/sync — Fase 3, dio)
```

O par port (`services/`) + adapter (`infra/`) segue o `CLAUDE.md`: trocar de
implementação é trocar **um** provider (`syncServerGatewayProvider`).

### 2. Port `SyncServerGateway` (`lib/src/services/sync/sync_server_gateway.dart`)

```dart
/// Servidor de sincronização visto pelo app. Transporta mensagens TLV
/// (`SyncFrame`) sem interpretá-las: o app nunca altera os bytes.
abstract interface class SyncServerGateway {
  /// 'stub', 'bench' ou 'https' (exibido na UI).
  String get mode;

  /// POST /v1/dongles/{devId}/desafio. Devolve o Desafio 0x01 (V com 45/47 B).
  Future<ChallengeEnvelope> requestChallenge(DevId devId);

  /// POST /v1/dongles/{devId}/desafio/resposta com a Resposta 0x02.
  Future<HandshakeEnvelope> submitResponse(DevId devId, SyncFrame response);

  /// POST /v1/dongles/blocos com 1–32 blocos 0x03, na ordem. Devolve o ACK 0x04.
  Future<SyncFrame> postBlocks(String token, List<SyncFrame> blocks);

  /// GET /v1/dongles/{devId}/confirmacao (opcional; `null` = sem confirmação).
  Future<SyncFrame?> latestConfirmation(DevId devId, String token);
}

@immutable
class ChallengeEnvelope {
  const ChallengeEnvelope({required this.challenge, this.expiresAt});
  final SyncFrame challenge;          // tipo 0x01
  final DateTime? expiresAt;
}

@immutable
class HandshakeEnvelope {
  const HandshakeEnvelope({required this.ack, required this.token,
                           required this.tokenExpiresAt, this.suggestedLatency});
  final SyncFrame ack;                // tipo 0x04
  final String token;                 // JWT (opaco para o app)
  final DateTime tokenExpiresAt;      // UTC
  final Duration? suggestedLatency;   // t_lat_s (outros campos do JSON, ex. token_validade_s, são ignorados)
}
```

Exceções do port (`lib/src/services/sync/sync_server_exceptions.dart`):

| Exceção | HTTP (Fase 3) | `codigo` | Onde |
|---|---|---|---|
| `ServerUnauthorized` | 401 | `token_ausente`, `token_invalido`, `token_expirado` | blocos, confirmação |
| `ServerForbiddenOtherDongle` | 403 | `token_outro_dongle` | confirmação |
| `ServerDongleBlocked(reason)` | 403 / 404 | `dongle_inativo` / `dongle_desconhecido` | todos / desafio, resposta |
| `ServerNoConfirmation` | 404 | `sem_confirmacao` | confirmação |
| `ServerChallengeRejected` | 409 | `desafio_invalido`, `c_s_antigo` | resposta |
| `ServerResponseRejected(inconsistent)` | 422 | `tag_invalida` / `resposta_inconsistente` | resposta |
| `ServerBatchNoProgress(ack)` | 422 | `lote_sem_progresso` (corpo = TLV 0x04) | blocos |
| `ServerRateLimited(retryAfter)` | 429 | `limite_taxa`, `lote_grande` | todos |
| `ServerUnavailable(retryAfter?)` | 5xx/503, *timeout*, sem rede | `sobrecarga`, `kms_indisponivel`, `banco_ocupado` | todos |
| `ServerMalformed(code)` | 400 / 413 / 415 | `corpo_malformado`, `base64_invalido`, `tlv_invalido`, `lote_malformado`, `devid_invalido`, `corpo_grande`, `tipo_conteudo` | todos |

Mapeamento de bytes (o que a Fase 3 implementa no `HttpsSyncServerGateway`):

| Método | Requisição | Resposta 2xx → retorno |
|---|---|---|
| `requestChallenge` | `POST /v1/dongles/{devId.hex}/desafio`, corpo vazio | JSON → `SyncFrame.parseTlv(Base64Bytes.decode(desafio))` (exigir tipo 0x01), `expiresAt = DateTime.parse(expira_em)` |
| `submitResponse` | JSON `{"resposta": Base64Bytes.encode(response.toTlv())}` | `ack` = TLV 0x04 decodificado; `token`; `token_expira_em`; `t_lat_s` (ausente/0 → `null`) |
| `postBlocks` | `Authorization: Bearer <token>`, `Content-Type: application/octet-stream`, corpo = concatenação de `block.toTlv()` | Corpo = TLV 0x04 (`parseTlv`) |
| `latestConfirmation` | `GET …/confirmacao`, `Bearer` | Corpo = TLV 0x04; 404 `sem_confirmacao` → `null` |

Regras do adapter HTTPS (para a Fase 3): sem novas tentativas internas (a
política é do coordenador); `Retry-After` em segundos inteiros; *timeouts*
de conexão 10 s e de resposta 20 s; corpo de erro `problem+json` lido só
para escolher a exceção pelo `codigo`; nunca registrar token nem tag
completos em log.

### 3. Base64 (`lib/src/core/encoding/base64_bytes.dart`)

```dart
/// Base64 padrão (RFC 4648 §4) com padding — o formato aceito pelo servidor.
abstract final class Base64Bytes {
  static String encode(Uint8List bytes);          // base64.encode
  static Uint8List decode(String text);           // valida antes com RegExp(r'^[A-Za-z0-9+/]*={0,2}$') e length % 4 == 0 (o base64.decode do Dart aceita - e _); senão FormatException
}
```

### 4. Datasource e repository

`SyncServerDatasource(SyncServerGateway)` (`lib/src/data/datasources/`):
repassa as chamadas e **valida o formato** do que volta (desafio tipo 0x01
com `L` 45/47; ACK tipo 0x04 com `L` 25), lançando `ServerMalformed` se o
servidor devolver algo fora do contrato.

`SyncServerRepository` (`domain/repositories/` + `data/repositories/`):

```dart
@immutable
class SyncToken { final String value; final DateTime expiresAt; }

@immutable
class HandshakeOutcome {
  final SyncFrame ack;                 // a escrever no dongle
  final SyncToken token;
  final Duration? suggestedLatency;
}

sealed class PostResult {}
final class BlocksAccepted extends PostResult { final SyncFrame ack; }           // 200 e 422 lote_sem_progresso
final class Unauthorized extends PostResult {}                                   // 401
final class Forbidden extends PostResult {}                                      // 403 token_outro_dongle
final class DongleBlocked extends PostResult { final String reason; }           // 403 inativo / 404
final class RateLimited extends PostResult { final Duration retryAfter; }       // 429
final class ServerUnavailable extends PostResult { final Duration? retryAfter; } // 5xx, rede
final class MalformedRequest extends PostResult { final String code; }          // 400/413/415

abstract interface class SyncServerRepository {
  String get mode;
  SyncToken? get token;
  bool needsHandshake(DateTime now, Duration margin);   // token nulo ou expira em < margin
  Future<SyncFrame> requestChallenge(DevId devId);                       // lança SyncServerFailure
  Future<HandshakeOutcome> submitResponse(DevId devId, SyncFrame response); // guarda o token
  Future<PostResult> postBlocks(List<SyncFrame> blocks);                 // usa o token guardado
  void clearToken();
}
```

`SyncServerFailure` (`lib/src/core/errors/sync_failure.dart`, subclasses de
`Failure`): `ChallengeUnavailable`, `ChallengeRejected`, `ResponseRejected`,
`DongleBlockedFailure`, `RateLimitedFailure(retryAfter)`,
`ServerUnavailableFailure(retryAfter)`, `ServerContractFailure`. O token é
guardado só em memória (`SyncToken`), nunca persistido. **[Decisão]**
Token em memória: dura ~1 h; reabrir o app exige um handshake novo de
qualquer forma (conexão BLE nova).

### 5. `HandshakeRunner` (`lib/src/application/sync/handshake_runner.dart`)

Perna BLE da Fig. 5 (sem o gatilho, que é do coordenador):

```dart
enum HandshakeTrigger { noticeNoSession, noticeNoBootAnchor, tokenExpiring, unauthorized,
                        dongleNoSession, dongleRejected, anchorFailed, manual }

class HandshakeRunner {
  HandshakeRunner({required DongleSyncRepository dongle, required SyncServerRepository server,
                   Duration stepTimeout = kHandshakeStepTimeout});
  /// [generation]: geração do link SYNC no início; se mudar durante qualquer await, lança HandshakeLinkLost.
  Future<HandshakeOutcome> run(DevId devId, {required HandshakeTrigger trigger, required int generation});
}
```

| Passo | Ação | Falha → exceção |
|---|---|---|
| 0 | Assina `dongle.events` **antes** de qualquer envio (evita corrida com a resposta) | — |
| 1 | `challenge = await server.requestChallenge(devId)`; `ChallengeInfo.fromValue` | `SyncServerFailure` do repository |
| 2 | `await dongle.sendChallenge(challenge)` | `SyncLinkFailure` |
| 3 | Espera o primeiro de: `ResponseReceived` com `info.cS == challenge.cS`; `DongleErrorReceived(0x01\|0x02)`; `SyncLinkChanged(up: false)`; *timeout* de 5 s | `HandshakeRejectedByDongle(code)`, `HandshakeLinkLost`, `HandshakeTimeout` |
| 4 | `outcome = await server.submitResponse(devId, response.frame)` | 409 → `ChallengeRejected`; 422 → `ResponseRejected`; outros |
| 5 | `await dongle.sendAck(outcome.ack)` | `SyncLinkFailure` |
| 6 | Espera a confirmação positiva — o `NoticeReceived` com bit 0 desligado que o dongle envia após um ACK de abertura válido — ou `DongleErrorReceived(0x01\|0x03)` (→ `HandshakeAckRejected(code)`), com *timeout* de 1 s (sem confirmação: segue, e um eventual 0x05 no pedido de lote corrige) | — |
| 7 | Devolve `outcome` (o token já está no repository) | — |

Cada passo sai no log estruturado (`SYNC <ms> HS_STEP n=… trigger=… cS=…`),
com o tempo de cada perna (servidor × BLE), base do benchmark do lado app.

### 6. `UnavailableSyncServerGateway` (stub)

`mode = 'stub'`. Todos os métodos lançam `ServerUnavailable(retryAfter: Duration(minutes: 5))`.
Efeito: o coordenador fica em `handshakePending` mostrando "servidor ainda
não integrado (Fase 3)"; o dongle continua gravando na flash, como no ramo
"sem conexão" da Fig. 14. É o padrão em `kReleaseMode`.

### 7. Servidor de bancada

`lib/src/infra/sync/bench/bench_sync_server.dart` — lógica pura (sem
Flutter, sem rede), testável, com o comportamento do documento §4.3–4.4:

| Item | Comportamento |
|---|---|
| Chaves | `DevId`, `K_auth`, `K_rec` fixos da spec 01 (§2), ou `--dart-define=SYNC_BENCH_KAUTH=<hex64>` / `SYNC_BENCH_KREC=<hex64>` (vindos do `bench_keys.json` do `provision.py --profile dev`). |
| Estado por `DevId` | `lastIssuedCs`, `lastAcceptedCs`, desafios pendentes (`cS → nonce, S1, usado`), `ackSeq` (seq contíguo), `head`, `tail`, lacunas registradas (`from`, `to`, causa BOOT/LOSS/OVERFLOW). Só memória; "Esquecer estado do dongle" zera tudo menos `lastIssuedCs`. |
| Desafio | `cS = max(lastIssuedCs + 1, segundos desde a época Unix)` (cabe em u32 até 2106; mantém `c_S` crescente mesmo após reiniciar o app, porque o dongle persiste `last_cs`); `N_S` = 16 B de `Random.secure()`; `ACK_seq = ackSeq`; `c_conf = lastAcceptedCs`; `flags = 0`; tag `HMAC-SHA256_{K_auth}("MALCONG"‖01‖DevId‖payload)[0:16]` (pointycastle). Guarda `S1`. |
| Resposta | `S2` na chegada; tag inválida → `ServerResponseRejected`; `cS`/`N_S` desconhecidos, usados ou com mais de 60 s → `ServerChallengeRejected`; `TAIL > HEAD + 1` ou `HEAD` menor que o salvo → `ServerResponseRejected(inconsistent)`; **regra do TAIL**: `TAIL > ackSeq + 1` → lacuna OVERFLOW `[ackSeq+1, TAIL−1]` e `ackSeq = TAIL − 1`; âncora: `S2 − S1 ≤ 2 s` → `flags_a = 0`, senão 1. ACK `(ackSeq, c_conf = cS, flags_a)` com tag; token `bench.<devId>.<cS>` válido por 1 h. |
| Lote | Token válido (senão `ServerUnauthorized`); mais de 32 blocos → `ServerRateLimited(1 s)`; para cada bloco, em ordem: confere `L = tamanho + T` com `T ∈ {0, 32, 64}`; com `T = 32` verifica a tag `K_rec` (inválida → interrompe); aceitação: `seq == ackSeq + 1` (dados → `ackSeq = seq`; LOSS → exige `from = seq`, lacuna LOSS, `ackSeq = to`; BOOT → lacuna BOOT se `to ≥ from`, `ackSeq = seq`) **ou** BOOT com `from == ackSeq + 1` e `to == seq − 1` (lacuna BOOT, `ackSeq = seq`); bloco com `seq ≤ ackSeq` é **ignorado** (já aceito, ou coberto por OVERFLOW) sem interromper; qualquer outro → interrompe o lote. Devolve ACK `(ackSeq, c_conf do token, 0)` com tag. Blocos `T = 0` (perfil `none`) e `T = 64` (perfil `asym`) são aceitos sem verificação e contados como "não verificados". |
| Falhas injetáveis | `BenchFaults` por `--dart-define`: `SYNC_BENCH_DELAY_MS` (latência de cada chamada), `SYNC_BENCH_FAIL_503_EVERY`, `SYNC_BENCH_FAIL_401_EVERY`, `SYNC_BENCH_FAIL_429_EVERY` (a cada N chamadas). |
| Estatísticas | Blocos aceitos/rejeitados/não verificados, lacunas, último ACK — exibidas na seção "Depuração" da tela da spec 08. |

`BenchSyncServerGateway` (`infra/sync/bench/bench_sync_server_gateway.dart`):
`mode = 'bench'`; envolve o `BenchSyncServer` aplicando o atraso
configurado e as falhas injetadas.

**[Decisão] Servidor de bancada no app.** Sem ele, nenhum ACK chegaria ao
dongle na Fase 1: a flash nunca seria liberada e a Fig. 15 não poderia ser
demonstrada. Ele também serve de **oráculo** do contrato para a Fase 3: os
mesmos vetores e cenários devem passar no servidor Go.

### 8. Seleção da implementação

```dart
// lib/src/ui/sync/sync_providers.dart
final Provider<SyncServerGateway> syncServerGatewayProvider = Provider((ref) {
  const mode = String.fromEnvironment('SYNC_SERVER', defaultValue: kDebugMode ? 'bench' : 'stub');
  if (kReleaseMode) return const UnavailableSyncServerGateway();
  return switch (mode) {
    'bench' => BenchSyncServerGateway(BenchSyncServer(keys: BenchKeys.fromEnvironment()), faults: BenchFaults.fromEnvironment()),
    _       => const UnavailableSyncServerGateway(),
  };
});
```

**[Decisão]** Padrão `bench` em *debug* e `stub` em *release*; *release*
nunca usa o de bancada (chaves de desenvolvimento). A Fase 3 acrescenta
`'https'` e o torna padrão.

### 9. O que a Fase 3 precisa entregar no app (contrato)

1. `lib/src/infra/sync/https_sync_server_gateway.dart` implementando o port
   do §2 com `dio`, mapeamento da tabela de exceções e regras do adapter.
2. Configuração da URL base (`--dart-define=SYNC_API_BASE=https://…`) e,
   se desejado, *pinning* de certificado.
3. `syncServerGatewayProvider` com `'https'` como padrão.
4. Testes de contrato: os mesmos cenários do `bench_sync_server_test.dart`
   rodados contra um servidor Go local (docker compose da Fase 3).
5. Decidir o uso de `suggestedLatency` (spec 08, Riscos).

Nada mais no app muda: coordenador, buffer, BLE e UI ficam como estão.

## Plano arquivo por arquivo

| Ação | Arquivo | Conteúdo |
|---|---|---|
| Criar | `lib/src/services/sync/sync_server_gateway.dart`, `sync_server_exceptions.dart` | §2. |
| Criar | `lib/src/core/encoding/base64_bytes.dart` | §3. |
| Criar | `lib/src/data/datasources/sync_server_datasource.dart` | §4. |
| Criar | `lib/src/domain/repositories/sync_server_repository.dart`, `lib/src/data/repositories/sync_server_repository_impl.dart` | §4. |
| Alterar | `lib/src/core/errors/sync_failure.dart` | Falhas do §4. |
| Criar | `lib/src/application/sync/handshake_runner.dart` | §5. |
| Criar | `lib/src/infra/sync/unavailable_sync_server_gateway.dart` | §6. |
| Criar | `lib/src/infra/sync/bench/{bench_sync_server,bench_sync_server_gateway,bench_keys,bench_faults,bench_crypto}.dart` | §7 (`bench_crypto`: HMAC-SHA256 com pointycastle e tags truncadas). |
| Alterar | `lib/src/ui/sync/sync_providers.dart` | §8 + `syncServerDatasourceProvider`, `syncServerRepositoryProvider`, `handshakeRunnerProvider`. |
| Alterar | `lib/src/ui/settings/view/sync_settings_screen.dart` | Seção "Depuração": estatísticas e "esquecer estado" do servidor de bancada. |
| Criar | Testes abaixo | — |

## Testes (`fvm flutter test`)

| Arquivo | Casos |
|---|---|
| `test/core/encoding/base64_bytes_test.dart` | Ida e volta; rejeita URL-safe e sem *padding*. |
| `test/infra/sync/bench/bench_crypto_test.dart` | Tags dos vetores da spec 01 (desafio, resposta, ACK, blocos BOOT/LOSS) com as chaves fixas. |
| `test/infra/sync/bench/bench_sync_server_test.dart` | Desafio com `cS` crescente e baseado no relógio; resposta válida → ACK e token; tag errada → 422; desafio reapresentado → 409; regra do TAIL (OVERFLOW) com `ackSeq` avançando; âncora > 2 s → `flags_a = 1`; lote em ordem; LOSS (`from = seq`) avança para `to`; BOOT com `from = ack + 1` e `to = seq − 1` aceito; BOOT com faixa vazia; bloco fora de ordem interrompe e devolve o ACK parcial; tag `K_rec` errada interrompe; 33 blocos → 429; token expirado → 401. |
| `test/data/repositories/sync_server_repository_impl_test.dart` | Token guardado e `needsHandshake` (nulo, a 6 min, a 4 min de expirar); cada exceção do port vira o `PostResult`/`Failure` certo; datasource rejeita desafio com tipo/tamanho errado. |
| `test/application/sync/handshake_runner_test.dart` (`FakeDongle` + `BenchSyncServerGateway`) | Fluxo feliz com o dongle falso usando `HmacHandshakeAuth` equivalente em Dart (as tags conferem); dongle responde 0x02 (c_S antigo) → `HandshakeRejectedByDongle`; resposta com `cS` diferente é ignorada até o *timeout*; queda do link no passo 3; ACK rejeitado com 0x03 no passo 6. |

## Passos de implementação

1. Port, exceções, base64, datasource e repository + testes. *Checkpoint:* testes de repository verdes.
2. `BenchSyncServer` + `bench_crypto` + testes com os vetores. *Checkpoint:* `bench_sync_server_test` verde.
3. `HandshakeRunner` + testes. *Checkpoint:* verde.
4. Stub e seleção por `--dart-define`; integração com o coordenador (spec 07). *Checkpoint:* `fvm flutter run --dart-define=SYNC_SERVER=bench` contra o dongle: handshake ok e flash liberada (roteiro do README); com `SYNC_SERVER=stub`, app em "handshake pendente" e nenhum lote pedido.

## Critérios de aceite

- Port do §2 é o único ponto que a Fase 3 precisa implementar; nenhuma outra
  camada conhece HTTP, JSON ou base64.
- App aceita Desafio 0x01 e ACK 0x04 vindos do servidor como bytes (ou
  base64) e segue o fluxo BLE; repassa a Resposta 0x02 e os blocos 0x03
  exatamente como vieram do dongle.
- Erros 401/403/404/409/422/429/5xx têm destino definido (tabela do §2 +
  reações do coordenador, spec 07).
- Com `SYNC_SERVER=bench`, handshake e leitura contínua funcionam ponta a
  ponta com o dongle real; com `stub`, o app degrada como "sem conexão" (Fig. 14).

## Riscos e decisões em aberto

| Risco / ponto | Impacto | Recomendação |
|---|---|---|
| Chaves de desenvolvimento dentro do app de *debug* | Vazamento sem impacto (só valem para placas de bancada) | Nunca no *release* (`kReleaseMode` força o stub). |
| Servidor de bancada diverge do servidor Go | Falsa sensação de conformidade | Os vetores e cenários do `bench_sync_server_test` viram teste de contrato da Fase 3 (§9). |
| **[Aberto]** Verificação ECDSA (`asym`) no servidor de bancada | Perfil `asym` só tem tag conferida no host (`verify_block.py`, spec 01) | Opcional: `ECDSASigner` do pointycastle com a chave pública do `bench_keys.json`; não bloqueia a fase. |
| Relógio do celular para `c_S` | Celular com hora atrasada gera `c_S` menor que o último aceito → erro 0x02 | O servidor de bancada usa `max(último emitido + 1, época)`; após um 0x02, avança `lastIssuedCs` para `época + 3600` e tenta de novo. |
| **[Aberto]** `GET /confirmacao` | Doc prevê "ao reconectar, antes de enviar" | Na Fase 1 não é chamado (o handshake já traz o ACK); a Fase 3 decide se o coordenador o usa. |
