import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/data/datasources/elm327_client.dart';
import 'package:tccelta_mobile/src/data/datasources/obd2_datasource.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';

import '../../support/scripted_ble_connection.dart';

void main() {
  test('returns one structured reading per ECU', () async {
    final connection = ScriptedBleConnection(
      responses: const {
        '010C': '7E8 06 41 0C 17 70\r7E9 06 41 0C 18 00\r>',
      },
    );
    final client = Elm327Client(connection);
    final datasource = Obd2Datasource(client);

    final responses = await datasource.readPidResponses(Obd2Pid.rpm);

    expect(responses, hasLength(2));
    expect(responses.map((response) => response.ecuId), [0x7E8, 0x7E9]);
    expect(responses.map((response) => response.payload), [
      [0x17, 0x70],
      [0x18, 0x00],
    ]);
    client.dispose();
  });

  test(
    'readMonitorStatusResponses returns one structured reading per ECU',
    () async {
      final connection = ScriptedBleConnection(
        responses: const {
          '0101': '7E8 06 41 01 82 00 00 00\r7E9 06 41 01 00 00 00 00\r>',
        },
      );
      final client = Elm327Client(connection);
      final datasource = Obd2Datasource(client);

      final responses = await datasource.readMonitorStatusResponses();

      expect(responses, hasLength(2));
      expect(responses.map((response) => response.ecuId), [0x7E8, 0x7E9]);
      expect(responses.map((response) => response.payload), [
        [0x82, 0x00, 0x00, 0x00],
        [0x00, 0x00, 0x00, 0x00],
      ]);
      client.dispose();
    },
  );
}
