import 'dart:typed_data';

import 'package:meta/meta.dart';
import 'package:tccelta_mobile/src/domain/sync/dev_id.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';

/// Flags do aviso de novo bloco (Tabela 4). Bits reservados são ignorados.
@immutable
class NoticeFlags {
  /// Cria as flags a partir do byte [raw].
  const NoticeFlags(this.raw);

  /// Byte cru.
  final int raw;

  /// Bit 0: nenhum handshake nesta conexão.
  bool get noHandshakeConnection => raw & 0x01 != 0;

  /// Bit 1: nenhum handshake neste boot.
  bool get noHandshakeBoot => raw & 0x02 != 0;

  /// Bit 2: flash acima de 80 %.
  bool get flashHigh => raw & 0x04 != 0;

  @override
  bool operator ==(Object other) => other is NoticeFlags && other.raw == raw;

  @override
  int get hashCode => raw.hashCode;
}

/// Aviso 0x07 (`V` com 13 B): `HEAD_seq`, flags e `DevId`.
@immutable
class NewBlockNotice {
  /// Cria o aviso.
  const NewBlockNotice({
    required this.headSeq,
    required this.flags,
    required this.devId,
  });

  /// Lê o `V` do aviso; lança [FormatException] se não tiver 13 B.
  factory NewBlockNotice.fromValue(Uint8List v) {
    if (v.length != kNoticeLength) {
      throw const FormatException('aviso deve ter 13 B');
    }
    return NewBlockNotice(
      headSeq: v.buffer
          .asByteData(v.offsetInBytes, v.length)
          .getUint32(0, Endian.little),
      flags: NoticeFlags(v[4]),
      devId: DevId(v.sublist(5, 13)),
    );
  }

  /// Último `seq` gravado no dongle.
  final int headSeq;

  /// Flags do aviso.
  final NoticeFlags flags;

  /// Identidade do dongle.
  final DevId devId;
}
