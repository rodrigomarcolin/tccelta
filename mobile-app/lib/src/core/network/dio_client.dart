import 'package:dio/dio.dart';
import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/core/config/app_environment.dart';

/// Cria o [Dio] do servidor de sincronização. Sem interceptor de *retry*: as
/// retentativas dependem da semântica (409 pede novo desafio, 429 reduz K, 401
/// refaz o handshake) e ficam no `SyncCoordinator`.
Dio createSyncDio(AppEnvironment env) {
  final dio = Dio(
    BaseOptions(
      baseUrl: env.apiBaseUrl.toString(),
      connectTimeout: const Duration(seconds: 10),
      sendTimeout: const Duration(seconds: 30),
      receiveTimeout: const Duration(seconds: 30),
      // O status é tratado pelo cliente: o 422 do lote traz o ACK no corpo.
      validateStatus: (s) => s != null && s >= 200 && s < 600,
      responseType: ResponseType.bytes,
    ),
  );
  if (kDebugMode) dio.interceptors.add(SecretFreeLogInterceptor());
  return dio;
}

/// Loga método, caminho, status e duração. Nunca o `Authorization` nem o
/// corpo das respostas (o handshake devolve o token).
class SecretFreeLogInterceptor extends Interceptor {
  final Map<RequestOptions, Stopwatch> _timers = {};

  @override
  void onRequest(RequestOptions options, RequestInterceptorHandler handler) {
    _timers[options] = Stopwatch()..start();
    handler.next(options);
  }

  @override
  void onResponse(
    Response<dynamic> response,
    ResponseInterceptorHandler handler,
  ) {
    final t = _timers.remove(response.requestOptions)?.elapsedMilliseconds;
    debugPrint(
      '[sync-http] ${response.requestOptions.method} '
      '${response.requestOptions.path} '
      '-> ${response.statusCode} (${t ?? '?'} ms)',
    );
    handler.next(response);
  }

  @override
  void onError(DioException err, ErrorInterceptorHandler handler) {
    _timers.remove(err.requestOptions);
    debugPrint(
      '[sync-http] ${err.requestOptions.method} ${err.requestOptions.path} '
      '-> erro ${err.type.name}',
    );
    handler.next(err);
  }
}

/// [Dio] do servidor de sincronização.
final Provider<Dio> syncDioProvider = Provider<Dio>(
  (ref) => createSyncDio(ref.watch(appEnvironmentProvider)),
);
