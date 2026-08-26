import 'dart:async';

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
import 'package:tccelta_mobile/src/domain/telemetry/indicator_display.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view/painel_screen.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/panel_view_model.dart';

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
  Obd2Pid.shortFuelTrim1: 4.6875,
  Obd2Pid.longFuelTrim1: 4.6875,
  Obd2Pid.fuelPressureGauge: 399,
  Obd2Pid.intakeManifoldPressure: 120,
  Obd2Pid.intakeAirTemp: 50,
  Obd2Pid.maf: 40,
  Obd2Pid.o2Sensor1Voltage: 0.7,
  Obd2Pid.o2Sensor2Voltage: 0.7,
  Obd2Pid.engineRunTime: 2400,
  Obd2Pid.distanceWithMil: 35,
  Obd2Pid.commandedEgr: 54.90196078431372,
  Obd2Pid.egrError: 3.125,
  Obd2Pid.commandedEvapPurge: 65.09803921568627,
  Obd2Pid.fuelTankLevel: 30.19607843137255,
  Obd2Pid.warmupsSinceClear: 35,
  Obd2Pid.distanceSinceClear: 750,
  Obd2Pid.evapVaporPressure: 55,
  Obd2Pid.baroPressure: 100,
  Obd2Pid.catalystTemp1: 600,
  Obd2Pid.controlModuleVoltage: 13.8,
  Obd2Pid.absoluteLoad: 90.19607843137256,
  Obd2Pid.commandedEquivRatio: 1.0,
  Obd2Pid.relativeThrottle: 54.90196078431372,
  Obd2Pid.ambientAirTemp: 38,
  Obd2Pid.throttlePositionB: 54.90196078431372,
  Obd2Pid.acceleratorPedalD: 54.90196078431372,
  Obd2Pid.commandedThrottleActuator: 54.90196078431372,
  Obd2Pid.timeMilOn: 200,
  Obd2Pid.timeSinceClear: 750,
  Obd2Pid.ethanolPercent: 27.058823529411764,
  Obd2Pid.relativeAcceleratorPedal: 54.90196078431372,
  Obd2Pid.engineOilTemp: 115,
  Obd2Pid.fuelInjectionTiming: 25,
  Obd2Pid.engineFuelRate: 25,
  Obd2Pid.driverDemandTorque: 60,
  Obd2Pid.actualEngineTorque: 55,
  Obd2Pid.engineReferenceTorque: 450,
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
  Future<Set<Obd2Pid>> discoverSupported() async => Obd2Pid.values.toSet();

  @override
  Future<Obd2Reading> read(Obd2Pid pid) async =>
      Obd2Reading(pid: pid, value: _exampleValues[pid]!);

  @override
  Future<List<Obd2Reading>> readAll() async => [
    for (final pid in Obd2Pid.values)
      Obd2Reading(pid: pid, value: _exampleValues[pid]!),
  ];

  @override
  Future<List<Obd2Reading>> readMany(List<Obd2Pid> pids) async => [
    for (final pid in pids) Obd2Reading(pid: pid, value: _exampleValues[pid]!),
  ];
}

/// Repository que trava (nunca resolve) — mantém o painel na fase inicial, para
/// exercitar os skeletons de carregamento antes da primeira leitura.
class _HangingObd2Repository implements Obd2Repository {
  final Completer<Never> _never = Completer<Never>();

  @override
  List<Obd2Pid> get pids => Obd2Pid.values;

  @override
  Obd2AdapterInfo? get adapterInfo => null;

  @override
  Future<void> initialize() async {}

  @override
  Future<Set<Obd2Pid>> discoverSupported() => _never.future;

  @override
  Future<Obd2Reading> read(Obd2Pid pid) => _never.future;

  @override
  Future<List<Obd2Reading>> readAll() => _never.future;

  @override
  Future<List<Obd2Reading>> readMany(List<Obd2Pid> pids) => _never.future;
}

void main() {
  testWidgets('app entra pelo fluxo de conexão (permissões)', (tester) async {
    // As telas leem providers Riverpod, então precisam de um ProviderScope.
    // Sem permissão concedida => a tela de permissão deve aparecer.
    await tester.pumpWidget(
      ProviderScope(
        overrides: [
          permissionsRepositoryProvider.overrideWithValue(
            _FakePermissionsRepository(),
          ),
        ],
        child: const TcceltaApp(),
      ),
    );
    await tester.pump(const Duration(milliseconds: 100));
    expect(find.text('Permitir Bluetooth'), findsOneWidget);
  });

  testWidgets('painel começa vazio e mostra os valores dos indicadores '
      'adicionados', (tester) async {
    final container = ProviderContainer(
      overrides: [
        obd2RepositoryProvider.overrideWithValue(_FakeObd2Repository()),
      ],
    );

    await tester.pumpWidget(
      UncontrolledProviderScope(
        container: container,
        child: MaterialApp(theme: AppTheme.dark, home: const PainelScreen()),
      ),
    );
    await tester.pump();
    await tester.pump(const Duration(milliseconds: 50));

    // Painel começa vazio — nenhum PID some sozinho no painel.
    expect(find.text('Painel vazio'), findsOneWidget);
    expect(find.text('1500'), findsNothing);

    // O usuário adiciona indicadores (fora do escopo desta tela é o sheet de
    // sensores — aqui exercitamos o view model diretamente).
    container.read(panelViewModelProvider.notifier)
      ..addIndicator(Obd2Pid.rpm, IndicatorDisplay.defaultFor(Obd2Pid.rpm))
      ..addIndicator(Obd2Pid.speed, IndicatorDisplay.defaultFor(Obd2Pid.speed))
      ..addIndicator(
        Obd2Pid.coolantTemp,
        IndicatorDisplay.defaultFor(Obd2Pid.coolantTemp),
      );
    await tester.pump();

    expect(find.text('1500'), findsOneWidget); // RPM
    expect(find.text('60'), findsOneWidget); // velocidade
    expect(find.text('90'), findsOneWidget); // temp. do líquido

    // Descarta a árvore antes do container: o polling do
    // `telemetryViewModelProvider` (`autoDispose`) só cancela o timer quando
    // o container é encerrado, e o binding de teste reclama de timers
    // pendentes se isso acontecer depois do fim do teste.
    await tester.pumpWidget(const SizedBox());
    container.dispose();
  });

  testWidgets(
    'indicador adicionado sem leitura ainda mostra — (sem travar a tela)',
    (tester) async {
      final container = ProviderContainer(
        overrides: [
          obd2RepositoryProvider.overrideWithValue(_HangingObd2Repository()),
        ],
      );

      await tester.pumpWidget(
        UncontrolledProviderScope(
          container: container,
          child: MaterialApp(theme: AppTheme.dark, home: const PainelScreen()),
        ),
      );
      await tester.pump();

      container
          .read(panelViewModelProvider.notifier)
          .addIndicator(Obd2Pid.rpm, IndicatorDisplay.defaultFor(Obd2Pid.rpm));
      await tester.pump();

      // Descoberta/leitura nunca resolve => sem valor ainda, mas o card
      // aparece (rótulo do PID) sem travar em skeleton.
      expect(find.text('ROTAÇÃO DO MOTOR'), findsOneWidget);
      expect(find.text('1500'), findsNothing);

      await tester.pumpWidget(const SizedBox());
      container.dispose();
    },
  );

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

  testWidgets('SpinnerRing renderiza o anel e o rótulo central', (
    tester,
  ) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: Center(
            child: Column(
              mainAxisSize: MainAxisSize.min,
              children: [
                SpinnerRing(label: 'BLE'),
                SpinnerRing(),
              ],
            ),
          ),
        ),
      ),
    );
    // Uma pump animada: o arco gira em loop (useLoopController).
    await tester.pump(const Duration(milliseconds: 100));

    expect(find.byType(SpinnerRing), findsNWidgets(2));
    // Rótulo central só aparece quando informado.
    expect(find.text('BLE'), findsOneWidget);
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

  testWidgets('StatCard.value não estoura em largura apertada (2 colunas '
      'num telefone estreito)', (tester) async {
    await tester.pumpWidget(
      const MaterialApp(
        home: Scaffold(
          body: Center(
            child: SizedBox(
              width: 141, // largura real de uma célula em grid 2 colunas
              child: StatCard.value(
                label: 'Rotação do motor',
                value: 1500,
                unit: 'RPM',
              ),
            ),
          ),
        ),
      ),
    );
    await tester.pump(const Duration(milliseconds: 100));
    expect(tester.takeException(), isNull);
  });
}
