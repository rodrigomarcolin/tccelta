import 'package:tccelta_mobile/src/core/errors/obd_failure.dart';
import 'package:tccelta_mobile/src/data/datasources/elm327_client.dart';
import 'package:tccelta_mobile/src/domain/location/gps_fix.dart';

/// Comandos `AT#` do dongle coletor sobre o [Elm327Client] da conexão.
class CollectorCommandDatasource {
  /// Cria o datasource sobre o [_client].
  CollectorCommandDatasource(this._client);

  final Elm327Client _client;

  /// Envia a posição ao dongle. Lança [ObdCommandFailure] se a resposta não
  /// for `OK`.
  Future<void> sendGps(GpsFix fix) async {
    final reply = await _client.command(
      fix.toAtCommand(),
      timeout: const Duration(seconds: 2),
    );
    if (!reply.toUpperCase().contains('OK')) {
      throw ObdCommandFailure('Dongle recusou a posição: "$reply"');
    }
  }
}
