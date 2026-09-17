import 'package:flutter/foundation.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_component.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_freeze_frame_entry.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_severity.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_status.dart';

/// Um código de diagnóstico (DTC) do catálogo do veículo.
///
/// Modelo de domínio puro e imutável — mesmo estilo de `Obd2Reading`/`Panel`:
/// nenhuma dependência de Flutter além de `@immutable`. Um código pode existir
/// no catálogo sem estar ativo agora ([status] `null`); [causes] e
/// [freezeFrame] só vêm preenchidos para os poucos códigos com detalhe rico no
/// mock — a UI trata listas vazias como "sem essa informação".
@immutable
class DtcCode {
  /// Cria um código de diagnóstico.
  const DtcCode({
    required this.code,
    required this.component,
    required this.name,
    required this.severity,
    this.status,
    this.detectedLabel,
    this.causes = const [],
    this.freezeFrame = const [],
  });

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

  /// Valores congelados no momento da falha (Modo 02). Vazia = sem
  /// congelamento disponível (comum em códigos pendentes).
  final List<DtcFreezeFrameEntry> freezeFrame;

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
      listEquals(other.causes, causes) &&
      listEquals(other.freezeFrame, freezeFrame);

  @override
  int get hashCode => Object.hash(
    code,
    component,
    name,
    severity,
    status,
    detectedLabel,
    Object.hashAll(causes),
    Object.hashAll(freezeFrame),
  );

  @override
  String toString() => 'DtcCode($code: $name, status: $status)';
}
