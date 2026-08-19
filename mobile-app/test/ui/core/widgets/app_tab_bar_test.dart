import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

void main() {
  Widget wrap({String active = 'painel', ValueChanged<String>? onChanged}) =>
      MaterialApp(
        theme: AppTheme.dark,
        home: Scaffold(
          bottomNavigationBar: AppTabBar(active: active, onChanged: onChanged),
        ),
      );

  testWidgets('mostra as quatro abas padrão', (tester) async {
    await tester.pumpWidget(wrap());

    expect(find.text('Painel'), findsOneWidget);
    expect(find.text('Sensores'), findsOneWidget);
    expect(find.text('Terminal'), findsOneWidget);
    expect(find.text('Mais'), findsOneWidget);
  });

  testWidgets(
    '"Terminal" mostra a legenda "EM BREVE", sem estourar o layout',
    (tester) async {
      await tester.pumpWidget(wrap());

      expect(find.text('EM BREVE'), findsOneWidget);
      expect(tester.takeException(), isNull);
    },
  );

  testWidgets(
    '"Terminal" nunca fica ciano (comingSoon), mesmo se "active" apontar '
    'pra ela',
    (tester) async {
      await tester.pumpWidget(wrap(active: 'terminal'));

      final icon = tester.widget<AppIcon>(
        find.descendant(
          of: find.ancestor(
            of: find.text('Terminal'),
            matching: find.byType(Column),
          ),
          matching: find.byType(AppIcon),
        ),
      );
      expect(icon.color, AppColors.neutral400);
    },
  );

  testWidgets('tocar em "Terminal" ainda dispara onChanged com sua key', (
    tester,
  ) async {
    String? tapped;
    await tester.pumpWidget(wrap(onChanged: (key) => tapped = key));

    await tester.tap(find.text('Terminal'));
    await tester.pump();

    expect(tapped, 'terminal');
  });
}
