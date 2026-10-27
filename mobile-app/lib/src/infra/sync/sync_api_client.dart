import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:meta/meta.dart';

/// Resposta binária crua: status, corpo e headers (chaves em minúsculas).
@immutable
class BinaryResponse {
  /// Cria a resposta.
  const BinaryResponse(this.status, this.body, this.headers);

  /// Status HTTP.
  final int status;

  /// Corpo cru.
  final Uint8List body;

  /// Headers em minúsculas.
  final Map<String, String> headers;

  /// `true` se o corpo é um TLV (a regra é o `Content-Type`, nunca o status).
  bool get isOctetStream =>
      (headers['content-type'] ?? '').startsWith('application/octet-stream');
}

/// Erro HTTP já lido: status, `codigo` do `problem+json` e `Retry-After`.
class SyncApiException implements Exception {
  /// Cria a exceção.
  const SyncApiException(this.status, {this.code, this.retryAfter});

  /// Status HTTP.
  final int status;

  /// `codigo` do `application/problem+json`.
  final String? code;

  /// `Retry-After` em segundos inteiros.
  final Duration? retryAfter;

  @override
  String toString() => 'SyncApiException($status, $code)';
}

/// Cliente HTTP do contrato da API (spec 03/04/05): só faz HTTP, sem domínio,
/// sem token guardado e sem retentativas.
class SyncApiClient {
  /// Cria o cliente sobre o [_dio] (resposta em bytes).
  const SyncApiClient(this._dio);

  final Dio _dio;

  /// `POST /v1/dongles/{id}/desafio` (corpo vazio). Devolve `desafio` (base64)
  /// e `expira_em`.
  Future<({String desafio, DateTime? expiraEm})> requestChallenge(
    String devIdHex,
  ) async {
    final r = await _send(
      'POST',
      '/v1/dongles/$devIdHex/desafio',
      accept: 'application/json',
    );
    _ensureOk(r);
    final j = _json(r);
    return (
      desafio: j['desafio'] as String,
      expiraEm: DateTime.tryParse((j['expira_em'] as String?) ?? ''),
    );
  }

  /// `POST /v1/dongles/{id}/desafio/resposta` com `{"resposta": b64}`.
  Future<({String ack, String token, int tokenValidadeS, int? tLatS})>
  submitResponse(String devIdHex, String responseB64) async {
    final r = await _send(
      'POST',
      '/v1/dongles/$devIdHex/desafio/resposta',
      accept: 'application/json',
      contentType: 'application/json',
      data: Uint8List.fromList(
        utf8.encode(jsonEncode({'resposta': responseB64})),
      ),
    );
    _ensureOk(r);
    final j = _json(r);
    return (
      ack: j['ack'] as String,
      token: j['token'] as String,
      tokenValidadeS: (j['token_validade_s'] as num).toInt(),
      tLatS: (j['t_lat_s'] as num?)?.toInt(),
    );
  }

  /// `POST /v1/dongles/blocos`: `octet-stream` = concatenação exata dos TLVs.
  /// Devolve a resposta crua (200 e 422 `lote_sem_progresso` trazem o ACK).
  Future<BinaryResponse> postBlocks(Uint8List body, String token) => _send(
    'POST',
    '/v1/dongles/blocos',
    accept: 'application/octet-stream',
    contentType: 'application/octet-stream',
    data: body,
    token: token,
  );

  /// `GET /v1/dongles/{id}/confirmacao`.
  Future<BinaryResponse> latestConfirmation(String devIdHex, String token) =>
      _send(
        'GET',
        '/v1/dongles/$devIdHex/confirmacao',
        accept: 'application/octet-stream',
        token: token,
      );

  /// Lança [SyncApiException] se o status não for 2xx.
  static void ensureOk(BinaryResponse r) => _ensureOk(r);

  /// Converte uma resposta de erro em [SyncApiException] (lê o `problem+json`).
  static SyncApiException toException(BinaryResponse r) {
    String? code;
    if ((r.headers['content-type'] ?? '').contains('json')) {
      try {
        code = _json(r)['codigo'] as String?;
      } on Object {
        code = null;
      }
    }
    final seconds = int.tryParse(r.headers['retry-after'] ?? '');
    return SyncApiException(
      r.status,
      code: code,
      retryAfter: seconds == null ? null : Duration(seconds: seconds),
    );
  }

  static void _ensureOk(BinaryResponse r) {
    if (r.status < 200 || r.status >= 300) throw toException(r);
  }

  static Map<String, dynamic> _json(BinaryResponse r) =>
      jsonDecode(utf8.decode(r.body)) as Map<String, dynamic>;

  Future<BinaryResponse> _send(
    String method,
    String path, {
    required String accept,
    String? contentType,
    Uint8List? data,
    String? token,
  }) async {
    final res = await _dio.request<List<int>>(
      path,
      data: data,
      options: Options(
        method: method,
        responseType: ResponseType.bytes,
        contentType: contentType,
        headers: {
          'Accept': accept,
          if (token != null) 'Authorization': 'Bearer $token',
        },
      ),
    );
    final headers = <String, String>{
      for (final e in res.headers.map.entries)
        e.key.toLowerCase(): e.value.join(','),
    };
    return BinaryResponse(
      res.statusCode ?? 0,
      Uint8List.fromList(res.data ?? const <int>[]),
      headers,
    );
  }
}
