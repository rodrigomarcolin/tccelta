import 'package:flutter/foundation.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_status.dart';

/// Um código ativo agora no veículo — o que uma leitura de diagnóstico (Modos
/// 03/07/0A + 02) de fato devolve.
///
/// Diferente de `DtcCode`/`DtcDefinition` (catálogo, permanente), isto é dado
/// da leitura em si: só existe uma entrada para um código quando ele está
/// confirmado ou pendente agora — não há "entrada ativa inativa".
@immutable
class DtcActiveEntry {
  /// Cria uma entrada ativa.
  const DtcActiveEntry({
    required this.code,
    required this.status,
    this.detectedLabel,
  });

  /// Código no formato padrão OBD-II (ex.: "P0301") — casa com
  /// `DtcDefinition.code` do catálogo.
  final String code;

  /// Confirmado (Modo 03) ou pendente (Modo 07).
  final DtcStatus status;

  /// Texto pronto (pt-BR) de quando/como foi detectado (ex.: "há 2 dias · 3
  /// ciclos"). Não é um timestamp real — o firmware não guarda histórico
  /// entre sessões; é só um rótulo de exibição.
  final String? detectedLabel;

  @override
  bool operator ==(Object other) =>
      other is DtcActiveEntry &&
      other.code == code &&
      other.status == status &&
      other.detectedLabel == detectedLabel;

  @override
  int get hashCode => Object.hash(code, status, detectedLabel);

  @override
  String toString() => 'DtcActiveEntry($code: $status)';
}
