import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';
import 'package:tccelta_mobile/src/core/errors/last_dongle_failure.dart';
import 'package:tccelta_mobile/src/data/datasources/last_dongle_datasource.dart';
import 'package:tccelta_mobile/src/data/repositories/last_dongle_repository_impl.dart';
import 'package:tccelta_mobile/src/domain/ble/last_dongle.dart';

class _MockLastDongleDatasource extends Mock implements LastDongleDatasource {}

void main() {
  late _MockLastDongleDatasource ds;
  late LastDongleRepositoryImpl repo;

  setUp(() {
    ds = _MockLastDongleDatasource();
    repo = LastDongleRepositoryImpl(ds);
  });

  group('load', () {
    test('devolve null quando o datasource não tem nada salvo', () async {
      when(ds.readRaw).thenReturn(null);

      expect(await repo.load(), isNull);
    });

    test('decodifica um JSON válido de volta para LastDongle', () async {
      const device = LastDongle(id: 'AA:BB', name: 'OBD2Dongle');
      when(ds.readRaw).thenReturn(jsonEncode(device.toJson()));

      final loaded = await repo.load();

      expect(loaded, device);
    });

    test('lança LastDongleStorageFailure sobre um JSON corrompido', () async {
      when(ds.readRaw).thenReturn('{isso não é um json válido');

      expect(() => repo.load(), throwsA(isA<LastDongleStorageFailure>()));
    });
  });

  group('save', () {
    test('serializa o dongle e grava via writeRaw', () async {
      const device = LastDongle(id: 'AA:BB', name: 'OBD2Dongle');
      when(() => ds.writeRaw(any())).thenAnswer((_) async {});

      await repo.save(device);

      verify(() => ds.writeRaw(jsonEncode(device.toJson()))).called(1);
    });

    test('lança LastDongleStorageFailure quando o datasource lança', () async {
      const device = LastDongle(id: 'AA:BB', name: 'OBD2Dongle');
      when(() => ds.writeRaw(any())).thenThrow(Exception('disco cheio'));

      expect(() => repo.save(device), throwsA(isA<LastDongleStorageFailure>()));
    });
  });

  group('clear', () {
    test('delega para clearRaw', () async {
      when(ds.clearRaw).thenAnswer((_) async {});

      await repo.clear();

      verify(ds.clearRaw).called(1);
    });

    test('lança LastDongleStorageFailure quando o datasource lança', () async {
      when(ds.clearRaw).thenThrow(Exception('falha de storage'));

      expect(() => repo.clear(), throwsA(isA<LastDongleStorageFailure>()));
    });
  });
}
