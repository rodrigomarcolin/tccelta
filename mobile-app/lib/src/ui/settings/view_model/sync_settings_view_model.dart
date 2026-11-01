import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/domain/location/gps_fix.dart';
import 'package:tccelta_mobile/src/domain/location/gps_source.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_settings.dart';
import 'package:tccelta_mobile/src/ui/settings/settings_providers.dart';

/// Estado da tela de configurações de sincronização.
@immutable
class SyncSettingsState {
  /// Cria o estado.
  const SyncSettingsState({
    required this.settings,
    this.gpsSource = GpsSource.real,
    this.gpsPermission,
    this.saving = false,
  });

  /// Configurações vigentes.
  final SyncSettings settings;

  /// Origem das posições em uso (pode diferir da gravada fora da depuração).
  final GpsSource gpsSource;

  /// Permissão de localização (`null` até a primeira consulta).
  final LocationPermissionState? gpsPermission;

  /// Gravando uma mudança.
  final bool saving;

  /// Cópia com campos trocados.
  SyncSettingsState copyWith({
    SyncSettings? settings,
    GpsSource? gpsSource,
    LocationPermissionState? gpsPermission,
    bool? saving,
  }) => SyncSettingsState(
    settings: settings ?? this.settings,
    gpsSource: gpsSource ?? this.gpsSource,
    gpsPermission: gpsPermission ?? this.gpsPermission,
    saving: saving ?? this.saving,
  );
}

/// Lê e grava as configurações de sincronização e consulta/pede a permissão
/// de localização da origem de posições em uso.
class SyncSettingsViewModel extends Notifier<SyncSettingsState> {
  @override
  SyncSettingsState build() {
    final repo = ref.read(syncSettingsRepositoryProvider);
    final sub = repo.watch().listen((s) => state = state.copyWith(settings: s));
    ref
      ..onDispose(() => unawaited(sub.cancel()))
      ..listen(effectiveGpsSourceProvider, (_, next) {
        state = state.copyWith(gpsSource: next);
        unawaited(refreshPermission());
      });
    unawaited(Future.microtask(refreshPermission));
    return SyncSettingsState(
      settings: repo.current,
      gpsSource: ref.read(effectiveGpsSourceProvider),
    );
  }

  /// `T_lat`, em segundos.
  Future<void> setLatency(int seconds) =>
      _save((s) => s.copyWith(latency: Duration(seconds: seconds)));

  /// `B_max`, em blocos.
  Future<void> setBufferCapacity(int blocks) =>
      _save((s) => s.copyWith(bufferCapacity: blocks));

  /// `T_GPS`, em segundos.
  Future<void> setGpsPeriod(int seconds) =>
      _save((s) => s.copyWith(gpsPeriod: Duration(seconds: seconds)));

  /// Liga ou desliga o envio de GPS.
  Future<void> setGpsEnabled({required bool enabled}) =>
      _save((s) => s.copyWith(gpsEnabled: enabled));

  /// Grava a origem das posições.
  Future<void> setGpsSource(GpsSource source) =>
      _save((s) => s.copyWith(gpsSource: source));

  /// Consulta a permissão atual, sem diálogo.
  Future<void> refreshPermission() async {
    try {
      final p = await ref.read(locationRepositoryProvider).permission();
      state = state.copyWith(gpsPermission: p);
    } on Object {
      // Sem plugin ou sem serviço: a tela segue com a permissão desconhecida.
    }
  }

  /// Pede a permissão. Com `grantedWhileInUse`, a nova chamada pede também o
  /// acesso em segundo plano.
  Future<void> requestGpsPermission() async {
    try {
      final p = await ref.read(locationRepositoryProvider).requestPermission();
      state = state.copyWith(gpsPermission: p);
    } on Object {
      await refreshPermission();
    }
  }

  Future<void> _save(SyncSettings Function(SyncSettings) change) async {
    state = state.copyWith(saving: true);
    try {
      // Parte do valor do repository, não do estado: o fluxo `watch` chega
      // depois e duas mudanças seguidas perderiam a primeira.
      final repo = ref.read(syncSettingsRepositoryProvider);
      await repo.save(change(repo.current));
    } finally {
      state = state.copyWith(saving: false);
    }
  }
}
