import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

void main() {
  bool? result = false; // sentinela distinta de todos os valores possíveis
  Widget wrap({required bool destructive}) => MaterialApp(
        theme: AppTheme.dark,
        home: Scaffold(
          body: Builder(
            builder: (context) => TextButton(
              onPressed: () async {
                result = await showConfirmDialog(
                  context,
                  title: 'Remover Rotação do motor?',
                  message: 'Esse indicador vai sair do painel.',
                  confirmLabel: 'Remover',
                  destructive: destructive,
                );
              },
              child: const Text('abrir'),
            ),
          ),
        ),
      );

  setUp(() {
    result = false;
  });

  testWidgets('mostra título e mensagem', (tester) async {
    await tester.pumpWidget(wrap(destructive: true));
    await tester.tap(find.text('abrir'));
    await tester.pumpAndSettle();

    expect(find.text('Remover Rotação do motor?'), findsOneWidget);
    expect(find.text('Esse indicador vai sair do painel.'), findsOneWidget);
  });

  testWidgets('cancelar resolve false e fecha o diálogo', (tester) async {
    await tester.pumpWidget(wrap(destructive: true));
    await tester.tap(find.text('abrir'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Cancelar'));
    await tester.pumpAndSettle();

    expect(result, isFalse);
    expect(find.text('Remover Rotação do motor?'), findsNothing);
  });

  testWidgets('confirmar resolve true e fecha o diálogo', (tester) async {
    await tester.pumpWidget(wrap(destructive: true));
    await tester.tap(find.text('abrir'));
    await tester.pumpAndSettle();

    await tester.tap(find.text('Remover'));
    await tester.pumpAndSettle();

    expect(result, isTrue);
    expect(find.text('Remover Rotação do motor?'), findsNothing);
  });

  testWidgets('destructive=true usa AppButtonVariant.danger na confirmação',
      (tester) async {
    await tester.pumpWidget(wrap(destructive: true));
    await tester.tap(find.text('abrir'));
    await tester.pumpAndSettle();

    final confirmButton = tester.widget<AppButton>(
      find.ancestor(
        of: find.text('Remover'),
        matching: find.byType(AppButton),
      ),
    );
    expect(confirmButton.variant, AppButtonVariant.danger);
  });

  testWidgets(
      'destructive=false usa AppButtonVariant.primary na confirmação (ação '
      'não destrutiva)', (tester) async {
    await tester.pumpWidget(wrap(destructive: false));
    await tester.tap(find.text('abrir'));
    await tester.pumpAndSettle();

    final confirmButton = tester.widget<AppButton>(
      find.ancestor(
        of: find.text('Remover'),
        matching: find.byType(AppButton),
      ),
    );
    expect(confirmButton.variant, AppButtonVariant.primary);
  });

  testWidgets('há espaço visível entre os botões cancelar/confirmar',
      (tester) async {
    await tester.pumpWidget(wrap(destructive: true));
    await tester.tap(find.text('abrir'));
    await tester.pumpAndSettle();

    final cancelButton = find.ancestor(
      of: find.text('Cancelar'),
      matching: find.byType(AppButton),
    );
    final confirmButton = find.ancestor(
      of: find.text('Remover'),
      matching: find.byType(AppButton),
    );
    final gap = tester.getTopLeft(confirmButton).dx -
        tester.getTopRight(cancelButton).dx;

    expect(gap, greaterThanOrEqualTo(AppSpacing.s3));
  });
}
