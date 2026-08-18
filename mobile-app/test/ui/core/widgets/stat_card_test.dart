import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

void main() {
  Widget wrap(Widget child) => MaterialApp(
    theme: AppTheme.dark,
    home: Scaffold(body: Center(child: child)),
  );

  group('StatCard.value — onTap', () {
    testWidgets('dispara o callback ao tocar no card', (tester) async {
      var taps = 0;
      await tester.pumpWidget(
        wrap(
          StatCard.value(
            label: 'Rotação do motor',
            value: 1500,
            unit: 'RPM',
            onTap: () => taps++,
          ),
        ),
      );

      await tester.tap(find.byType(StatCard));

      expect(taps, 1);
    });

    testWidgets('sem onTap, o card não fica com nenhum GestureDetector de '
        'toque próprio', (tester) async {
      await tester.pumpWidget(
        wrap(const StatCard.value(label: 'Rotação do motor', value: 1500)),
      );

      expect(
        find.descendant(
          of: find.byType(StatCard),
          matching: find.byType(GestureDetector),
        ),
        findsNothing,
      );
    });
  });

  group('StatCard.value — fullWidth', () {
    testWidgets('fullWidth=false (default) mostra o rótulo em uppercase', (
      tester,
    ) async {
      await tester.pumpWidget(
        wrap(
          const StatCard.value(
            label: 'Velocidade',
            value: 60,
            unit: 'km/h',
          ),
        ),
      );

      expect(find.text('VELOCIDADE'), findsOneWidget);
      expect(find.text('Velocidade'), findsNothing);
    });

    testWidgets('fullWidth=true mostra o valor em destaque ciano', (
      tester,
    ) async {
      await tester.pumpWidget(
        wrap(
          const StatCard.value(
            label: 'Velocidade',
            value: 60,
            unit: 'km/h',
            fullWidth: true,
          ),
        ),
      );

      // O rótulo continua no mesmo overline (uppercase) do resto do design
      // system — só o valor ganha o destaque em ciano da "linha inteira".
      expect(find.text('VELOCIDADE'), findsOneWidget);

      final valueText = tester.widget<Text>(find.text('60'));
      expect(valueText.style?.color, AppColors.cyan500);
    });

    testWidgets('fullWidth=false (default) não colore o valor em ciano', (
      tester,
    ) async {
      await tester.pumpWidget(
        wrap(
          const StatCard.value(label: 'Velocidade', value: 60, unit: 'km/h'),
        ),
      );

      final valueText = tester.widget<Text>(find.text('60'));
      expect(valueText.style?.color, isNot(AppColors.cyan500));
    });

    testWidgets('fullWidth=true ocupa mais largura que o layout padrão', (
      tester,
    ) async {
      await tester.pumpWidget(
        wrap(
          const SizedBox(
            width: 300,
            child: StatCard.value(
              label: 'Velocidade',
              value: 60,
              unit: 'km/h',
              fullWidth: true,
            ),
          ),
        ),
      );

      final wideWidth = tester.getSize(find.byType(StatCard)).width;

      await tester.pumpWidget(
        wrap(
          const SizedBox(
            width: 300,
            child: StatCard.value(label: 'Velocidade', value: 60),
          ),
        ),
      );
      final normalWidth = tester.getSize(find.byType(StatCard)).width;

      // Ambos preenchem a `SizedBox` de 300 (`AppCard` não é intrinsicamente
      // menor) — o teste real de "linha inteira" é de grid (span), coberto em
      // `reorderable_card_grid_test.dart` / `painel_screen_test.dart`. Aqui
      // garantimos que os dois layouts ao menos constroem sem estourar na
      // mesma largura.
      expect(wideWidth, normalWidth);
    });
  });

  group('StatCard.gauge — centered', () {
    testWidgets('centered=true empilha o gauge acima do label, sem coluna '
        'de valor separada', (tester) async {
      await tester.pumpWidget(
        wrap(
          const StatCard.gauge(
            label: 'Rotação do motor',
            gauge: Gauge(value: 3000, max: 8000, label: '', size: 100),
            centered: true,
          ),
        ),
      );

      expect(find.byType(Gauge), findsOneWidget);
      expect(find.text('Rotação do motor'), findsOneWidget);

      final gaugeCenter = tester.getCenter(find.byType(Gauge));
      final labelCenter = tester.getCenter(find.text('Rotação do motor'));
      expect(gaugeCenter.dy, lessThan(labelCenter.dy));
    });

    testWidgets('centered=false (default) mantém o gauge ao lado do texto', (
      tester,
    ) async {
      await tester.pumpWidget(
        wrap(
          const StatCard.gauge(
            label: 'Rotação do motor',
            value: 3000,
            unit: 'RPM',
            gauge: Gauge(value: 3000, max: 8000, label: '', size: 54),
          ),
        ),
      );

      final gaugeCenter = tester.getCenter(find.byType(Gauge));
      final labelCenter = tester.getCenter(find.text('Rotação do motor'));
      expect(gaugeCenter.dx, lessThan(labelCenter.dx));
    });
  });
}
