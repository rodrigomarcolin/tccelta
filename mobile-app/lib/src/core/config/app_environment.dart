import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/core/errors/sync_failure.dart';

/// Ambiente de execução (escolhido por `--dart-define-from-file=env/*.json`).
enum Flavor {
  /// Desenvolvimento (aceita HTTP puro para o emulador).
  dev,

  /// Ensaio com a stack `prodlike`.
  prodlike,

  /// Produção.
  producao,
}

/// Configuração de ambiente do app: flavor e URL base da API.
@immutable
class AppEnvironment {
  /// Cria o ambiente.
  const AppEnvironment({required this.flavor, required this.apiBaseUrl});

  /// Lê `FLAVOR` e `API_BASE_URL` dos `--dart-define`.
  factory AppEnvironment.fromDefines() => AppEnvironment.parse(
    flavor: const String.fromEnvironment('FLAVOR', defaultValue: 'dev'),
    apiBaseUrl: const String.fromEnvironment(
      'API_BASE_URL',
      defaultValue: 'https://10.0.2.2:8443',
    ),
  );

  /// Valida e cria o ambiente a partir de textos. Esquema `https`, ou `http`
  /// só no flavor [Flavor.dev]; senão lança [SyncLinkFailure].
  factory AppEnvironment.parse({
    required String flavor,
    required String apiBaseUrl,
  }) {
    final f = Flavor.values.asNameMap()[flavor];
    if (f == null) throw SyncLinkFailure('FLAVOR inválido: $flavor');
    final uri = Uri.tryParse(apiBaseUrl);
    if (uri == null || !uri.hasAuthority) {
      throw SyncLinkFailure('API_BASE_URL inválida: $apiBaseUrl');
    }
    final env = AppEnvironment(flavor: f, apiBaseUrl: uri);
    final ok =
        uri.scheme == 'https' ||
        (uri.scheme == 'http' && env.permiteHttpSemTls);
    if (!ok) {
      throw SyncLinkFailure('API_BASE_URL deve usar https: $apiBaseUrl');
    }
    return env;
  }

  /// Flavor atual.
  final Flavor flavor;

  /// URL base da API.
  final Uri apiBaseUrl;

  /// HTTP sem TLS só no flavor dev (emulador apontando para a porta 8080).
  bool get permiteHttpSemTls => flavor == Flavor.dev;
}

/// Ambiente do app.
final Provider<AppEnvironment> appEnvironmentProvider =
    Provider<AppEnvironment>((ref) => AppEnvironment.fromDefines());
