import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/data/datasources/dtc_datasource.dart';
import 'package:tccelta_mobile/src/data/datasources/elm327_client.dart';

import '../../support/scripted_ble_connection.dart';

void main() {
  group('DtcDatasource.readDtcResponses', () {
    test('"43 00" resolve normalmente (zero DTCs é sucesso)', () async {
      final connection = ScriptedBleConnection(
        responses: const {'03': '43 00\r>'},
      );
      final client = Elm327Client(connection);
      final datasource = DtcDatasource(client);

      final responses = await datasource.readDtcResponses(0x03);

      expect(responses, hasLength(1));
      expect(responses.single.payload, [0x00]);
      client.dispose();
    });

    test('NO DATA lança (resposta incompleta, não "zero DTCs")', () async {
      final connection = ScriptedBleConnection(
        responses: const {'03': 'NO DATA\r>'},
      );
      final client = Elm327Client(connection);
      final datasource = DtcDatasource(client);

      await expectLater(
        datasource.readDtcResponses(0x03),
        throwsA(isA<StateError>()),
      );
      client.dispose();
    });

    test('comando rejeitado ("?") lança', () async {
      final connection = ScriptedBleConnection(
        responses: const {'03': '?\r>'},
      );
      final client = Elm327Client(connection);
      final datasource = DtcDatasource(client);

      await expectLater(
        datasource.readDtcResponses(0x03),
        throwsA(isA<StateError>()),
      );
      client.dispose();
    });

    test('linha malformada (sem marcador reconhecível) lança', () async {
      final connection = ScriptedBleConnection(
        responses: const {'03': 'GARBAGE\r>'},
      );
      final client = Elm327Client(connection);
      final datasource = DtcDatasource(client);

      await expectLater(
        datasource.readDtcResponses(0x03),
        throwsA(isA<StateError>()),
      );
      client.dispose();
    });
  });
}
