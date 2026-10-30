import 'dart:async';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:tccelta_mobile/src/core/encoding/base64_bytes.dart';
import 'package:tccelta_mobile/src/domain/sync/dev_id.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';
import 'package:tccelta_mobile/src/infra/sync/sync_api_client.dart';
import 'package:tccelta_mobile/src/services/sync/sync_server_exceptions.dart';
import 'package:tccelta_mobile/src/services/sync/sync_server_gateway.dart';

/// Gateway HTTPS: implementa o port da Fase 1 sobre a API Go (Fase 3). Não
/// guarda sessão: o token vive no `SyncServerRepository`. Os bytes dos TLVs
/// passam sem alteração (base64 só nos JSON do handshake).
class HttpsSyncServerGateway implements SyncServerGateway {
  /// Cria o gateway sobre o [_api]. [clock] devolve o instante atual (UTC).
  HttpsSyncServerGateway(this._api, {DateTime Function()? clock})
    : _clock = clock ?? DateTime.now;

  final SyncApiClient _api;
  final DateTime Function() _clock;

  @override
  String get mode => 'https';

  @override
  Future<ChallengeEnvelope> requestChallenge(DevId devId) => _guard(() async {
    final dto = await _api.requestChallenge(devId.hex);
    final frame = _parse(
      Base64Bytes.decode(dto.desafio),
      SyncMessageType.challenge,
    );
    final len = frame.value.length;
    if (len != 45 && len != 47) throw const ServerMalformed('desafio_tamanho');
    return ChallengeEnvelope(challenge: frame, expiresAt: dto.expiraEm);
  });

  @override
  Future<HandshakeEnvelope> submitResponse(DevId devId, SyncFrame response) =>
      _guard(() async {
        final dto = await _api.submitResponse(
          devId.hex,
          Base64Bytes.encode(response.toTlv()),
        );
        final ack = _parse(Base64Bytes.decode(dto.ack), SyncMessageType.ack);
        // A validade é contada pelo relógio local a partir do recebimento
        // (sem depender do relógio de parede do servidor).
        return HandshakeEnvelope(
          ack: ack,
          token: dto.token,
          tokenExpiresAt: _clock().toUtc().add(
            Duration(seconds: dto.tokenValidadeS),
          ),
          suggestedLatency: dto.tLatS == null
              ? null
              : Duration(seconds: dto.tLatS!),
        );
      });

  @override
  Future<SyncFrame> postBlocks(String token, List<SyncFrame> blocks) =>
      _guard(() async {
        final out = BytesBuilder(copy: false);
        for (final b in blocks) {
          out.add(b.toTlv());
        }
        final r = await _api.postBlocks(out.takeBytes(), token);
        // "O corpo é um ACK se e só se o Content-Type é octet-stream": o 422
        // `lote_sem_progresso` traz o ACK; os demais 422 são problem+json.
        if (r.isOctetStream && (r.status == 200 || r.status == 422)) {
          final ack = _parse(r.body, SyncMessageType.ack);
          if (r.status == 422) throw ServerBatchNoProgress(ack);
          return ack;
        }
        throw SyncApiClient.toException(r);
      });

  @override
  Future<SyncFrame?> latestConfirmation(DevId devId, String token) =>
      _guard(() async {
        final r = await _api.latestConfirmation(devId.hex, token);
        if (r.status == 404) return null;
        SyncApiClient.ensureOk(r);
        return _parse(r.body, SyncMessageType.ack);
      });

  SyncFrame _parse(Uint8List bytes, SyncMessageType expected) {
    final f = SyncFrame.parseTlv(bytes);
    if (f.type != expected) throw const ServerMalformed('tipo_inesperado');
    if (expected == SyncMessageType.ack && f.value.length != 25) {
      throw const ServerMalformed('ack_tamanho');
    }
    return f;
  }

  /// Traduz erros crus em [SyncServerException] pelo `codigo`.
  Future<T> _guard<T>(Future<T> Function() body) async {
    try {
      return await body();
    } on SyncServerException {
      rethrow;
    } on SyncApiException catch (e) {
      throw _map(e);
    } on DioException {
      throw const ServerUnavailable();
    } on FormatException {
      throw const ServerMalformed('resposta_fora_do_contrato');
    } on TimeoutException {
      throw const ServerUnavailable();
    }
  }

  SyncServerException _map(SyncApiException e) {
    final c = e.code ?? '';
    switch (e.status) {
      case 401:
        return ServerUnauthorized(c.isEmpty ? 'token_invalido' : c);
      case 403:
        return c == 'token_outro_dongle'
            ? const ServerForbiddenOtherDongle()
            : ServerDongleBlocked(c.isEmpty ? 'dongle_inativo' : c);
      case 404:
        return c == 'sem_confirmacao'
            ? const ServerNoConfirmation()
            : ServerDongleBlocked(c.isEmpty ? 'dongle_desconhecido' : c);
      case 409:
        return ServerChallengeRejected(c.isEmpty ? 'desafio_invalido' : c);
      case 422:
        return ServerResponseRejected(
          inconsistent: c == 'resposta_inconsistente',
        );
      case 429:
        return ServerRateLimited(e.retryAfter ?? const Duration(seconds: 1));
      case 400 || 413 || 415:
        return ServerMalformed(c.isEmpty ? 'requisicao_invalida' : c);
      default:
        if (e.status >= 500) return ServerUnavailable(retryAfter: e.retryAfter);
        return ServerMalformed(c.isEmpty ? 'status_${e.status}' : c);
    }
  }
}
