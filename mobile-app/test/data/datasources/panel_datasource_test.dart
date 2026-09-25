import 'package:flutter_test/flutter_test.dart';
import 'package:shared_preferences/shared_preferences.dart';
import 'package:tccelta_mobile/src/data/datasources/panel_datasource.dart';

void main() {
  setUp(() {
    SharedPreferences.setMockInitialValues({});
  });

  test('readRaw devolve null quando nada foi salvo ainda', () async {
    final ds = PanelDatasource(await SharedPreferences.getInstance());

    expect(ds.readRaw(), isNull);
  });

  test('writeRaw seguido de readRaw devolve o mesmo conteúdo', () async {
    final ds = PanelDatasource(await SharedPreferences.getInstance());

    await ds.writeRaw('{"panels":[],"activeId":"p1"}');

    expect(ds.readRaw(), '{"panels":[],"activeId":"p1"}');
  });

  test('writeRaw sobrescreve o conteúdo anterior', () async {
    final ds = PanelDatasource(await SharedPreferences.getInstance());
    await ds.writeRaw('primeiro');

    await ds.writeRaw('segundo');

    expect(ds.readRaw(), 'segundo');
  });
}
