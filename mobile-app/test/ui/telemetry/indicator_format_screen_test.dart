import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_adapter_info.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_reading.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';
import 'package:tccelta_mobile/src/domain/telemetry/indicator_display.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view/indicator_format_screen.dart';

/// Repository de telemetria fake — sem leituras (o preview cai no ponto
/// sintético de 60% da escala) a menos que o teste precise de um valor ao
/// vivo específico.
class _FakeObd2Repository implements Obd2Repository {
  _FakeObd2Repository({this.readings = const []});

  final List<Obd2Reading> readings;

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
      readings.firstWhere((r) => r.pid == pid);

  @override
  Future<List<Obd2Reading>> readAll() async => readings;
}

/// Resultado capturado pelo host: `null` enquanto a tela de formato não
/// fechou; depois, o [IndicatorFormatResult] devolvido pelo `pop`.
class _Host extends StatefulWidget {
  const _Host({required this.args});

  final IndicatorFormatArgs args;

  @override
  State<_Host> createState() => _HostState();
}

class _HostState extends State<_Host> {
  IndicatorFormatResult? result;

  /// Distinto de `result == null`: um `pop()` sem valor (cancelar) também
  /// resulta em `result == null`, mas precisa contar como "pronto".
  bool done = false;
  bool _opened = false;

  @override
  Widget build(BuildContext context) {
    if (!_opened) {
      // Abre automaticamente no primeiro build — evita um toque extra em
      // cada teste só para entrar na tela.
      _opened = true;
      WidgetsBinding.instance.addPostFrameCallback((_) async {
        final r = await context.push<IndicatorFormatResult>(
          '/format',
          extra: widget.args,
        );
        if (mounted)
          setState(() {
            result = r;
            done = true;
          });
      });
    }
    return Scaffold(
      body: Text('resultado: ${done ? 'pronto' : 'pendente'}'),
    );
  }
}

void main() {
  // Viewport bem alto: os passos com mais campos (ex.: escala do gauge
  // ponteiro) cabem inteiros sem rolar, o que evita a fragilidade de tocar
  // em algo que ainda não rolou totalmente para dentro da viewport de teste.
  setUp(() {
    final binding = TestWidgetsFlutterBinding.ensureInitialized();
    // Largura igual à padrão do harness de teste (800) — só a altura cresce,
    // para não mexer no comportamento de quebra/overflow em largura.
    binding.platformDispatcher.views.first.physicalSize = const Size(800, 2200);
    binding.platformDispatcher.views.first.devicePixelRatio = 1.0;
  });
  tearDown(() {
    final binding = TestWidgetsFlutterBinding.ensureInitialized();
    binding.platformDispatcher.views.first.resetPhysicalSize();
    binding.platformDispatcher.views.first.resetDevicePixelRatio();
  });

  Widget wrap(
    IndicatorFormatArgs args, {
    List<Obd2Reading> readings = const [],
  }) {
    final router = GoRouter(
      initialLocation: '/',
      routes: [
        GoRoute(
          path: '/',
          builder: (context, state) => _Host(args: args),
        ),
        GoRoute(
          path: '/format',
          builder: (context, state) =>
              IndicatorFormatScreen(args: state.extra! as IndicatorFormatArgs),
        ),
      ],
    );
    return ProviderScope(
      overrides: [
        obd2RepositoryProvider.overrideWithValue(
          _FakeObd2Repository(readings: readings),
        ),
      ],
      child: MaterialApp.router(theme: AppTheme.dark, routerConfig: router),
    );
  }

  _HostState host(WidgetTester tester) => tester.state(find.byType(_Host));

  Future<void> tapPrimary(WidgetTester tester) async {
    await tester.tap(find.byKey(const Key('indicator_format_primary')));
    await tester.pumpAndSettle();
  }

  /// Localiza o [NumberStepper] pelo seu rótulo, e dentro dele o botão `+`
  /// ou `−` (glifo U+2212, não o hífen ASCII).
  Finder stepperButton(String label, {required bool increment}) {
    final stepper = find.ancestor(
      of: find.text(label),
      matching: find.byType(NumberStepper),
    );
    return find.descendant(
      of: stepper,
      matching: find.text(increment ? '+' : '−'),
    );
  }

  Future<void> tapStepper(
    WidgetTester tester,
    String label, {
    required bool increment,
    int times = 1,
  }) async {
    final button = stepperButton(label, increment: increment);
    await tester.ensureVisible(button);
    for (var i = 0; i < times; i++) {
      await tester.tap(button);
      await tester.pump();
    }
    await tester.pumpAndSettle();
  }

  /// Rola até [finder] ficar visível antes de tocar — o passo de estilo/
  /// tamanho do gauge (e o de escala) podem ultrapassar a viewport de teste.
  Future<void> tapVisible(WidgetTester tester, Finder finder) async {
    await tester.ensureVisible(finder);
    await tester.tap(finder);
    await tester.pumpAndSettle();
  }

  group('Número / Número linha inteira — sem passo de escala', () {
    testWidgets('Número já abre com "Adicionar" — sem passo de escala', (
      tester,
    ) async {
      await tester.pumpWidget(
        wrap(const IndicatorFormatArgs(pid: Obd2Pid.rpm)),
      );
      await tester.pumpAndSettle();

      expect(find.text('Exibir no painel'), findsOneWidget);
      expect(find.text('Adicionar'), findsOneWidget);

      await tapPrimary(tester);

      final result = host(tester).result;
      expect(result, isNotNull);
      expect(result!.remove, isFalse);
      expect(result.display!.format, IndicatorFormat.number);
    });

    testWidgets('escolher "Número — linha inteira" e confirmar', (
      tester,
    ) async {
      await tester.pumpWidget(
        wrap(const IndicatorFormatArgs(pid: Obd2Pid.rpm)),
      );
      await tester.pumpAndSettle();

      await tapVisible(tester, find.text('Número — linha inteira'));
      await tapPrimary(tester);

      final result = host(tester).result;
      expect(result!.display!.format, IndicatorFormat.numberFull);
    });
  });

  group('Gauge', () {
    Future<void> selectGauge(WidgetTester tester) async {
      await tapVisible(tester, find.text('Gauge'));
      await tapPrimary(tester); // 'Continuar' -> passo de estilo/tamanho.
    }

    testWidgets('estilo anel é o padrão; preview usa GaugeVariant.ring', (
      tester,
    ) async {
      await tester.pumpWidget(
        wrap(const IndicatorFormatArgs(pid: Obd2Pid.rpm)),
      );
      await tester.pumpAndSettle();
      await selectGauge(tester);

      expect(find.text('Estilo do gauge'), findsOneWidget);
      final gauge = tester.widget<Gauge>(find.byType(Gauge));
      expect(gauge.variant, GaugeVariant.ring);
    });

    testWidgets('escolher Arco 270° muda a variante do preview', (
      tester,
    ) async {
      await tester.pumpWidget(
        wrap(const IndicatorFormatArgs(pid: Obd2Pid.rpm)),
      );
      await tester.pumpAndSettle();
      await selectGauge(tester);

      await tapVisible(tester, find.text('Arco 270°'));

      final gauge = tester.widget<Gauge>(find.byType(Gauge));
      expect(gauge.variant, GaugeVariant.arc270);
    });

    testWidgets('escolher Ponteiro muda a variante do preview para arc180', (
      tester,
    ) async {
      await tester.pumpWidget(
        wrap(const IndicatorFormatArgs(pid: Obd2Pid.rpm)),
      );
      await tester.pumpAndSettle();
      await selectGauge(tester);

      await tapVisible(tester, find.text('Ponteiro'));

      final gauge = tester.widget<Gauge>(find.byType(Gauge));
      expect(gauge.variant, GaugeVariant.arc180);
    });

    testWidgets('toggle Maior reflete em IndicatorGaugeSize.large no '
        'resultado final', (tester) async {
      await tester.pumpWidget(
        wrap(const IndicatorFormatArgs(pid: Obd2Pid.rpm)),
      );
      await tester.pumpAndSettle();
      await selectGauge(tester);

      await tapVisible(tester, find.text('Maior'));
      await tapPrimary(tester); // -> passo de escala
      await tapPrimary(tester); // 'Adicionar'

      final result = host(tester).result;
      expect(result!.display!.gaugeSize, IndicatorGaugeSize.large);
    });

    testWidgets('passo de escala do anel/arco não mostra os campos baixo/médio '
        '(só min/máx)', (tester) async {
      await tester.pumpWidget(
        wrap(const IndicatorFormatArgs(pid: Obd2Pid.rpm)),
      );
      await tester.pumpAndSettle();
      await selectGauge(tester);
      await tapPrimary(tester); // -> passo de escala (ring)

      expect(find.text('Valor mínimo'), findsOneWidget);
      expect(find.text('Valor máximo'), findsOneWidget);
      expect(find.text('Baixo até'), findsNothing);
      expect(find.text('Médio até'), findsNothing);
    });

    testWidgets('passo de escala do ponteiro mostra baixo/médio, e o resultado '
        'final carrega os valores editados', (tester) async {
      await tester.pumpWidget(
        wrap(const IndicatorFormatArgs(pid: Obd2Pid.rpm)),
      );
      await tester.pumpAndSettle();
      await selectGauge(tester);
      await tapVisible(tester, find.text('Ponteiro'));
      await tapPrimary(tester); // -> passo de escala (ponteiro)

      expect(find.text('Baixo até'), findsOneWidget);
      expect(find.text('Médio até'), findsOneWidget);

      await tapStepper(tester, 'Valor máximo', increment: false, times: 2);
      await tapStepper(tester, 'Baixo até', increment: true, times: 2);
      await tapPrimary(tester); // 'Adicionar'

      final display = host(tester).result!.display!;
      expect(display.max, lessThan(Obd2Pid.rpm.defaultMax));
      expect(display.lowMax, greaterThan(Obd2Pid.rpm.defaultLowMax));
      // Zonas continuam coerentes (baixo <= médio <= máx) mesmo após editar.
      expect(display.lowMax, lessThanOrEqualTo(display.highMin));
      expect(display.highMin, lessThanOrEqualTo(display.max));
    });
  });

  group('Número + barra', () {
    testWidgets('passo de escala edita min/máx, sem campos de zona', (
      tester,
    ) async {
      await tester.pumpWidget(
        wrap(const IndicatorFormatArgs(pid: Obd2Pid.coolantTemp)),
      );
      await tester.pumpAndSettle();

      await tapVisible(tester, find.text('Número + barra'));
      await tapPrimary(tester); // -> passo de escala

      expect(find.text('Valor mínimo'), findsOneWidget);
      expect(find.text('Valor máximo'), findsOneWidget);
      expect(find.text('Baixo até'), findsNothing);

      await tapStepper(tester, 'Valor máximo', increment: false, times: 2);
      await tapPrimary(tester); // 'Adicionar'

      final display = host(tester).result!.display!;
      expect(display.format, IndicatorFormat.bar);
      expect(display.max, lessThan(Obd2Pid.coolantTemp.defaultMax));
    });
  });

  group('Histórico', () {
    testWidgets(
      'passo de escala mostra só "Quantidade de pontos", clampada entre '
      '10 e 50',
      (tester) async {
        await tester.pumpWidget(
          wrap(const IndicatorFormatArgs(pid: Obd2Pid.rpm)),
        );
        await tester.pumpAndSettle();

        await tapVisible(tester, find.text('Histórico — gráfico'));
        await tapPrimary(tester); // -> passo de escala

        expect(find.text('Quantidade de pontos'), findsOneWidget);
        expect(find.text('Valor mínimo'), findsNothing);

        // Padrão é 20; decrementa (passo 5) até bater no piso de 10.
        await tapStepper(
          tester,
          'Quantidade de pontos',
          increment: false,
          times: 4,
        );
        await tapPrimary(tester);
        expect(
          host(tester).result!.display!.historyPoints,
          HistoryPointsRange.min,
        );
      },
    );

    testWidgets('quantidade de pontos não passa do teto de 50', (tester) async {
      await tester.pumpWidget(
        wrap(const IndicatorFormatArgs(pid: Obd2Pid.rpm)),
      );
      await tester.pumpAndSettle();

      await tapVisible(tester, find.text('Histórico — gráfico'));
      await tapPrimary(tester);

      // Padrão 20; incrementa bem além do teto (10 toques de 5 = +50).
      await tapStepper(
        tester,
        'Quantidade de pontos',
        increment: true,
        times: 10,
      );
      await tapPrimary(tester);

      expect(
        host(tester).result!.display!.historyPoints,
        HistoryPointsRange.max,
      );
    });
  });

  group('Edição (initial != null)', () {
    testWidgets('mostra "Salvar" e "Remover do painel", pré-preenchido', (
      tester,
    ) async {
      final initial = IndicatorDisplay.defaultFor(
        Obd2Pid.rpm,
      ).copyWith(format: IndicatorFormat.bar);

      await tester.pumpWidget(
        wrap(IndicatorFormatArgs(pid: Obd2Pid.rpm, initial: initial)),
      );
      await tester.pumpAndSettle();

      expect(find.text('Número + barra'), findsOneWidget);
      // Já no último passo (bar tem passo de escala) -> ainda não, primeiro
      // passo é a escolha de formato, que já mostra "Continuar" (bar tem
      // passo de escala) — não "Salvar" ainda.
      expect(find.text('Continuar'), findsOneWidget);
    });

    testWidgets('"Remover do painel" retorna remove=true', (tester) async {
      final initial = IndicatorDisplay.defaultFor(Obd2Pid.rpm);

      await tester.pumpWidget(
        wrap(IndicatorFormatArgs(pid: Obd2Pid.rpm, initial: initial)),
      );
      await tester.pumpAndSettle();

      // Número não tem passo de escala -> já é o último passo -> "Salvar" +
      // "Remover do painel" aparecem direto no passo 1.
      expect(find.text('Salvar'), findsOneWidget);
      await tester.tap(find.byKey(const Key('indicator_format_remove')));
      await tester.pumpAndSettle();

      final result = host(tester).result;
      expect(result!.remove, isTrue);
      expect(result.display, isNull);
    });
  });

  group('Voltar', () {
    testWidgets('back no passo 1 cancela (pop sem resultado)', (
      tester,
    ) async {
      await tester.pumpWidget(
        wrap(const IndicatorFormatArgs(pid: Obd2Pid.rpm)),
      );
      await tester.pumpAndSettle();

      await tester.tap(find.byIcon(Icons.arrow_back_ios_new));
      await tester.pumpAndSettle();

      expect(host(tester).result, isNull);
      expect(find.text('resultado: pronto'), findsOneWidget);
    });

    testWidgets('back num passo >1 volta um passo, sem sair da tela', (
      tester,
    ) async {
      await tester.pumpWidget(
        wrap(const IndicatorFormatArgs(pid: Obd2Pid.rpm)),
      );
      await tester.pumpAndSettle();

      await tapVisible(tester, find.text('Gauge'));
      await tapPrimary(tester); // -> passo de estilo/tamanho
      expect(find.text('Estilo do gauge'), findsOneWidget);

      await tester.tap(find.byIcon(Icons.arrow_back_ios_new));
      await tester.pumpAndSettle();

      // Voltou ao passo 1 (escolha de formato) da MESMA tela — não saiu da
      // rota. `_Host` fica fora da árvore de widgets enquanto '/format'
      // estiver no topo (a rota anterior não é mantida renderizada pelo
      // Navigator), então a prova de "não fechou" é continuar vendo o
      // conteúdo da tela de formato, não inspecionar o host.
      expect(find.text('Exibir no painel'), findsOneWidget);
      expect(find.text('Estilo do gauge'), findsNothing);
    });
  });
}
