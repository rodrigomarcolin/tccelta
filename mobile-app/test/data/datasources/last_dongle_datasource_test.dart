import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:tccelta_mobile/src/data/datasources/last_dongle_datasource.dart';

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  test('readRaw devolve null quando nada foi salvo ainda', () async {
    final ds = LastDongleDatasource(await SharedPreferences.getInstance());

    expect(ds.readRaw(), isNull);
  });

  test('writeRaw seguido de readRaw devolve o mesmo conteúdo', () async {
    final ds = LastDongleDatasource(await SharedPreferences.getInstance());

    await ds.writeRaw('{"id":"AA:BB","name":"OBD2Dongle"}');

    expect(ds.readRaw(), '{"id":"AA:BB","name":"OBD2Dongle"}');
  });

  test('writeRaw sobrescreve o conteúdo anterior', () async {
    final ds = LastDongleDatasource(await SharedPreferences.getInstance());
    await ds.writeRaw('primeiro');

    await ds.writeRaw('segundo');

    expect(ds.readRaw(), 'segundo');
  });

  test('clearRaw apaga o conteúdo salvo', () async {
    final ds = LastDongleDatasource(await SharedPreferences.getInstance());
    await ds.writeRaw('{"id":"AA:BB","name":"OBD2Dongle"}');

    await ds.clearRaw();

    expect(ds.readRaw(), isNull);
  });

  test('clearRaw sem nada salvo não lança', () async {
    final ds = LastDongleDatasource(await SharedPreferences.getInstance());

    await expectLater(ds.clearRaw(), completes);
  });
}
