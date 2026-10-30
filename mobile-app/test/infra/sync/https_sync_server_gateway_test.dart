import 'dart:convert';
import 'dart:typed_data';

import 'package:dio/dio.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/config/app_environment.dart';
import 'package:tccelta_mobile/src/core/encoding/base64_bytes.dart';
import 'package:tccelta_mobile/src/domain/sync/dev_id.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';
import 'package:tccelta_mobile/src/infra/sync/https_sync_server_gateway.dart';
import 'package:tccelta_mobile/src/infra/sync/sync_api_client.dart';
import 'package:tccelta_mobile/src/services/sync/sync_server_exceptions.dart';

class _Req {
  _Req(this.method, this.path, this.headers, this.body);
  final String method;
  final String path;
  final Map<String, dynamic> headers;
  final Uint8List body;
}

class _Reply {
  _Reply(this.status, this.body, this.headers);
  final int status;
  final List<int> body;
  final Map<String, List<String>> headers;
}

class _FakeAdapter implements HttpClientAdapter {
  final List<_Req> requests = [];
  final List<_Reply> replies = [];

  @override
  Future<ResponseBody> fetch(
    RequestOptions options,
    Stream<Uint8List>? requestStream,
    Future<void>? cancelFuture,
  ) async {
    final chunks = <int>[];
    if (requestStream != null) {
      await requestStream.forEach(chunks.addAll);
    }
    requests.add(
      _Req(
        options.method,
        options.path,
        options.headers,
        Uint8List.fromList(chunks),
      ),
    );
    final r = replies.removeAt(0);
    return ResponseBody.fromBytes(r.body, r.status, headers: r.headers);
  }

  @override
  void close({bool force = false}) {}
}

_Reply _json(
  int status,
  Map<String, Object?> j, {
  Map<String, List<String>>? h,
}) => _Reply(status, utf8.encode(jsonEncode(j)), {
  'content-type': ['application/problem+json'],
  ...?h,
});

Uint8List _tlv(int t, List<int> v) =>
    Uint8List.fromList([t, v.length & 0xFF, v.length >> 8, ...v]);

final Uint8List _ackTlv = _tlv(0x04, List<int>.generate(25, (i) => i + 1));
final Uint8List _challengeTlv = _tlv(0x01, List<int>.generate(45, (i) => i));
final DevId _dev = DevId.fromHex('0011223344556677');

void main() {
  late _FakeAdapter adapter;
  late HttpsSyncServerGateway gw;

  setUp(() {
    adapter = _FakeAdapter();
    final dio = Dio(
      BaseOptions(
        baseUrl: 'https://api.test',
        validateStatus: (s) => s != null && s < 600,
        responseType: ResponseType.bytes,
      ),
    )..httpClientAdapter = adapter;
    gw = HttpsSyncServerGateway(
      SyncApiClient(dio),
      clock: () => DateTime.utc(2026),
    );
  });

  test('requestChallenge: POST sem corpo e bytes do TLV preservados', () async {
    adapter.replies.add(
      _Reply(
        200,
        utf8.encode(
          jsonEncode({
            'desafio': Base64Bytes.encode(_challengeTlv),
            'expira_em': '2026-10-26T14:30:00.000Z',
          }),
        ),
        {
          'content-type': ['application/json'],
        },
      ),
    );
    final env = await gw.requestChallenge(_dev);
    expect(adapter.requests.single.method, 'POST');
    expect(
      adapter.requests.single.path,
      '/v1/dongles/0011223344556677/desafio',
    );
    expect(env.challenge.toTlv(), _challengeTlv);
    expect(env.expiresAt, isNotNull);
  });

  test('requestChallenge: L diferente de 45/47 é malformado', () async {
    adapter.replies.add(
      _Reply(
        200,
        utf8.encode(
          jsonEncode({
            'desafio': Base64Bytes.encode(_tlv(0x01, List.filled(46, 0))),
          }),
        ),
        {
          'content-type': ['application/json'],
        },
      ),
    );
    expect(gw.requestChallenge(_dev), throwsA(isA<ServerMalformed>()));
  });

  test(
    'submitResponse: JSON com base64, ack e validade pelo relógio local',
    () async {
      final resp = SyncFrame.parseTlv(_tlv(0x02, List.filled(50, 7)));
      adapter.replies.add(
        _Reply(
          200,
          utf8.encode(
            jsonEncode({
              'ack': Base64Bytes.encode(_ackTlv),
              'token': 'tk',
              'token_expira_em': '2030-01-01T00:00:00Z',
              'token_validade_s': 3600,
              't_lat_s': 60,
            }),
          ),
          {
            'content-type': ['application/json'],
          },
        ),
      );
      final env = await gw.submitResponse(_dev, resp);
      final sent =
          jsonDecode(utf8.decode(adapter.requests.single.body))
              as Map<String, dynamic>;
      expect(Base64Bytes.decode(sent['resposta'] as String), resp.toTlv());
      expect(env.ack.toTlv(), _ackTlv);
      expect(env.token, 'tk');
      expect(env.tokenExpiresAt, DateTime.utc(2026, 1, 1, 1));
      expect(env.suggestedLatency, const Duration(seconds: 60));
    },
  );

  test('postBlocks: octet-stream com a concatenação exata e Bearer', () async {
    final b1 = SyncFrame(
      SyncMessageType.block,
      Uint8List.fromList(List.filled(49, 1)),
    );
    final b2 = SyncFrame(
      SyncMessageType.block,
      Uint8List.fromList(List.filled(49, 2)),
    );
    adapter.replies.add(
      _Reply(200, _ackTlv, {
        'content-type': ['application/octet-stream'],
      }),
    );
    final ack = await gw.postBlocks('t', [b1, b2]);
    final r = adapter.requests.single;
    expect(r.body, Uint8List.fromList([...b1.toTlv(), ...b2.toTlv()]));
    expect(r.headers['Authorization'], 'Bearer t');
    expect(ack.toTlv(), _ackTlv);
  });

  test(
    'postBlocks: 422 octet-stream => ServerBatchNoProgress com o ACK',
    () async {
      adapter.replies.add(
        _Reply(422, _ackTlv, {
          'content-type': ['application/octet-stream'],
        }),
      );
      await expectLater(
        gw.postBlocks('t', [SyncFrame(SyncMessageType.block, Uint8List(49))]),
        throwsA(
          isA<ServerBatchNoProgress>().having(
            (e) => e.ack.toTlv(),
            'ack',
            _ackTlv,
          ),
        ),
      );
    },
  );

  test('mapeia status/codigo para as exceções do port', () async {
    Future<Object> falha(_Reply r) async {
      adapter.replies.add(r);
      try {
        await gw.postBlocks('t', [
          SyncFrame(SyncMessageType.block, Uint8List(49)),
        ]);
      } on Object catch (e) {
        return e;
      }
      return 'sem erro';
    }

    expect(
      await falha(_json(401, {'codigo': 'token_expirado'})),
      isA<ServerUnauthorized>(),
    );
    expect(
      await falha(_json(403, {'codigo': 'dongle_inativo'})),
      isA<ServerDongleBlocked>(),
    );
    expect(
      await falha(_json(403, {'codigo': 'token_outro_dongle'})),
      isA<ServerForbiddenOtherDongle>(),
    );
    expect(
      await falha(_json(409, {'codigo': 'desafio_invalido'})),
      isA<ServerChallengeRejected>(),
    );
    expect(
      await falha(_json(422, {'codigo': 'resposta_inconsistente'})),
      isA<ServerResponseRejected>().having(
        (e) => e.inconsistent,
        'inconsistent',
        true,
      ),
    );
    expect(
      await falha(
        _json(
          429,
          {'codigo': 'limite_taxa'},
          h: {
            'retry-after': ['7'],
          },
        ),
      ),
      isA<ServerRateLimited>().having(
        (e) => e.retryAfter,
        'retryAfter',
        const Duration(seconds: 7),
      ),
    );
    expect(
      await falha(
        _json(
          503,
          {'codigo': 'kms_indisponivel'},
          h: {
            'retry-after': ['9'],
          },
        ),
      ),
      isA<ServerUnavailable>().having(
        (e) => e.retryAfter,
        'retryAfter',
        const Duration(seconds: 9),
      ),
    );
    expect(
      await falha(_json(400, {'codigo': 'lote_malformado'})),
      isA<ServerMalformed>(),
    );
  });

  test('latestConfirmation: 404 vira null; 200 devolve o ACK', () async {
    adapter.replies.add(_json(404, {'codigo': 'sem_confirmacao'}));
    expect(await gw.latestConfirmation(_dev, 't'), isNull);
    adapter.replies.add(
      _Reply(200, _ackTlv, {
        'content-type': ['application/octet-stream'],
      }),
    );
    expect((await gw.latestConfirmation(_dev, 't'))!.toTlv(), _ackTlv);
  });

  group('AppEnvironment', () {
    test('https sempre; http só no dev', () {
      expect(
        AppEnvironment.parse(
          flavor: 'dev',
          apiBaseUrl: 'http://10.0.2.2:8080',
        ).permiteHttpSemTls,
        isTrue,
      );
      expect(
        () => AppEnvironment.parse(
          flavor: 'producao',
          apiBaseUrl: 'http://x.com',
        ),
        throwsA(anything),
      );
      expect(
        () => AppEnvironment.parse(flavor: 'dev', apiBaseUrl: 'ftp://x.com'),
        throwsA(anything),
      );
      expect(
        AppEnvironment.parse(
          flavor: 'producao',
          apiBaseUrl: 'https://api.x.com',
        ).apiBaseUrl.host,
        'api.x.com',
      );
    });
  });
}
