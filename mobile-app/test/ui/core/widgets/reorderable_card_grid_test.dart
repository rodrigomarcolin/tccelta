import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

void main() {
  const gridWidth = 320.0;

  Widget wrap({
    required List<int> items,
    int Function(int item)? spanOf,
    double Function(int item)? minHeightOf,
  }) => MaterialApp(
    theme: AppTheme.dark,
    home: Scaffold(
      body: SizedBox(
        width: gridWidth,
        child: ReorderableCardGrid<int>(
          items: items,
          keyOf: ValueKey.new,
          onReorder: (_, _) {},
          spanOf: spanOf,
          minHeightOf: minHeightOf,
          itemBuilder: (context, item, index) => ColoredBox(
            key: ValueKey('cell_$item'),
            color: Colors.blueGrey,
            child: Text('item $item'),
          ),
        ),
      ),
    ),
  );

  double cellWidth(WidgetTester tester, int item) =>
      tester.getSize(find.byKey(ValueKey('cell_$item'))).width;

  double cellHeight(WidgetTester tester, int item) =>
      tester.getSize(find.byKey(ValueKey('cell_$item'))).height;

  testWidgets('sem spanOf, todo item ocupa meia largura (2 colunas)', (
    tester,
  ) async {
    await tester.pumpWidget(wrap(items: const [1, 2]));

    const gap = AppSpacing.s3;
    const expectedWidth = (gridWidth - gap) / 2;
    expect(cellWidth(tester, 1), closeTo(expectedWidth, 0.5));
    expect(cellWidth(tester, 2), closeTo(expectedWidth, 0.5));
  });

  testWidgets('item com spanOf == 2 ocupa a largura inteira do grid', (
    tester,
  ) async {
    await tester.pumpWidget(
      wrap(items: const [1, 2, 3], spanOf: (item) => item == 2 ? 2 : 1),
    );

    const gap = AppSpacing.s3;
    const halfWidth = (gridWidth - gap) / 2;
    expect(cellWidth(tester, 1), closeTo(halfWidth, 0.5));
    expect(cellWidth(tester, 2), closeTo(gridWidth, 0.5));
    expect(cellWidth(tester, 3), closeTo(halfWidth, 0.5));
  });

  testWidgets(
    'item de largura inteira força quebra de linha antes e depois de si',
    (tester) async {
      await tester.pumpWidget(
        wrap(items: const [1, 2, 3], spanOf: (item) => item == 2 ? 2 : 1),
      );

      // 1 e 2 não cabem na mesma linha (2 é largura inteira) -> 2 fica abaixo
      // de 1; 3 fica abaixo de 2, não ao lado.
      final topOf1 = tester.getTopLeft(find.byKey(const ValueKey('cell_1'))).dy;
      final topOf2 = tester.getTopLeft(find.byKey(const ValueKey('cell_2'))).dy;
      final topOf3 = tester.getTopLeft(find.byKey(const ValueKey('cell_3'))).dy;

      expect(topOf2, greaterThan(topOf1));
      expect(topOf3, greaterThan(topOf2));
    },
  );

  testWidgets(
    'itens da mesma linha esticam para a mesma altura (a maior '
    'minHeightOf entre eles) — cada card preenche o espaço que o grid lhe '
    'reservou',
    (tester) async {
      await tester.pumpWidget(
        wrap(
          items: const [1, 2],
          minHeightOf: (item) => item == 2 ? 220 : 86,
        ),
      );

      expect(cellHeight(tester, 1), closeTo(220, 0.5));
      expect(cellHeight(tester, 2), closeTo(220, 0.5));
    },
  );

  testWidgets(
    'itens de linhas diferentes (largura inteira) não esticam entre si',
    (
      tester,
    ) async {
      await tester.pumpWidget(
        wrap(
          items: const [1, 2],
          spanOf: (item) => 2,
          minHeightOf: (item) => item == 2 ? 220 : 86,
        ),
      );

      expect(cellHeight(tester, 1), closeTo(86, 0.5));
      expect(cellHeight(tester, 2), closeTo(220, 0.5));
    },
  );

  testWidgets('sem minHeightOf, a altura mínima default é 86', (tester) async {
    await tester.pumpWidget(wrap(items: const [1]));

    expect(cellHeight(tester, 1), closeTo(86, 0.5));
  });
}
