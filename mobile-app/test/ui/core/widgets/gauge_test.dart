import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/gauge/gauge_painter.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

void main() {
  Widget wrap(Widget child) => MaterialApp(
    theme: AppTheme.dark,
    home: Scaffold(body: Center(child: child)),
  );

  GaugePainter painterOf(WidgetTester tester) => tester
      .widgetList<CustomPaint>(find.byType(CustomPaint))
      .map((w) => w.painter)
      .whereType<GaugePainter>()
      .single;

  testWidgets('com min negativo, valor abaixo de 0 preenche parte do arco', (
    tester,
  ) async {
    await tester.pumpWidget(
      wrap(const Gauge(value: -10, min: -25, max: 25, label: '')),
    );
    await tester.pump(const Duration(milliseconds: 400));

    // (value - min) / (max - min) = 15 / 50 = 0.3
    expect(painterOf(tester).pct, closeTo(0.3, 0.01));
  });

  testWidgets('sem min informado, comportamento antigo (0..max) é mantido', (
    tester,
  ) async {
    await tester.pumpWidget(wrap(const Gauge(value: 4000, label: '')));
    await tester.pump(const Duration(milliseconds: 400));

    expect(painterOf(tester).pct, closeTo(0.5, 0.01));
  });
}
