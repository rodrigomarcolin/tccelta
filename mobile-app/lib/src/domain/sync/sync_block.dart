import 'dart:typed_data';

import 'package:meta/meta.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';

/// Tipo de marcador de um bloco.
enum MarkerKind {
  /// Bloco de dados comum.
  none,

  /// Marcador de boot (`0xFE`).
  boot,

  /// Marcador de perda (`0xFD`).
  loss,
}

/// Bloco 0x03 com os campos públicos do cabeçalho e, nos marcadores, a faixa
/// de `seq`. Não valida a tag: ela é conferida só pelo servidor.
@immutable
class SyncBlock {
  /// Cria o bloco.
  const SyncBlock({
    required this.seq,
    required this.bootId,
    required this.tMs,
    required this.length,
    required this.marker,
    required this.fromSeq,
    required this.toSeq,
    required this.frame,
  });

  /// Lê o cabeçalho (e o marcador, se houver) do [frame] tipo 0x03; lança
  /// [FormatException] se curto ou incoerente.
  factory SyncBlock.fromFrame(SyncFrame frame) {
    final v = frame.value;
    if (frame.type != SyncMessageType.block || v.length < kBlockHeaderSize) {
      throw const FormatException('bloco inválido');
    }
    final d = v.buffer.asByteData(v.offsetInBytes, v.length);
    final length = d.getUint16(10, Endian.little);
    if (length < kBlockHeaderSize || length > v.length) {
      throw const FormatException('tamanho do bloco incoerente');
    }
    var marker = MarkerKind.none;
    var from = 0;
    var to = 0;
    if (length == 23 && (v[12] == 0xFD || v[12] == 0xFE)) {
      marker = v[12] == 0xFE ? MarkerKind.boot : MarkerKind.loss;
      from = d.getUint32(15, Endian.little);
      to = d.getUint32(19, Endian.little);
    }
    return SyncBlock(
      seq: d.getUint32(0, Endian.little),
      bootId: d.getUint16(4, Endian.little),
      tMs: d.getUint32(6, Endian.little),
      length: length,
      marker: marker,
      fromSeq: from,
      toSeq: to,
      frame: frame,
    );
  }

  /// Número de sequência.
  final int seq;

  /// Identificador do boot (0 no LOSS).
  final int bootId;

  /// `t_ms` inicial do bloco.
  final int tMs;

  /// Bytes da área de dados (cabeçalho incluído).
  final int length;

  /// Tipo de marcador.
  final MarkerKind marker;

  /// Primeiro `seq` da faixa do marcador (0 se não é marcador).
  final int fromSeq;

  /// Último `seq` da faixa do marcador (0 se não é marcador).
  final int toSeq;

  /// Frame original, repassado sem alteração.
  final SyncFrame frame;

  /// Último `seq` coberto por este bloco (`toSeq` no LOSS, `seq` nos demais).
  int get lastCoveredSeq => marker == MarkerKind.loss ? toSeq : seq;
}
