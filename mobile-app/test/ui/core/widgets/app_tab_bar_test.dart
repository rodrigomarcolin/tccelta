import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

void main() {
  Widget wrap({
    String active = 'painel',
    List<AppTab> tabs = AppTabBar.defaultTabs,
    ValueChanged<String>? onChanged,
  }) => MaterialApp(
    theme: AppTheme.dark,
    home: Scaffold(
      bottomNavigationBar: AppTabBar(
        active: active,
        tabs: tabs,
        onChanged: onChanged,
      ),
    ),
  );

  testWidgets('mostra as quatro abas padrão', (tester) async {
    await tester.pumpWidget(wrap());

    expect(find.text('Painel'), findsOneWidget);
    expect(find.text('Sensores'), findsOneWidget);
    expect(find.text('DTCs'), findsOneWidget);
    expect(find.text('Mais'), findsOneWidget);
  });

  testWidgets('tocar numa aba dispara onChanged com sua key', (tester) async {
    String? tapped;
    await tester.pumpWidget(wrap(onChanged: (key) => tapped = key));

    await tester.tap(find.text('DTCs'));
    await tester.pump();

    expect(tapped, 'dtc');
  });

  testWidgets('badgeCount > 0 mostra o círculo de contagem sobre o ícone', (
    tester,
  ) async {
    await tester.pumpWidget(
      wrap(
        tabs: const [
          AppTab(key: 'painel', label: 'Painel', icon: AppIconData.painel),
          AppTab(
            key: 'dtc',
            label: 'DTCs',
            icon: AppIconData.motor,
            badgeCount: 3,
          ),
        ],
      ),
    );

    expect(find.text('3'), findsOneWidget);
  });

  testWidgets('badgeCount 0/nulo não mostra o círculo de contagem', (
    tester,
  ) async {
    await tester.pumpWidget(wrap());

    expect(find.text('0'), findsNothing);
  });

  testWidgets(
    'o badge sobreposto não desalinha os ícones das abas na mesma linha',
    (tester) async {
      await tester.pumpWidget(
        wrap(
          tabs: const [
            AppTab(key: 'painel', label: 'Painel', icon: AppIconData.painel),
            AppTab(
              key: 'sensores',
              label: 'Sensores',
              icon: AppIconData.sensores,
            ),
            AppTab(
              key: 'dtc',
              label: 'DTCs',
              icon: AppIconData.motor,
              badgeCount: 3,
            ),
            AppTab(key: 'mais', label: 'Mais', icon: AppIconData.mais),
          ],
        ),
      );

      final icons = tester.widgetList<AppIcon>(find.byType(AppIcon)).toList();
      expect(icons, hasLength(4));
      final tops = icons
          .map((icon) => tester.getTopLeft(find.byWidget(icon)).dy)
          .toSet();
      expect(tops, hasLength(1)); // todos na mesma linha
      expect(tester.takeException(), isNull);
    },
  );
}
