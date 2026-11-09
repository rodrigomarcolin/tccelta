import 'package:flutter/services.dart' show rootBundle;
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/infra/sync/inject/telemetry_profile.dart';

/// Sequência de amostras de telemetria lida de um CSV, entregue em ciclo.
///
/// O CSV tem uma linha de cabeçalho com nomes de [Obd2Pid] (ex.: `rpm,speed`)
/// e uma linha por amostra, com os valores em unidade de engenharia. Cada
/// amostra é um mapa PID→valor; [next] percorre as amostras repetindo do
/// início ao chegar ao fim.
class CsvTelemetrySource {
  /// Cria a fonte a partir das [samples] já parseadas (ao menos uma).
  CsvTelemetrySource(this.samples)
    : assert(samples.isNotEmpty, 'CSV precisa de ao menos uma amostra');

  /// Amostras na ordem do arquivo.
  final List<Map<Obd2Pid, double>> samples;

  int _cursor = 0;

  /// Próxima amostra, ciclando ao fim do arquivo.
  Map<Obd2Pid, double> next() {
    final sample = samples[_cursor % samples.length];
    _cursor++;
    return sample;
  }

  /// Carrega e parseia o CSV do [profile] a partir dos assets.
  static Future<CsvTelemetrySource> load(TelemetryProfile profile) async {
    final raw = await rootBundle.loadString(profile.assetPath);
    return CsvTelemetrySource(parse(raw));
  }

  /// Converte o texto [csv] em amostras; lança [FormatException] se o cabeçalho
  /// nomear um PID desconhecido ou uma linha não tiver o número de colunas.
  static List<Map<Obd2Pid, double>> parse(String csv) {
    final lines = csv
        .split('\n')
        .map((l) => l.trim())
        .where((l) => l.isNotEmpty)
        .toList();
    if (lines.length < 2) {
      throw const FormatException('CSV de telemetria sem amostras');
    }
    final header = lines.first.split(',').map((h) => h.trim()).toList();
    final pids = [for (final name in header) Obd2Pid.values.byName(name)];
    final out = <Map<Obd2Pid, double>>[];
    for (final line in lines.skip(1)) {
      final cells = line.split(',');
      if (cells.length != pids.length) {
        throw FormatException(
          'linha com ${cells.length} colunas, esperado ${pids.length}',
        );
      }
      out.add({
        for (var i = 0; i < pids.length; i++)
          pids[i]: double.parse(cells[i].trim()),
      });
    }
    return out;
  }
}
