import 'package:flutter/foundation.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_active_entry.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_component.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_definition.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_severity.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_status.dart';

/// Um código de diagnóstico (DTC), pronto para exibição na aba de
/// Diagnóstico.
///
/// Modelo de domínio puro e imutável — mesmo estilo de `Obd2Reading`/`Panel`:
/// nenhuma dependência de Flutter além de `@immutable`. Diferente de
/// [DtcDefinition] (catálogo, permanente) e [DtcActiveEntry] (leitura, só os
/// ativos agora), um `DtcCode` é a junção dos dois — construído via
/// [DtcCode.fromDefinition], nunca devolvido cru por um repository. Um
/// código pode existir no catálogo sem estar ativo agora ([status] `null`).
@immutable
class DtcCode {
  /// Cria um código de diagnóstico já pronto para exibição.
  const DtcCode({
    required this.code,
    required this.component,
    required this.name,
    required this.severity,
    this.status,
    this.detectedLabel,
    this.causes = const [],
  });

  /// Junta uma entrada do catálogo (dado permanente) com o resultado de uma
  /// leitura (`DtcRepository.read()`), quando o código está ativo agora —
  /// é assim que a vista completa do catálogo é "customizada" para destacar
  /// os DTCs ativos, sem o catálogo em si passar por nenhum repository.
  factory DtcCode.fromDefinition(
    DtcDefinition definition, {
    DtcActiveEntry? active,
  }) => DtcCode(
    code: definition.code,
    component: definition.component,
    name: definition.name,
    severity: definition.severity,
    causes: definition.causes,
    status: active?.status,
    detectedLabel: active?.detectedLabel,
  );

  /// Código no formato padrão OBD-II (ex.: "P0301").
  final String code;

  /// Componente/sistema do veículo a que pertence.
  final DtcComponent component;

  /// Descrição legível (pt-BR) da falha.
  final String name;

  /// Severidade curada, para ênfase visual.
  final DtcSeverity severity;

  /// Estado de ativação na leitura atual. `null` = não ativo agora (código do
  /// catálogo, visto em leituras anteriores ou nunca disparado).
  final DtcStatus? status;

  /// Texto pronto (pt-BR) de quando/como foi detectado (ex.: "há 2 dias · 3
  /// ciclos"). Não é um timestamp real — o firmware não guarda histórico
  /// entre sessões; é só um rótulo de exibição. `null` quando [status] é
  /// `null` ou a informação não está disponível.
  final String? detectedLabel;

  /// Possíveis causas (pt-BR), na ordem de exibição. Vazia = sem causas
  /// catalogadas para este código.
  final List<String> causes;

  /// `true` quando o código está ativo na leitura atual ([status] não nulo).
  bool get isActive => status != null;

  @override
  bool operator ==(Object other) =>
      other is DtcCode &&
      other.code == code &&
      other.component == component &&
      other.name == name &&
      other.severity == severity &&
      other.status == status &&
      other.detectedLabel == detectedLabel &&
      listEquals(other.causes, causes);

  @override
  int get hashCode => Object.hash(
    code,
    component,
    name,
    severity,
    status,
    detectedLabel,
    Object.hashAll(causes),
  );

  @override
  String toString() => 'DtcCode($code: $name, status: $status)';
}
