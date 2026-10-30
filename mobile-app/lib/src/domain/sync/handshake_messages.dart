import 'dart:typed_data';

import 'package:meta/meta.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';

ByteData _view(Uint8List v) => v.buffer.asByteData(v.offsetInBytes, v.length);

/// Campos públicos do Desafio 0x01. Nunca verifica a tag.
@immutable
class ChallengeInfo {
  /// Cria a informação do desafio.
  const ChallengeInfo({
    required this.cS,
    required this.ackSeq,
    required this.cConf,
    required this.flags,
    this.diagMin,
  });

  /// Lê o `V` (45 ou 47 B); lança [FormatException] se o tamanho não bate.
  factory ChallengeInfo.fromValue(Uint8List v) {
    if (v.length != kChallengeLength && v.length != kChallengeDiagLength) {
      throw const FormatException('desafio deve ter 45 ou 47 B');
    }
    final d = _view(v);
    final flags = v[28];
    final hasDiag = flags & 0x01 != 0;
    if ((v.length == kChallengeDiagLength) != hasDiag) {
      throw const FormatException('tamanho do desafio não bate com flags');
    }
    return ChallengeInfo(
      cS: d.getUint32(0, Endian.little),
      ackSeq: d.getUint32(20, Endian.little),
      cConf: d.getUint32(24, Endian.little),
      flags: flags,
      diagMin: hasDiag ? d.getUint16(29, Endian.little) : null,
    );
  }

  /// Contador do servidor.
  final int cS;

  /// Último `seq` confirmado pelo servidor.
  final int ackSeq;

  /// Último `c_S` aceito pelo servidor.
  final int cConf;

  /// Byte de flags.
  final int flags;

  /// Minutos de diagnóstico (só com o bit 0 das flags).
  final int? diagMin;
}

/// Campos públicos da Resposta 0x02. Nunca verifica a tag.
@immutable
class ResponseInfo {
  /// Cria a informação da resposta.
  const ResponseInfo({
    required this.cS,
    required this.tailSeq,
    required this.headSeq,
    required this.bootId,
    required this.tMs,
  });

  /// Lê o `V` (50 B); lança [FormatException] se o tamanho não bate.
  factory ResponseInfo.fromValue(Uint8List v) {
    if (v.length != kResponseLength) {
      throw const FormatException('resposta deve ter 50 B');
    }
    final d = _view(v);
    return ResponseInfo(
      cS: d.getUint32(0, Endian.little),
      tailSeq: d.getUint32(20, Endian.little),
      headSeq: d.getUint32(24, Endian.little),
      bootId: d.getUint16(28, Endian.little),
      tMs: d.getUint32(30, Endian.little),
    );
  }

  /// Eco do `c_S`.
  final int cS;

  /// Primeiro `seq` ainda na flash.
  final int tailSeq;

  /// Último `seq` gravado.
  final int headSeq;

  /// Identificador do boot.
  final int bootId;

  /// Relógio do dongle em ms.
  final int tMs;
}

/// Campos públicos do ACK 0x04. Nunca verifica a tag.
@immutable
class AckInfo {
  /// Cria a informação do ACK.
  const AckInfo({
    required this.ackSeq,
    required this.cConf,
    required this.anchorFailed,
  });

  /// Lê o `V` (25 B); lança [FormatException] se o tamanho não bate.
  factory AckInfo.fromValue(Uint8List v) {
    if (v.length != kAckLength) {
      throw const FormatException('ACK deve ter 25 B');
    }
    final d = _view(v);
    return AckInfo(
      ackSeq: d.getUint32(0, Endian.little),
      cConf: d.getUint32(4, Endian.little),
      anchorFailed: v[8] & 0x01 != 0,
    );
  }

  /// Último `seq` confirmado.
  final int ackSeq;

  /// `c_S` confirmado.
  final int cConf;

  /// `true` se a âncora de tempo falhou (repetir o handshake).
  final bool anchorFailed;
}
