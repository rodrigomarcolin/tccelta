import 'package:flutter/material.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/main.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/showcase/view/showcase_screen.dart';

void main() {
  testWidgets('app entra pelo fluxo de conexão (permissões)', (tester) async {
    await tester.pumpWidget(const TcceltaApp());
    await tester.pump(const Duration(milliseconds: 100));
    expect(find.text('Permitir Bluetooth'), findsOneWidget);
  });

  testWidgets('showcase builds the design system gallery', (tester) async {
    await tester.pumpWidget(
      MaterialApp(theme: AppTheme.dark, home: const ShowcaseScreen()),
    );
    await tester.pump(const Duration(milliseconds: 100));
    expect(find.text('Design System'), findsOneWidget);
  });

  testWidgets('design system atoms build without error', (tester) async {
    await tester.pumpWidget(
      MaterialApp(
        home: Scaffold(
          body: ListView(
            children: [
              AppButton(onPressed: () {}, child: const Text('Permitir')),
              const StatusBadge(label: 'AO VIVO', pulse: true),
              const Gauge(value: 5200),
              const Gauge(
                value: 89,
                max: 120,
                label: '',
                unit: '°',
                variant: GaugeVariant.arc180,
              ),
              const StatCard.progress(
                label: 'Carga',
                value: 34,
                unit: '%',
                pct: 34,
              ),
              const CardButton(
                icon: AppIconData.estrela,
                title: 'Rotação do motor',
                subtitle: '01 0C',
                value: 5200,
                unit: 'rpm',
                iconColor: AppColors.cyan500,
              ),
              const AppTabBar(),
            ],
          ),
        ),
      ),
    );
    // Uma única pump animada para acomodar TweenAnimationBuilder/pulse.
    await tester.pump(const Duration(milliseconds: 100));

    expect(find.byType(Gauge), findsNWidgets(2));
    expect(find.byType(CardButton), findsOneWidget);
    expect(find.byType(AppTabBar), findsOneWidget);
  });

  testWidgets('StatCard.gauge não estoura em largura apertada', (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: Center(
            child: SizedBox(
              width: 140,
              child: StatCard.gauge(
                label: 'Temperatura de arrefecimento do motor',
                value: 999999,
                unit: 'km/h',
                gauge: Gauge(value: 86, max: 130, label: '', size: 64),
              ),
            ),
          ),
        ),
      ),
    );
    await tester.pump(const Duration(milliseconds: 100));
    // Overflow vira um FlutterError pego pelo binding; takeException() o expõe.
    expect(tester.takeException(), isNull);
  });
}
