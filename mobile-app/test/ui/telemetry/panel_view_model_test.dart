import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/panel_view_model.dart';

void main() {
  late ProviderContainer container;

  setUp(() {
    container = ProviderContainer();
    addTearDown(container.dispose);
  });

  PanelViewModel notifier() => container.read(panelViewModelProvider.notifier);
  PanelState state() => container.read(panelViewModelProvider);

  test('painel começa vazio', () {
    expect(state().indicators, isEmpty);
  });

  test('addIndicator adiciona no fim, na ordem de inclusão', () {
    notifier()
      ..addIndicator(Obd2Pid.rpm)
      ..addIndicator(Obd2Pid.speed);

    expect(state().indicators, [Obd2Pid.rpm, Obd2Pid.speed]);
  });

  test('addIndicator é idempotente (já adicionado não duplica)', () {
    notifier()
      ..addIndicator(Obd2Pid.rpm)
      ..addIndicator(Obd2Pid.rpm);

    expect(state().indicators, [Obd2Pid.rpm]);
  });

  test('removeIndicator tira o indicador da lista', () {
    notifier()
      ..addIndicator(Obd2Pid.rpm)
      ..addIndicator(Obd2Pid.speed)
      ..removeIndicator(Obd2Pid.rpm);

    expect(state().indicators, [Obd2Pid.speed]);
  });

  test('removeIndicator sem efeito quando não está presente', () {
    notifier().addIndicator(Obd2Pid.rpm);

    notifier().removeIndicator(Obd2Pid.speed);

    expect(state().indicators, [Obd2Pid.rpm]);
  });

  test('reorder move o item para frente (newIndex > oldIndex)', () {
    notifier()
      ..addIndicator(Obd2Pid.rpm)
      ..addIndicator(Obd2Pid.speed)
      ..addIndicator(Obd2Pid.coolantTemp)
      ..reorder(0, 2); // arrasta rpm para o lugar de coolantTemp

    expect(state().indicators, [
      Obd2Pid.speed,
      Obd2Pid.coolantTemp,
      Obd2Pid.rpm,
    ]);
  });

  test('reorder move o item para trás (newIndex < oldIndex)', () {
    notifier()
      ..addIndicator(Obd2Pid.rpm)
      ..addIndicator(Obd2Pid.speed)
      ..addIndicator(Obd2Pid.coolantTemp)
      ..reorder(2, 0);

    expect(state().indicators, [
      Obd2Pid.coolantTemp,
      Obd2Pid.rpm,
      Obd2Pid.speed,
    ]);
  });
}
