import 'dart:typed_data';

import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_block.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';

/// Reescreve a área de dados de um bloco de dados (`0x03`, marcador `none`):
/// mantém o cabeçalho (`seq`/`bootId`/`tMs`) e **todos os registros de GPS**
/// intactos, troca os registros de PID pelos valores informados e devolve o
/// bloco com **tag vazia** (perfil `none`).
abstract final class BlockForger {
  static const int _gpsType = 0xFF;
  static const int _gpsRecordSize = 15;
  static const int _pidRecordSize = 5;
  static const int _bootType = 0xFE;
  static const int _lossType = 0xFD;
  static const int _maxDataSize = 1024;

  /// Monta o bloco forjado a partir de [real] e da amostra [sample] (PID→valor
  /// em unidade de engenharia). Cada PID vira um registro de 5 B
  /// (`pid | dt(u16)=0 | A | B`) via [Obd2Pid.encode]; os registros de GPS de
  /// [real] são copiados sem alteração.
  static SyncBlock forge(SyncBlock real, Map<Obd2Pid, double> sample) {
    final records = <int>[];
    sample.forEach((pid, value) {
      final bytes = pid.encode(value);
      final a = bytes.isNotEmpty ? bytes[0] : 0;
      final b = bytes.length > 1 ? bytes[1] : 0;
      records
        ..add(pid.pid)
        ..add(0)
        ..add(0)
        ..add(a)
        ..add(b);
    });
    for (final gps in _gpsRecords(real.frame.value, real.length)) {
      if (kBlockHeaderSize + records.length + gps.length > _maxDataSize) break;
      records.addAll(gps);
    }

    final size = kBlockHeaderSize + records.length;
    final value = Uint8List(size);
    value.buffer.asByteData()
      ..setUint32(0, real.seq, Endian.little)
      ..setUint16(4, real.bootId, Endian.little)
      ..setUint32(6, real.tMs, Endian.little)
      ..setUint16(10, size, Endian.little);
    value.setRange(kBlockHeaderSize, size, records);
    return SyncBlock.fromFrame(SyncFrame(SyncMessageType.block, value));
  }

  /// Extrai os registros de GPS (`0xFF`, 15 B cada) da área de dados.
  static List<List<int>> _gpsRecords(Uint8List value, int length) {
    final gps = <List<int>>[];
    var i = kBlockHeaderSize;
    while (i < length) {
      final type = value[i];
      if (type == _gpsType) {
        if (i + _gpsRecordSize > length) break;
        gps.add(value.sublist(i, i + _gpsRecordSize));
        i += _gpsRecordSize;
      } else if (type == _bootType || type == _lossType) {
        break; // marcador não ocorre num bloco de dados
      } else {
        if (i + _pidRecordSize > length) break;
        i += _pidRecordSize;
      }
    }
    return gps;
  }
}
