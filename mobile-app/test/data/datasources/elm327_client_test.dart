import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/errors/obd_failure.dart';
import 'package:tccelta_mobile/src/data/datasources/elm327_client.dart';

import '../../support/scripted_ble_connection.dart';

void main() {
  group('Elm327Client', () {
    test('remonta chunks fragmentados até o prompt', () async {
      final conn = ScriptedBleConnection(autoRespond: false);
      final client = Elm327Client(conn);

      final future = client.command('010C');
      // Deixa o write/registro do pending acontecer.
      await Future<void>.delayed(Duration.zero);

      conn.emit('41 0C ');
      await Future<void>.delayed(Duration.zero);
      conn.emit('17 70\r>');

      expect(await future, '41 0C 17 70');
      client.dispose();
    });

    test('resolve resposta automática do dongle', () async {
      final conn = ScriptedBleConnection(
        responses: const {'010D': '41 0D 3C\r>'},
      );
      final client = Elm327Client(conn);

      expect(await client.command('010D'), '41 0D 3C');
      expect(conn.written, contains('010D'));
      client.dispose();
    });

    test('serializa comandos: um write por vez, em ordem', () async {
      final conn = ScriptedBleConnection(
        responses: const {
          '010C': '41 0C 17 70\r>',
          '010D': '41 0D 3C\r>',
        },
      );
      final client = Elm327Client(conn);

      final results = await Future.wait([
        client.command('010C'),
        client.command('010D'),
      ]);

      expect(results, ['41 0C 17 70', '41 0D 3C']);
      expect(conn.written, ['010C', '010D']);
      client.dispose();
    });

    test('estoura ObdCommandFailure em timeout sem resposta', () async {
      final conn = ScriptedBleConnection(autoRespond: false);
      final client = Elm327Client(conn);

      await expectLater(
        client.command('010C', timeout: const Duration(milliseconds: 20)),
        throwsA(isA<ObdCommandFailure>()),
      );
      client.dispose();
    });
  });
}
