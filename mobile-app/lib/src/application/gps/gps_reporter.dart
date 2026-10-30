// Parâmetros nomeados não podem ser formais privados: a lista de inicialização
// é necessária.
// As assinaturas são canceladas em dispose/_stopStreaming.
// ignore_for_file: prefer_initializing_formals, cancel_subscriptions

import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:tccelta_mobile/src/core/time/monotonic_clock.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/location/gps_fix.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/location_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/sync_settings_repository.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_settings.dart';

/// Precisão horizontal máxima aceita para enviar uma posição, em metros.
const double kGpsMaxAccuracyM = 100;

/// Fase do envio periódico de GPS ao dongle.
enum GpsReportPhase {
  /// Sem conexão pronta, GPS desligado ou aguardando configuração.
  stopped,

  /// Sem permissão de localização (o reporter nunca abre o diálogo).
  noPermission,

  /// Ligado, esperando uma posição recente e precisa.
  awaitingFix,

  /// Enviando posições ao dongle.
  sending,

  /// Último envio falhou; tenta de novo no próximo período.
  error,
}

/// Estado do [GpsReporter].
@immutable
class GpsReportStatus {
  /// Cria o estado.
  const GpsReportStatus(this.phase, {this.lastSentAt, this.message});

  /// Fase atual.
  final GpsReportPhase phase;

  /// Instante (monotônico) do último envio bem-sucedido.
  final Duration? lastSentAt;

  /// Mensagem da última falha, se houver.
  final String? message;
}

/// Envia a posição do celular ao dongle a cada `T_GPS` enquanto a conexão está
/// pronta, o GPS ligado e há permissão. Posição velha ou imprecisa não é
/// enviada: melhor ausência que posição errada.
class GpsReporter {
  /// Cria o reporter.
  GpsReporter({
    required DongleRepository dongle,
    required LocationRepository location,
    required Obd2Repository obd2,
    required SyncSettingsRepository settings,
    MonotonicClock? clock,
  }) : _dongle = dongle,
       _location = location,
       _obd2 = obd2,
       _settings = settings,
       _clock = clock ?? SystemMonotonicClock();

  final DongleRepository _dongle;
  final LocationRepository _location;
  final Obd2Repository _obd2;
  final SyncSettingsRepository _settings;
  final MonotonicClock _clock;

  final StreamController<GpsReportStatus> _statusCtrl =
      StreamController<GpsReportStatus>.broadcast();
  StreamSubscription<BleConnectionPhase>? _phaseSub;
  StreamSubscription<SyncSettings>? _settingsSub;
  StreamSubscription<GpsFix>? _fixSub;
  Timer? _timer;
  GpsFix? _latest;
  bool _busy = false;
  bool _disposed = false;
  int _epoch = 0;
  SyncSettings? _active;
  GpsReportStatus _status = const GpsReportStatus(GpsReportPhase.stopped);

  /// Fluxo de estados.
  Stream<GpsReportStatus> get status => _statusCtrl.stream;

  /// Último estado.
  GpsReportStatus get current => _status;

  /// Começa a observar conexão e configurações.
  void start() {
    _phaseSub = _dongle.connectionPhase.listen(
      (_) => unawaited(_reevaluate()),
      onError: (_) => unawaited(_reevaluate()),
    );
    _settingsSub = _settings.watch().listen((s) {
      final a = _active;
      if (a == null ||
          a.gpsEnabled != s.gpsEnabled ||
          a.gpsPeriod != s.gpsPeriod) {
        unawaited(_reevaluate());
      }
    });
    unawaited(_reevaluate());
  }

  /// Encerra tudo.
  Future<void> dispose() async {
    _disposed = true;
    _epoch++;
    await _phaseSub?.cancel();
    await _settingsSub?.cancel();
    _stopStreaming();
    await _statusCtrl.close();
  }

  void _emit(GpsReportStatus s) {
    _status = s;
    if (!_statusCtrl.isClosed) _statusCtrl.add(s);
  }

  void _stopStreaming() {
    _timer?.cancel();
    _timer = null;
    final sub = _fixSub;
    _fixSub = null;
    _latest = null;
    unawaited(sub?.cancel());
  }

  Future<void> _reevaluate() async {
    final epoch = ++_epoch;
    _stopStreaming();
    final settings = _settings.current;
    _active = settings;
    if (_disposed || !_dongle.isReady || !settings.gpsEnabled) {
      _emit(const GpsReportStatus(GpsReportPhase.stopped));
      return;
    }
    final LocationPermissionState permission;
    try {
      permission = await _location.permission();
    } on Object catch (e) {
      if (epoch == _epoch) {
        _emit(GpsReportStatus(GpsReportPhase.error, message: '$e'));
      }
      return;
    }
    if (epoch != _epoch) return; // reavaliado no meio: este resultado é velho
    if (permission != LocationPermissionState.granted &&
        permission != LocationPermissionState.grantedWhileInUse) {
      _emit(const GpsReportStatus(GpsReportPhase.noPermission));
      return;
    }
    final period = settings.gpsPeriod;
    _fixSub = _location
        .fixes(
          interval: period,
          background: permission == LocationPermissionState.granted,
        )
        .listen(
          (fix) => _latest = fix,
          onError: (Object e) =>
              _emit(GpsReportStatus(GpsReportPhase.error, message: '$e')),
        );
    _timer = Timer.periodic(period, (_) => unawaited(_tick(period)));
    _emit(const GpsReportStatus(GpsReportPhase.awaitingFix));
  }

  Future<void> _tick(Duration period) async {
    final fix = _latest;
    if (_busy || fix == null) return;
    final age = _clock.now() - fix.at;
    if (age > period * 2 || fix.accuracyM > kGpsMaxAccuracyM) return;
    _busy = true;
    try {
      await _obd2.reportGps(fix);
      _emit(GpsReportStatus(GpsReportPhase.sending, lastSentAt: _clock.now()));
    } on Object catch (e) {
      _emit(
        GpsReportStatus(
          GpsReportPhase.error,
          lastSentAt: _status.lastSentAt,
          message: '$e',
        ),
      );
    } finally {
      _busy = false;
    }
  }
}
