import 'dart:typed_data';

import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_block.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';
import 'package:tccelta_mobile/src/infra/sync/inject/block_forger.dart';

/// Registro de GPS reconhecível (15 B): `0xFF | dt(u16) | lat | lon | alt`.
final List<int> _gps = [
  0xFF, 0x64, 0x00, //
  1, 2, 3, 4, //
  5, 6, 7, 8, //
  9, 10, 11, 12,
];

/// Monta um bloco de dados real (com um PID e um GPS) e [tagLen] bytes de tag.
SyncBlock _realBlock({
  required int seq,
  required int bootId,
  required int tMs,
  List<int> gps = const [],
  int tagLen = 0,
}) {
  final records = <int>[
    0x0C, 0, 0, 0x10, 0x00, // rpm real qualquer
    ...gps,
  ];
  final size = kBlockHeaderSize + records.length;
  final value = Uint8List(size + tagLen);
  value.buffer.asByteData()
    ..setUint32(0, seq, Endian.little)
    ..setUint16(4, bootId, Endian.little)
    ..setUint32(6, tMs, Endian.little)
    ..setUint16(10, size, Endian.little);
  value.setRange(kBlockHeaderSize, size, records);
  return SyncBlock.fromFrame(SyncFrame(SyncMessageType.block, value));
}

/// Lê os registros de PID (pid → [A, B]) de um bloco forjado.
Map<int, List<int>> _pids(Uint8List v) {
  final size = v.buffer.asByteData().getUint16(10, Endian.little);
  final out = <int, List<int>>{};
  var i = kBlockHeaderSize;
  while (i < size) {
    if (v[i] == 0xFF) {
      i += 15;
    } else {
      out[v[i]] = [v[i + 3], v[i + 4]];
      i += 5;
    }
  }
  return out;
}

void main() {
  group('BlockForger.forge', () {
    test('preserva cabeçalho e emite tag vazia (none)', () {
      final real = _realBlock(seq: 42, bootId: 7, tMs: 123456, tagLen: 32);
      final forged = BlockForger.forge(real, {Obd2Pid.rpm: 3000});

      expect(forged.seq, 42);
      expect(forged.bootId, 7);
      expect(forged.tMs, 123456);
      // tag vazia: o valor do frame termina exatamente no fim da área de dados.
      expect(forged.frame.value.length, forged.length);
      expect(forged.marker, MarkerKind.none);
    });

    test('substitui os PIDs pelos valores da amostra', () {
      final real = _realBlock(seq: 1, bootId: 1, tMs: 0, tagLen: 32);
      final forged = BlockForger.forge(real, {
        Obd2Pid.rpm: 3000,
        Obd2Pid.speed: 120,
      });

      final pids = _pids(forged.frame.value);
      expect(pids.keys, containsAll(<int>[Obd2Pid.rpm.pid, Obd2Pid.speed.pid]));
      expect(Obd2Pid.rpm.decode(pids[Obd2Pid.rpm.pid]!), closeTo(3000, 0.3));
      expect(Obd2Pid.speed.decode(pids[Obd2Pid.speed.pid]!), closeTo(120, 0.5));
    });

    test('mantém os registros de GPS intactos', () {
      final real = _realBlock(
        seq: 9,
        bootId: 2,
        tMs: 10,
        gps: _gps,
        tagLen: 64,
      );
      final forged = BlockForger.forge(real, {Obd2Pid.rpm: 2000});

      final v = forged.frame.value;
      final start = v.indexOf(0xFF);
      expect(start, greaterThanOrEqualTo(kBlockHeaderSize));
      expect(v.sublist(start, start + 15), _gps);
    });

    test('funciona sem GPS e respeita o tamanho mínimo', () {
      final real = _realBlock(seq: 5, bootId: 1, tMs: 0);
      final forged = BlockForger.forge(real, {
        Obd2Pid.rpm: 1500,
        Obd2Pid.speed: 40,
      });
      expect(forged.length, greaterThanOrEqualTo(kBlockHeaderSize + 5));
      expect(forged.frame.value.indexOf(0xFF), -1);
    });
  });
}
