import 'package:flutter/foundation.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_active_entry.dart';

/// Resultado de uma leitura de diagnóstico: os códigos ativos agora no
/// veículo + o estado da luz de falha (MIL).
///
/// [active] traz **só** os códigos de fato ativos (confirmados/pendentes) —
/// nunca o catálogo inteiro: no protocolo real, os Modos 03/07/0A não têm
/// como devolver "o que existe mas não está ativo". O catálogo completo vem
/// de `dtcCatalog` (dado de domínio, não desta leitura).
///
/// [milOn] é um campo próprio, não derivado de [active] — no protocolo real
/// ele vem de um PID independente (Modo 01, PID 01), lido separadamente dos
/// Modos 03/07/0A que trazem os códigos em si.
@immutable
class DtcSnapshot {
  /// Cria um retrato de diagnóstico.
  const DtcSnapshot({required this.active, required this.milOn});

  /// Códigos ativos agora (confirmados ou pendentes).
  final List<DtcActiveEntry> active;

  /// `true` quando a luz de falha (MIL) está acesa.
  final bool milOn;

  @override
  bool operator ==(Object other) =>
      other is DtcSnapshot &&
      other.milOn == milOn &&
      listEquals(other.active, active);

  @override
  int get hashCode => Object.hash(milOn, Object.hashAll(active));
}
