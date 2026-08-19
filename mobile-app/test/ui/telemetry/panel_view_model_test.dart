import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/telemetry/indicator_display.dart';
import 'package:tccelta_mobile/src/domain/telemetry/panel.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/panel_view_model.dart';

void main() {
  late ProviderContainer container;

  setUp(() {
    container = ProviderContainer();
    addTearDown(container.dispose);
  });

  PanelViewModel notifier() => container.read(panelViewModelProvider.notifier);
  PanelsState state() => container.read(panelViewModelProvider);
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

  group('gerenciamento de painéis', () {
    test('começa com um único painel ativo, vazio', () {
      expect(state().panels, hasLength(1));
      expect(state().active.name, 'Painel 1');
      expect(state().activeId, state().active.id);
    });

    test('createPanel adiciona um painel vazio numerado e o torna ativo', () {
      notifier().createPanel();

      expect(state().panels, hasLength(2));
      expect(state().active.name, 'Painel 2');
      expect(state().active.indicators, isEmpty);
    });

    test('addIndicator só afeta o painel ativo', () {
      final firstId = state().activeId;
      notifier()
        ..addIndicator(Obd2Pid.rpm, display(Obd2Pid.rpm))
        ..createPanel()
        ..addIndicator(Obd2Pid.speed, display(Obd2Pid.speed));

      final first = state().panels.firstWhere((p) => p.id == firstId);
      expect(first.indicators, [Obd2Pid.rpm]);
      expect(state().active.indicators, [Obd2Pid.speed]);
    });

    test(
      'duplicatePanel clona indicadores/displays num painel "(cópia)" e o '
      'torna ativo',
      () {
        final originalId = state().activeId;
        notifier()
          ..addIndicator(Obd2Pid.rpm, display(Obd2Pid.rpm))
          ..duplicatePanel(originalId);

        expect(state().panels, hasLength(2));
        expect(state().active.name, 'Painel 1 (cópia)');
        expect(state().active.indicators, [Obd2Pid.rpm]);
        expect(state().active.id, isNot(originalId));
        // O original continua intacto.
        final original = state().panels.firstWhere((p) => p.id == originalId);
        expect(original.indicators, [Obd2Pid.rpm]);
      },
    );

    test('renamePanel muda só o nome do painel indicado', () {
      final id = state().activeId;

      notifier().renamePanel(id, 'Pista');

      expect(state().active.name, 'Pista');
    });

    test(
      'deletePanel remove o painel e reatribui o ativo se era o excluído',
      () {
        final firstId = state().activeId;
        notifier().createPanel();
        final secondId = state().activeId;

        notifier().deletePanel(secondId);

        expect(state().panels, hasLength(1));
        expect(state().activeId, firstId);
      },
    );

    test('deletePanel sem efeito quando só resta 1 painel', () {
      final onlyId = state().activeId;

      notifier().deletePanel(onlyId);

      expect(state().panels, hasLength(1));
      expect(state().activeId, onlyId);
    });

    test('deletePanel de um painel inativo não muda o ativo', () {
      final firstId = state().activeId;
      notifier().createPanel();
      final activeId = state().activeId;

      notifier().deletePanel(firstId);

      expect(state().panels, hasLength(1));
      expect(state().activeId, activeId);
    });

    test('switchPanel troca o painel ativo', () {
      final firstId = state().activeId;
      notifier().createPanel();
      final secondId = state().activeId;

      notifier().switchPanel(firstId);

      expect(state().activeId, firstId);
      expect(secondId, isNot(firstId));
    });

    test('switchPanel sem efeito para um id inexistente', () {
      final id = state().activeId;

      notifier().switchPanel('nao-existe');

      expect(state().activeId, id);
    });

    test('renamePanel corta o nome em PanelNameLimits.max caracteres', () {
      final id = state().activeId;
      final long = 'x' * (PanelNameLimits.max + 10);

      notifier().renamePanel(id, long);

      expect(state().active.name, 'x' * PanelNameLimits.max);
    });

    test(
      'duplicatePanel corta o nome "(cópia)" quando ultrapassa o limite',
      () {
        final id = state().activeId;
        notifier().renamePanel(id, 'x' * PanelNameLimits.max);

        notifier().duplicatePanel(id);

        expect(state().active.name.length, PanelNameLimits.max);
      },
    );
  });

  group('operações sobre um painel específico (*To/*In/*From)', () {
    test(
      'addIndicatorTo adiciona num painel não-ativo, sem afetar o ativo',
      () {
        final firstId = state().activeId;
        notifier().createPanel();
        final activeId = state().activeId;

        notifier().addIndicatorTo(
          firstId,
          Obd2Pid.rpm,
          display(Obd2Pid.rpm),
        );

        final first = state().panels.firstWhere((p) => p.id == firstId);
        expect(first.indicators, [Obd2Pid.rpm]);
        expect(state().activeId, activeId); // não muda o ativo
        expect(state().active.indicators, isEmpty);
      },
    );

    test(
      'addIndicator (painel ativo) equivale a addIndicatorTo(activeId)',
      () {
        final id = state().activeId;

        notifier().addIndicatorTo(id, Obd2Pid.rpm, display(Obd2Pid.rpm));

        expect(state().indicators, [Obd2Pid.rpm]);
      },
    );

    test('updateDisplayIn edita a customização só no painel indicado', () {
      final firstId = state().activeId;
      notifier()
        ..addIndicatorTo(firstId, Obd2Pid.rpm, display(Obd2Pid.rpm))
        ..createPanel()
        ..addIndicatorTo(state().activeId, Obd2Pid.rpm, display(Obd2Pid.rpm));
      final secondId = state().activeId;

      notifier().updateDisplayIn(
        firstId,
        Obd2Pid.rpm,
        display(Obd2Pid.rpm).copyWith(format: IndicatorFormat.gauge),
      );

      final first = state().panels.firstWhere((p) => p.id == firstId);
      final second = state().panels.firstWhere((p) => p.id == secondId);
      expect(first.displayFor(Obd2Pid.rpm).format, IndicatorFormat.gauge);
      expect(second.displayFor(Obd2Pid.rpm).format, IndicatorFormat.number);
    });

    test(
      'updateDisplayIn sem efeito quando o indicador não está no painel '
      'indicado',
      () {
        final id = state().activeId;

        notifier().updateDisplayIn(
          id,
          Obd2Pid.rpm,
          display(Obd2Pid.rpm).copyWith(format: IndicatorFormat.gauge),
        );

        expect(state().indicators, isEmpty);
      },
    );

    test('removeIndicatorFrom tira o indicador só do painel indicado', () {
      final firstId = state().activeId;
      notifier()
        ..addIndicatorTo(firstId, Obd2Pid.rpm, display(Obd2Pid.rpm))
        ..createPanel()
        ..addIndicatorTo(state().activeId, Obd2Pid.rpm, display(Obd2Pid.rpm));
      final secondId = state().activeId;

      notifier().removeIndicatorFrom(firstId, Obd2Pid.rpm);

      final first = state().panels.firstWhere((p) => p.id == firstId);
      final second = state().panels.firstWhere((p) => p.id == secondId);
      expect(first.indicators, isEmpty);
      expect(second.indicators, [Obd2Pid.rpm]);
    });

    test('*To/*In/*From sem efeito para um panelId inexistente', () {
      notifier().addIndicatorTo(
        'nao-existe',
        Obd2Pid.rpm,
        display(Obd2Pid.rpm),
      );

      expect(state().panels, hasLength(1));
      expect(state().indicators, isEmpty);
    });
  });
}
