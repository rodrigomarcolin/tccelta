import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/application/sync/handshake_runner.dart';
import 'package:tccelta_mobile/src/application/sync/sync_coordinator.dart';
import 'package:tccelta_mobile/src/data/datasources/sync_server_datasource.dart';
import 'package:tccelta_mobile/src/data/repositories/dongle_sync_repository_impl.dart';
import 'package:tccelta_mobile/src/data/repositories/sync_server_repository_impl.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_sync_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/sync_server_repository.dart';
import 'package:tccelta_mobile/src/infra/sync/bench/bench_faults.dart';
import 'package:tccelta_mobile/src/infra/sync/bench/bench_sync_server.dart';
import 'package:tccelta_mobile/src/infra/sync/bench/bench_sync_server_gateway.dart';
import 'package:tccelta_mobile/src/infra/sync/unavailable_sync_server_gateway.dart';
import 'package:tccelta_mobile/src/services/sync/sync_server_gateway.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/settings/settings_providers.dart';

/// Servidor escolhido no build: `--dart-define=SYNC_SERVER=bench` usa o
/// servidor de bancada embutido (só em debug); qualquer outro valor usa o
/// gateway indisponível até a Fase 3 entregar o HTTPS.
const String kSyncServerMode = String.fromEnvironment(
  'SYNC_SERVER',
  defaultValue: 'stub',
);

/// Servidor de bancada embutido (estado em memória), usado só no modo `bench`.
final Provider<BenchSyncServer> benchSyncServerProvider =
    Provider<BenchSyncServer>((ref) => BenchSyncServer());

/// Gateway do servidor conforme [kSyncServerMode].
final Provider<SyncServerGateway> syncServerGatewayProvider =
    Provider<SyncServerGateway>((ref) {
      if (kDebugMode && kSyncServerMode == 'bench') {
        return BenchSyncServerGateway(
          ref.read(benchSyncServerProvider),
          faults: BenchFaults.fromEnvironment(),
        );
      }
      return const UnavailableSyncServerGateway();
    });

/// Datasource do servidor.
final Provider<SyncServerDatasource> syncServerDatasourceProvider =
    Provider<SyncServerDatasource>(
      (ref) => SyncServerDatasource(ref.read(syncServerGatewayProvider)),
    );

/// Repository do servidor (guarda o token só em memória).
final Provider<SyncServerRepository> syncServerRepositoryProvider =
    Provider<SyncServerRepository>(
      (ref) => SyncServerRepositoryImpl(ref.read(syncServerDatasourceProvider)),
    );

/// Canal SYNC com o dongle, sobre o [dongleRepositoryProvider]. Não é
/// autoDispose: acompanha a sessão BLE inteira.
final Provider<DongleSyncRepository> dongleSyncRepositoryProvider =
    Provider<DongleSyncRepository>((ref) {
      final repo = DongleSyncRepositoryImpl(ref.read(dongleRepositoryProvider));
      ref.onDispose(repo.dispose);
      return repo;
    });

/// Executor do handshake app ↔ dongle ↔ servidor.
final Provider<HandshakeRunner> handshakeRunnerProvider =
    Provider<HandshakeRunner>(
      (ref) => HandshakeRunner(
        dongle: ref.read(dongleSyncRepositoryProvider),
        server: ref.read(syncServerRepositoryProvider),
      ),
    );

/// Coordenador da leitura contínua. Instanciado pelo `ConnectionGuard`
/// (global).
final Provider<SyncCoordinator> syncCoordinatorProvider =
    Provider<SyncCoordinator>((ref) {
      final coordinator = SyncCoordinator(
        dongle: ref.read(dongleSyncRepositoryProvider),
        server: ref.read(syncServerRepositoryProvider),
        settings: ref.read(syncSettingsRepositoryProvider),
        handshake: ref.read(handshakeRunnerProvider),
      )..start();
      ref.onDispose(coordinator.dispose);
      return coordinator;
    });
