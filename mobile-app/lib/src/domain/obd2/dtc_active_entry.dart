import 'package:flutter/foundation.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_freeze_frame_entry.dart';
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
    this.ecuId,
    this.detectedLabel,
    this.freezeFrame = const [],
  });

  /// Código no formato padrão OBD-II (ex.: "P0301") — casa com
  /// `DtcDefinition.code` do catálogo.
  final String code;

  /// Confirmado (Modo 03) ou pendente (Modo 07).
  final DtcStatus status;

  /// CAN response identifier when the DTC response included a header.
  final int? ecuId;

  /// Texto pronto (pt-BR) de quando/como foi detectado (ex.: "há 2 dias · 3
  /// ciclos"). Não é um timestamp real — o firmware não guarda histórico
  /// entre sessões; é só um rótulo de exibição.
  final String? detectedLabel;

  /// Valores congelados no momento da falha (Modo 02). Vazia = sem
  /// congelamento disponível (comum em códigos pendentes).
  final List<DtcFreezeFrameEntry> freezeFrame;

  /// Cópia com os campos sobrescritos — usada pelo repository real pra
  /// anexar o freeze frame (Modo 02) numa entrada já montada, sem refazer a
  /// leitura dos Modos 03/07/0A.
  DtcActiveEntry copyWith({
    DtcStatus? status,
    int? ecuId,
    String? detectedLabel,
    List<DtcFreezeFrameEntry>? freezeFrame,
  }) => DtcActiveEntry(
    code: code,
    status: status ?? this.status,
    ecuId: ecuId ?? this.ecuId,
    detectedLabel: detectedLabel ?? this.detectedLabel,
    freezeFrame: freezeFrame ?? this.freezeFrame,
  );

  @override
  bool operator ==(Object other) =>
      other is DtcActiveEntry &&
      other.code == code &&
      other.status == status &&
      other.ecuId == ecuId &&
      other.detectedLabel == detectedLabel &&
      listEquals(other.freezeFrame, freezeFrame);

  @override
  int get hashCode =>
      Object.hash(code, status, ecuId, detectedLabel, Object.hashAll(freezeFrame));

  @override
  String toString() => 'DtcActiveEntry($code: $status)';
}
