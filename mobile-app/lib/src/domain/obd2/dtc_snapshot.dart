import 'package:flutter/foundation.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_code.dart';

/// Resultado de uma leitura de diagnóstico: o catálogo de códigos do veículo
/// + o estado da luz de injeção (MIL).
///
/// [milOn] é um campo próprio, não derivado de [codes] — no protocolo real
/// ele vem de um PID independente (Modo 01, PID 01), lido separadamente dos
/// Modos 03/07/0A que trazem os códigos em si.
@immutable
class DtcSnapshot {
  /// Cria um retrato de diagnóstico.
  const DtcSnapshot({required this.codes, required this.milOn});

  /// Catálogo de códigos do veículo (ativos e inativos).
  final List<DtcCode> codes;

  /// `true` quando a luz de injeção (MIL) está acesa.
  final bool milOn;

  @override
  bool operator ==(Object other) =>
      other is DtcSnapshot &&
      other.milOn == milOn &&
      listEquals(other.codes, codes);

  @override
  int get hashCode => Object.hash(milOn, Object.hashAll(codes));
}
