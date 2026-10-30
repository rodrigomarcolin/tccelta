import 'dart:typed_data';

import 'package:dio/io.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/config/app_environment.dart';
import 'package:tccelta_mobile/src/core/network/dio_client.dart';

void main() {
  final env = AppEnvironment.parse(
    flavor: 'dev',
    apiBaseUrl: 'https://192.168.0.10:8443',
  );

  IOHttpClientAdapter adapterOf(Uint8List? ca) =>
      createSyncDio(env, trustedCa: ca).httpClientAdapter
          as IOHttpClientAdapter;

  test('sem CA extra o cliente usa só os certificados do sistema', () {
    expect(adapterOf(null).createHttpClient, isNull);
  });

  test('com CA extra o cliente cria um contexto próprio', () {
    expect(
      adapterOf(Uint8List.fromList([1, 2, 3])).createHttpClient,
      isNotNull,
    );
  });
}
