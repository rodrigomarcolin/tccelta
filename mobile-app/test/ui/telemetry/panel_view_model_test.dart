import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/telemetry/indicator_display.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/panel_view_model.dart';

void main() {
  late ProviderContainer container;

  setUp(() {
    container = ProviderContainer();
    addTearDown(container.dispose);
  });

  PanelViewModel notifier() => container.read(panelViewModelProvider.notifier);
  PanelState state() => container.read(panelViewModelProvider);
  IndicatorDisplay display(Obd2Pid pid) => IndicatorDisplay.defaultFor(pid);

  test('painel começa vazio', () {
    expect(state().indicators, isEmpty);
  });

  test('addIndicator adiciona no fim, na ordem de inclusão', () {
    notifier()
      ..addIndicator(Obd2Pid.rpm, display(Obd2Pid.rpm))
      ..addIndicator(Obd2Pid.speed, display(Obd2Pid.speed));

    expect(state().indicators, [Obd2Pid.rpm, Obd2Pid.speed]);
  });

  test('addIndicator é idempotente na ordem (já adicionado não duplica)', () {
    notifier()
      ..addIndicator(Obd2Pid.rpm, display(Obd2Pid.rpm))
      ..addIndicator(Obd2Pid.rpm, display(Obd2Pid.rpm));

    expect(state().indicators, [Obd2Pid.rpm]);
  });

  test('addIndicator grava a customização de exibição do indicador', () {
    final rpmGauge = display(
      Obd2Pid.rpm,
    ).copyWith(format: IndicatorFormat.gauge);

    notifier().addIndicator(Obd2Pid.rpm, rpmGauge);

    expect(state().displayFor(Obd2Pid.rpm), rpmGauge);
  });

  test(
    'addIndicator sobre um indicador já presente sobrescreve o display sem '
    'duplicar a ordem',
    () {
      notifier()
        ..addIndicator(Obd2Pid.rpm, display(Obd2Pid.rpm))
        ..addIndicator(
          Obd2Pid.rpm,
          display(Obd2Pid.rpm).copyWith(format: IndicatorFormat.bar),
        );

      expect(state().indicators, [Obd2Pid.rpm]);
      expect(state().displayFor(Obd2Pid.rpm).format, IndicatorFormat.bar);
    },
  );

  test(
    'displayFor cai nos defaults do PID quando não há customização salva',
    () {
      expect(
        state().displayFor(Obd2Pid.rpm),
        IndicatorDisplay.defaultFor(Obd2Pid.rpm),
      );
    },
  );

  test('updateDisplay edita a customização sem alterar a ordem', () {
    notifier()
      ..addIndicator(Obd2Pid.rpm, display(Obd2Pid.rpm))
      ..addIndicator(Obd2Pid.speed, display(Obd2Pid.speed))
      ..updateDisplay(
        Obd2Pid.rpm,
        display(Obd2Pid.rpm).copyWith(format: IndicatorFormat.history),
      );

    expect(state().indicators, [Obd2Pid.rpm, Obd2Pid.speed]);
    expect(state().displayFor(Obd2Pid.rpm).format, IndicatorFormat.history);
  });

  test('updateDisplay sem efeito quando o indicador não está presente', () {
    notifier().updateDisplay(
      Obd2Pid.rpm,
      display(Obd2Pid.rpm).copyWith(format: IndicatorFormat.history),
    );

    expect(state().indicators, isEmpty);
    expect(state().displays, isEmpty);
  });

  test('removeIndicator tira o indicador da lista', () {
    notifier()
      ..addIndicator(Obd2Pid.rpm, display(Obd2Pid.rpm))
      ..addIndicator(Obd2Pid.speed, display(Obd2Pid.speed))
      ..removeIndicator(Obd2Pid.rpm);

    expect(state().indicators, [Obd2Pid.speed]);
  });

  test('removeIndicator também limpa o display salvo', () {
    notifier()
      ..addIndicator(Obd2Pid.rpm, display(Obd2Pid.rpm))
      ..removeIndicator(Obd2Pid.rpm);

    expect(state().displays.containsKey(Obd2Pid.rpm.name), isFalse);
  });

  test('removeIndicator sem efeito quando não está presente', () {
    notifier().addIndicator(Obd2Pid.rpm, display(Obd2Pid.rpm));

    notifier().removeIndicator(Obd2Pid.speed);

    expect(state().indicators, [Obd2Pid.rpm]);
  });

  test('reorder move o item para frente (newIndex > oldIndex)', () {
    notifier()
      ..addIndicator(Obd2Pid.rpm, display(Obd2Pid.rpm))
      ..addIndicator(Obd2Pid.speed, display(Obd2Pid.speed))
      ..addIndicator(Obd2Pid.coolantTemp, display(Obd2Pid.coolantTemp))
      ..reorder(0, 2); // arrasta rpm para o lugar de coolantTemp

    expect(state().indicators, [
      Obd2Pid.speed,
      Obd2Pid.coolantTemp,
      Obd2Pid.rpm,
    ]);
  });

  test('reorder move o item para trás (newIndex < oldIndex)', () {
    notifier()
      ..addIndicator(Obd2Pid.rpm, display(Obd2Pid.rpm))
      ..addIndicator(Obd2Pid.speed, display(Obd2Pid.speed))
      ..addIndicator(Obd2Pid.coolantTemp, display(Obd2Pid.coolantTemp))
      ..reorder(2, 0);

    expect(state().indicators, [
      Obd2Pid.coolantTemp,
      Obd2Pid.rpm,
      Obd2Pid.speed,
    ]);
  });
}
