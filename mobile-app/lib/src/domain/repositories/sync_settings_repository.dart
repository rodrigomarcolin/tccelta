import 'package:tccelta_mobile/src/domain/sync/sync_settings.dart';

/// Configurações de sincronização e GPS (`T_lat`, `B_max`, `T_GPS`).
abstract interface class SyncSettingsRepository {
  /// Valor atual (já dentro das faixas).
  SyncSettings get current;

  /// Emite o valor atual ao assinar e cada valor salvo depois.
  Stream<SyncSettings> watch();

  /// Ajusta [settings] às faixas, persiste e emite.
  Future<void> save(SyncSettings settings);
}
