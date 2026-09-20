import 'package:flutter/foundation.dart';

/// Um valor congelado no momento em que um `DtcCode` foi detectado (Modo 02).
///
/// [label] e [value] já vêm formatados para exibição (ex.: `('Rotação',
/// '2.480 rpm')`) — não há grandeza física a converter aqui, ao contrário de
/// `Obd2Reading`: o freeze frame é um retrato estático, não uma leitura ao
/// vivo.
@immutable
class DtcFreezeFrameEntry {
  /// Cria uma entrada de congelamento.
  const DtcFreezeFrameEntry({required this.label, required this.value});

  /// Rótulo da grandeza (ex.: "Rotação").
  final String label;

  /// Valor já formatado, com unidade (ex.: "2.480 rpm").
  final String value;

  @override
  bool operator ==(Object other) =>
      other is DtcFreezeFrameEntry &&
      other.label == label &&
      other.value == value;

  @override
  int get hashCode => Object.hash(label, value);

  @override
  String toString() => 'DtcFreezeFrameEntry($label: $value)';
}
