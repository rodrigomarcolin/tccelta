import 'dart:typed_data';

import 'package:tccelta_mobile/src/domain/sync/dev_id.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_block.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';
import 'package:tccelta_mobile/src/infra/sync/bench/bench_crypto.dart';
import 'package:tccelta_mobile/src/infra/sync/bench/bench_keys.dart';

import 'sync_known_vectors.dart';

/// `DevId` das chaves fixas de bancada.
final DevId benchDevId = DevId(SyncVectors.devId);

SyncBlock _build(
  Uint8List data, {
  required int tagLength,
  BenchKeys? keys,
  Uint8List? tag,
}) {
  final k = keys ?? BenchKeys.defaults();
  final t =
      tag ??
      (tagLength == 32
          ? blockTag(kRec: k.kRec, devId: SyncVectors.devId, data: data)
          : Uint8List(tagLength));
  final frame = SyncFrame(
    SyncMessageType.block,
    Uint8List.fromList([...data, ...t]),
  );
  return SyncBlock.fromFrame(frame);
}

Uint8List _header(int seq, int bootId, int tMs, int size) {
  final b = Uint8List(size);
  b.buffer.asByteData()
    ..setUint32(0, seq, Endian.little)
    ..setUint16(4, bootId, Endian.little)
    ..setUint32(6, tMs, Endian.little)
    ..setUint16(10, size, Endian.little);
  return b;
}

/// Bloco de dados (um registro de PID) com tag de [tagLength] bytes (0, 32 ou
/// 64); com 32, o HMAC é válido para as chaves de bancada.
SyncBlock dataBlock(
  int seq, {
  int bootId = 1,
  int tagLength = 32,
  BenchKeys? keys,
  Uint8List? tag,
}) {
  final data = _header(seq, bootId, seq * 10, 17)
    ..setRange(12, 17, [0x0C, 0, 0, 0x10, 0]);
  return _build(data, tagLength: tagLength, keys: keys, tag: tag);
}

/// Marcador BOOT em [seq] cobrindo `[from, to]`.
SyncBlock bootBlock(
  int seq,
  int from,
  int to, {
  int bootId = 2,
  int tagLength = 32,
}) {
  final data = _header(seq, bootId, 812, 23);
  data[12] = 0xFE;
  data.buffer.asByteData()
    ..setUint32(15, from, Endian.little)
    ..setUint32(19, to, Endian.little);
  return _build(data, tagLength: tagLength);
}

/// Marcador LOSS cobrindo `[from, to]` (o `seq` é `from`).
SyncBlock lossBlock(int from, int to, {int tagLength = 32}) {
  final data = _header(from, 0, 0, 23);
  data[12] = 0xFD;
  data.buffer.asByteData()
    ..setUint32(15, from, Endian.little)
    ..setUint32(19, to, Endian.little);
  return _build(data, tagLength: tagLength);
}
