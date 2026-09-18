import 'package:flutter/foundation.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_component.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_severity.dart';

/// Uma entrada do catálogo de DTCs — dado de referência permanente, que não
/// vem de nenhuma leitura do veículo.
///
/// Diferente de `DtcCode`, não carrega nenhum estado "desta leitura" (sem
/// status/detectedLabel): o protocolo OBD-II (Modos 03/07/0A) só
/// devolve os códigos ativos agora, nunca um catálogo do que o veículo
/// suporta — então o catálogo é dado próprio do app, não do datasource, e
/// sobrevive à troca do repository mockado pelo real.
@immutable
class DtcDefinition {
  /// Cria uma entrada de catálogo.
  const DtcDefinition({
    required this.code,
    required this.component,
    required this.name,
    required this.severity,
    this.causes = const [],
  });

  /// Código no formato padrão OBD-II (ex.: "P0301").
  final String code;

  /// Componente/sistema do veículo a que pertence.
  final DtcComponent component;

  /// Descrição legível (pt-BR) da falha.
  final String name;

  /// Severidade curada, para ênfase visual.
  final DtcSeverity severity;

  /// Possíveis causas (pt-BR), na ordem de exibição. Vazia = sem causas
  /// catalogadas para este código.
  final List<String> causes;

  @override
  bool operator ==(Object other) =>
      other is DtcDefinition &&
      other.code == code &&
      other.component == component &&
      other.name == name &&
      other.severity == severity &&
      listEquals(other.causes, causes);

  @override
  int get hashCode =>
      Object.hash(code, component, name, severity, Object.hashAll(causes));

  @override
  String toString() => 'DtcDefinition($code: $name)';
}
