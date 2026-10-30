import 'package:meta/meta.dart';

/// Parâmetros editáveis da sincronização e do envio de GPS.
@immutable
class SyncSettings {
  /// Cria as configurações (padrões do documento de segurança).
  const SyncSettings({
    this.latency = const Duration(seconds: 60),
    this.bufferCapacity = 64,
    this.gpsPeriod = const Duration(seconds: 1),
    this.gpsEnabled = true,
  });

  /// Menor `T_lat`.
  static const Duration minLatency = Duration(seconds: 10);

  /// Maior `T_lat`.
  static const Duration maxLatency = Duration(seconds: 300);

  /// Menor `B_max` (2 × 32).
  static const int minBuffer = 64;

  /// Maior `B_max`.
  static const int maxBuffer = 512;

  /// Menor período de GPS.
  static const Duration minGpsPeriod = Duration(seconds: 1);

  /// Maior período de GPS.
  static const Duration maxGpsPeriod = Duration(seconds: 60);

  /// `T_lat`: tempo máximo de um bloco no buffer.
  final Duration latency;

  /// `B_max`: blocos guardados no celular.
  final int bufferCapacity;

  /// `T_GPS`: período do envio de posição.
  final Duration gpsPeriod;

  /// Envio de GPS ligado.
  final bool gpsEnabled;

  /// Cópia com todos os valores dentro das faixas.
  SyncSettings clamped() => SyncSettings(
    latency: _clampDuration(latency, minLatency, maxLatency),
    bufferCapacity: bufferCapacity.clamp(minBuffer, maxBuffer),
    gpsPeriod: _clampDuration(gpsPeriod, minGpsPeriod, maxGpsPeriod),
    gpsEnabled: gpsEnabled,
  );

  /// Cópia com campos trocados.
  SyncSettings copyWith({
    Duration? latency,
    int? bufferCapacity,
    Duration? gpsPeriod,
    bool? gpsEnabled,
  }) => SyncSettings(
    latency: latency ?? this.latency,
    bufferCapacity: bufferCapacity ?? this.bufferCapacity,
    gpsPeriod: gpsPeriod ?? this.gpsPeriod,
    gpsEnabled: gpsEnabled ?? this.gpsEnabled,
  );

  static Duration _clampDuration(Duration v, Duration lo, Duration hi) =>
      v < lo ? lo : (v > hi ? hi : v);

  @override
  bool operator ==(Object other) =>
      other is SyncSettings &&
      other.latency == latency &&
      other.bufferCapacity == bufferCapacity &&
      other.gpsPeriod == gpsPeriod &&
      other.gpsEnabled == gpsEnabled;

  @override
  int get hashCode =>
      Object.hash(latency, bufferCapacity, gpsPeriod, gpsEnabled);
}
