import 'dart:convert';

import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';
import 'package:tccelta_mobile/src/core/errors/panel_failure.dart';
import 'package:tccelta_mobile/src/data/datasources/panel_datasource.dart';
import 'package:tccelta_mobile/src/data/repositories/panel_repository_impl.dart';
import 'package:tccelta_mobile/src/domain/telemetry/panel.dart';
import 'package:tccelta_mobile/src/domain/telemetry/panel_collection.dart';

class _MockPanelDatasource extends Mock implements PanelDatasource {}

void main() {
  late _MockPanelDatasource ds;
  late PanelRepositoryImpl repo;

  setUp(() {
    ds = _MockPanelDatasource();
    repo = PanelRepositoryImpl(ds);
  });

  group('load', () {
    test('devolve null quando o datasource não tem nada salvo', () async {
      when(ds.readRaw).thenReturn(null);

      expect(await repo.load(), isNull);
    });

    test('decodifica um JSON válido de volta para PanelsState', () async {
      const state = PanelsState(
        panels: [Panel(id: 'p1', name: 'Painel 1')],
        activeId: 'p1',
      );
      when(ds.readRaw).thenReturn(jsonEncode(state.toJson()));

      final loaded = await repo.load();

      expect(loaded?.activeId, state.activeId);
      expect(loaded?.panels.single.id, 'p1');
    });

    test('lança PanelStorageFailure sobre um JSON corrompido', () async {
      when(ds.readRaw).thenReturn('{isso não é um json válido');

      expect(() => repo.load(), throwsA(isA<PanelStorageFailure>()));
    });
  });

  group('save', () {
    test('serializa o estado e grava via writeRaw', () async {
      const state = PanelsState(
        panels: [Panel(id: 'p1', name: 'Painel 1')],
        activeId: 'p1',
      );
      when(() => ds.writeRaw(any())).thenAnswer((_) async {});

      await repo.save(state);

      verify(() => ds.writeRaw(jsonEncode(state.toJson()))).called(1);
    });

    test('lança PanelStorageFailure quando o datasource lança', () async {
      const state = PanelsState(
        panels: [Panel(id: 'p1', name: 'Painel 1')],
        activeId: 'p1',
      );
      when(() => ds.writeRaw(any())).thenThrow(Exception('disco cheio'));

      expect(() => repo.save(state), throwsA(isA<PanelStorageFailure>()));
    });
  });
}
