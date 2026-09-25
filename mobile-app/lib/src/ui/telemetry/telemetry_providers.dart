import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/application/telemetry/panel_persistence_use_case.dart';
import 'package:tccelta_mobile/src/core/persistence/shared_preferences_provider.dart';
import 'package:tccelta_mobile/src/data/datasources/panel_datasource.dart';
import 'package:tccelta_mobile/src/data/repositories/obd2_repository_impl.dart';
import 'package:tccelta_mobile/src/data/repositories/panel_repository_impl.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/panel_repository.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';

/// Repository = *source of truth* da telemetria OBD-II. Consome o
/// `dongleRepositoryProvider` para obter a conexão BLE viva. Não é autoDispose:
/// o `Elm327Client` interno assina a TX da conexão e deve viver com ela.
final Provider<Obd2Repository> obd2RepositoryProvider =
    Provider<Obd2Repository>((ref) {
      final repo = Obd2RepositoryImpl(ref.read(dongleRepositoryProvider));
      ref.onDispose(repo.dispose);
      return repo;
    });

/// Datasource da persistência local dos painéis, sobre
/// [sharedPreferencesProvider].
final Provider<PanelDatasource> panelDatasourceProvider =
    Provider<PanelDatasource>(
      (ref) => PanelDatasource(ref.read(sharedPreferencesProvider)),
    );

/// Repository = *source of truth* dos painéis salvos. Indiferente a local
/// vs. remoto: trocar de storage local para um backend futuro é só trocar
/// esta wiring, sem tocar em domain/application/ui.
final Provider<PanelRepository> panelRepositoryProvider =
    Provider<PanelRepository>(
      (ref) => PanelRepositoryImpl(ref.read(panelDatasourceProvider)),
    );

/// Use case que carrega/grava os painéis, reconciliando dado obsoleto e
/// garantindo que sempre há ao menos um painel.
final Provider<PanelPersistenceUseCase> panelPersistenceUseCaseProvider =
    Provider<PanelPersistenceUseCase>(
      (ref) => PanelPersistenceUseCase(ref.read(panelRepositoryProvider)),
    );
