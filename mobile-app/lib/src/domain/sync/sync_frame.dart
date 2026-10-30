import 'dart:typed_data';

import 'package:meta/meta.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';

/// Mensagem TLV do serviço SYNC: tipo (`T`, u8), comprimento (`L`, u16 LE) e
/// valor (`V`). O app transporta os bytes sem alterá-los.
@immutable
class SyncFrame {
  /// Cria um frame de [type] com [value].
  const SyncFrame(this.type, this.value);

  /// Pedido de lote de [k] blocos (1…65535).
  factory SyncFrame.batchRequest(int k) {
    if (k < 1 || k > 0xFFFF) {
      throw RangeError.range(k, 1, 0xFFFF, 'k');
    }
    return SyncFrame(
      SyncMessageType.batchRequest,
      Uint8List(2)..buffer.asByteData().setUint16(0, k, Endian.little),
    );
  }

  /// Mensagem de erro com [code].
  factory SyncFrame.error(SyncErrorCode code) =>
      SyncFrame(SyncMessageType.error, Uint8List.fromList([code.code]));

  /// Interpreta [bytes] como um TLV completo; exige `L == len − 3` e tipo
  /// conhecido. Lança [FormatException] se incoerente.
  factory SyncFrame.parseTlv(Uint8List bytes) {
    if (bytes.length < 3) throw const FormatException('TLV curto');
    final length = bytes.buffer
        .asByteData(bytes.offsetInBytes, bytes.length)
        .getUint16(1, Endian.little);
    if (length != bytes.length - 3) {
      throw const FormatException('TLV com L incoerente');
    }
    final type = SyncMessageType.fromCode(bytes[0]);
    if (type == null) throw const FormatException('TLV de tipo desconhecido');
    return SyncFrame(type, Uint8List.fromList(bytes.sublist(3)));
  }

  /// Tipo da mensagem.
  final SyncMessageType type;

  /// Bytes de `V`.
  final Uint8List value;

  /// Serializa em `T | L | V`.
  Uint8List toTlv() {
    final out = Uint8List(3 + value.length);
    out[0] = type.code;
    out.buffer.asByteData().setUint16(1, value.length, Endian.little);
    out.setRange(3, out.length, value);
    return out;
  }

  @override
  bool operator ==(Object other) =>
      other is SyncFrame &&
      other.type == type &&
      other.value.length == value.length &&
      _same(other.value, value);

  @override
  int get hashCode => Object.hash(type, Object.hashAll(value));

  static bool _same(Uint8List a, Uint8List b) {
    for (var i = 0; i < a.length; i++) {
      if (a[i] != b[i]) return false;
    }
    return true;
  }

  @override
  String toString() => 'SyncFrame(${type.name}, ${value.length} B)';
}
