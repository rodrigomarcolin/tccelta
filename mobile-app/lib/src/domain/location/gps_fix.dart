import 'package:meta/meta.dart';

/// Posição do celular no formato aceito pelo `AT#GPS` do dongle.
@immutable
class GpsFix {
  /// Cria a posição com os campos já convertidos.
  const GpsFix({
    required this.latE7,
    required this.lonE7,
    required this.altMm,
    required this.accuracyM,
    required this.at,
  });

  /// Converte graus/metros para as unidades do dongle, com `clamp` às faixas
  /// do parser do firmware.
  factory GpsFix.fromDegrees({
    required double lat,
    required double lon,
    required double altM,
    required double accuracyM,
    required Duration at,
  }) => GpsFix(
    latE7: (lat * 1e7).round().clamp(-_maxLatE7, _maxLatE7),
    lonE7: (lon * 1e7).round().clamp(-_maxLonE7, _maxLonE7),
    altMm: (altM * 1000).round().clamp(_minInt32, _maxInt32),
    accuracyM: accuracyM,
    at: at,
  );

  static const int _maxLatE7 = 900000000;
  static const int _maxLonE7 = 1800000000;
  static const int _minInt32 = -2147483648;
  static const int _maxInt32 = 2147483647;

  /// Latitude em graus × 10⁷.
  final int latE7;

  /// Longitude em graus × 10⁷.
  final int lonE7;

  /// Altitude em milímetros.
  final int altMm;

  /// Precisão horizontal estimada, em metros.
  final double accuracyM;

  /// Instante da leitura no relógio monotônico.
  final Duration at;

  /// Texto do comando ELM327 (sem terminador).
  String toAtCommand() => 'AT#GPS $latE7 $lonE7 $altMm';
}

/// Estado da permissão de localização do app.
enum LocationPermissionState {
  /// Permitida o tempo todo (inclui segundo plano).
  granted,

  /// Permitida só com o app em primeiro plano.
  grantedWhileInUse,

  /// Negada; ainda dá para pedir.
  denied,

  /// Negada para sempre; só pelos ajustes do sistema.
  deniedForever,

  /// Serviço de localização desligado no aparelho.
  serviceDisabled,
}
