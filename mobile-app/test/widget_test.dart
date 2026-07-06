import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/main.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_adapter_info.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_reading.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/permissions_repository.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view/painel_screen.dart';

/// Repository de permissões fake para os testes de widget: controla se a
/// permissão já foi concedida sem tocar no plugin real.
class _FakePermissionsRepository implements PermissionsRepository {
  @override
  Future<bool> hasBluetoothPermission() async => false;

  @override
  Future<bool> requestBluetoothPermission() async => false;
}

/// Valores de exemplo da documentação, indexados por PID.
const Map<Obd2Pid, double> _exampleValues = {
  Obd2Pid.engineLoad: 40,
  Obd2Pid.coolantTemp: 90,
  Obd2Pid.rpm: 1500,
  Obd2Pid.speed: 60,
  Obd2Pid.timingAdvance: 10,
  Obd2Pid.throttle: 20,
};

/// Repository de telemetria fake: devolve os valores de exemplo sem tocar no
/// transporte BLE.
class _FakeObd2Repository implements Obd2Repository {
  @override
  List<Obd2Pid> get pids => Obd2Pid.values;

  @override
  Obd2AdapterInfo? get adapterInfo => null;

  @override
  Future<void> initialize() async {}

  @override
  Future<Obd2Reading> read(Obd2Pid pid) async =>
      Obd2Reading(pid: pid, value: _exampleValues[pid]!);

  @override
  Future<List<Obd2Reading>> readAll() async => [
        for (final pid in Obd2Pid.values)
          Obd2Reading(pid: pid, value: _exampleValues[pid]!),
      ];
}

void main() {
  testWidgets('app entra pelo fluxo de conexão (permissões)', (tester) async {
    // As telas leem providers Riverpod, então precisam de um ProviderScope.
    // Sem permissão concedida => a tela de permissão deve aparecer.
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          permissionsRepositoryProvider
              .overrideWithValue(_FakePermissionsRepository()),
        ],
        child: const TcceltaApp(),
      ),
    );
    await tester.pump(const Duration(milliseconds: 100));
    expect(find.text('Permitir Bluetooth'), findsOneWidget);
  });

  testWidgets('painel mostra os valores de exemplo dos PIDs', (tester) async {
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          obd2RepositoryProvider.overrideWithValue(_FakeObd2Repository()),
        ],
        child: MaterialApp(theme: AppTheme.dark, home: const PainelScreen()),
      ),
    );
    // Deixa o primeiro ciclo de leitura resolver e popular o estado.
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 50));

    // "Painel" aparece no título e na tab bar — basta existir.
    expect(find.text('Painel'), findsWidgets);
    expect(find.text('1500'), findsOneWidget); // RPM
    expect(find.text('60'), findsOneWidget); // velocidade
    expect(find.text('90'), findsOneWidget); // temp. do líquido
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
