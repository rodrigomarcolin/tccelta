import 'dart:async';
import 'dart:math';

import 'package:flutter/material.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// Galeria do OBD2 Cockpit design system.
///
/// Renderiza as foundations (cores, tipografia, espaçamento/raios, ícones) e
/// todos os átomos de UI em suas variações, para validar fidelidade, apoiar
/// manutenção e servir de onboarding. Não contém regra de negócio — é apenas
/// uma vitrine dos componentes de `ui/core/`.
class ShowcaseScreen extends StatefulWidget {
  /// Cria a tela de showcase.
  const ShowcaseScreen({super.key});

  @override
  State<ShowcaseScreen> createState() => _ShowcaseScreenState();
}

class _ShowcaseScreenState extends State<ShowcaseScreen> {
  final Random _random = Random();
  String _activeTab = 'painel';

  // Valores ao vivo simulados (para demonstrar o ease dos gauges/barras).
  double _rpm = 5200;
  double _speed = 88;
  double _temp = 89;
  double _load = 34;

  void _simulate() {
    setState(() {
      _rpm = 1000 + _random.nextDouble() * 7000;
      _speed = _random.nextDouble() * 220;
      _temp = 60 + _random.nextDouble() * 60;
      _load = _random.nextDouble() * 100;
    });
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      body: SafeArea(
        bottom: false,
        child: Column(
          children: [
            const StatusBand(),
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(
                  AppSpacing.s7,
                  AppSpacing.s7,
                  AppSpacing.s7,
                  AppSpacing.s9,
                ),
                children: [
                  Text('Design System', style: AppTypography.heading),
                  const SizedBox(height: 4),
                  Text(
                    'OBD2 Cockpit · vitrine de tokens e componentes',
                    style: AppTypography.body,
                  ),
                  const _Section(title: 'CORES', child: _ColorsDemo()),
                  const _Section(title: 'TIPOGRAFIA', child: _TypographyDemo()),
                  const _Section(
                    title: 'ESPAÇAMENTO & RAIOS',
                    child: _SpacingDemo(),
                  ),
                  const _Section(title: 'BOTÕES', child: _ButtonsDemo()),
                  const _Section(title: 'STATUS', child: _StatusDemo()),
                  _Section(
                    title: 'GAUGES',
                    child: _GaugesDemo(
                      rpm: _rpm,
                      speed: _speed,
                      temp: _temp,
                      onSimulate: _simulate,
                    ),
                  ),
                  _Section(
                    title: 'STAT CARDS',
                    child: _StatCardsDemo(
                      speed: _speed,
                      load: _load,
                      temp: _temp,
                    ),
                  ),
                  const _Section(title: 'GRÁFICO', child: _StatGraphCardDemo()),
                  const _Section(title: 'SENSORES', child: _SensorsDemo()),
                  const _Section(title: 'ÍCONES', child: _IconsDemo()),
                ],
              ),
            ),
          ],
        ),
      ),
      bottomNavigationBar: AppTabBar(
        active: _activeTab,
        onChanged: (key) => setState(() => _activeTab = key),
      ),
    );
  }
}

/// Bloco de seção com um overline e o conteúdo abaixo.
class _Section extends StatelessWidget {
  const _Section({required this.title, required this.child});

  final String title;
  final Widget child;

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.only(top: AppSpacing.s9),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          Text(title, style: AppTypography.overline),
          const SizedBox(height: AppSpacing.s5),
          child,
        ],
      ),
    );
  }
}

class _ColorsDemo extends StatelessWidget {
  const _ColorsDemo();

  static const List<(String, Color)> _swatches = [
    ('neutral50', AppColors.neutral50),
    ('neutral200', AppColors.neutral200),
    ('neutral400', AppColors.neutral400),
    ('neutral700', AppColors.neutral700),
    ('surfaceCard', AppColors.surfaceCard),
    ('bgScreen', AppColors.bgScreen),
    ('cyan500', AppColors.cyan500),
    ('amber500', AppColors.amber500),
    ('red500', AppColors.red500),
    ('green500', AppColors.green500),
  ];

  @override
  Widget build(BuildContext context) {
    return Wrap(
      spacing: AppSpacing.s3,
      runSpacing: AppSpacing.s3,
      children: [
        for (final (name, color) in _swatches)
          SizedBox(
            width: 72,
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.start,
              children: [
                Container(
                  height: 48,
                  decoration: BoxDecoration(
                    color: color,
                    borderRadius: AppRadii.brMd,
                    border: Border.all(color: AppColors.borderHairline),
                  ),
                ),
                const SizedBox(height: 6),
                Text(
                  name,
                  style: AppTypography.mono(
                    const TextStyle(
                      fontSize: 10,
                      color: AppColors.textTertiary,
                    ),
                  ),
                ),
              ],
            ),
          ),
      ],
    );
  }
}

class _TypographyDemo extends StatelessWidget {
  const _TypographyDemo();

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Text('5.200', style: AppTypography.displayL),
        const SizedBox(height: AppSpacing.s2),
        Text('Título de tela', style: AppTypography.heading),
        const SizedBox(height: AppSpacing.s2),
        Text('Título de painel', style: AppTypography.title),
        const SizedBox(height: AppSpacing.s2),
        Text('Corpo proeminente / botão', style: AppTypography.bodyL),
        const SizedBox(height: AppSpacing.s2),
        Text(
          'Parágrafo de instrução comum, em pt-BR, calmo e técnico.',
          style: AppTypography.body,
        ),
        const SizedBox(height: AppSpacing.s2),
        Text('Label de lista', style: AppTypography.label),
        const SizedBox(height: AppSpacing.s2),
        Text('OVERLINE DE SEÇÃO', style: AppTypography.overline),
      ],
    );
  }
}

class _SpacingDemo extends StatelessWidget {
  const _SpacingDemo();

  static const List<(String, double)> _spaces = [
    ('s2', AppSpacing.s2),
    ('s3', AppSpacing.s3),
    ('s4', AppSpacing.s4),
    ('s5', AppSpacing.s5),
    ('s7', AppSpacing.s7),
    ('s9', AppSpacing.s9),
  ];

  static const List<(String, double)> _radii = [
    ('sm', AppRadii.sm),
    ('md', AppRadii.md),
    ('btn', AppRadii.btn),
    ('lg', AppRadii.lg),
    ('xl', AppRadii.xl),
  ];

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        for (final (name, size) in _spaces)
          Padding(
            padding: const EdgeInsets.only(bottom: AppSpacing.s2),
            child: Row(
              children: [
                SizedBox(
                  width: 28,
                  child: Text(
                    name,
                    style: AppTypography.mono(
                      const TextStyle(
                        fontSize: 11,
                        color: AppColors.textTertiary,
                      ),
                    ),
                  ),
                ),
                Container(height: 10, width: size, color: AppColors.cyan500),
              ],
            ),
          ),
        const SizedBox(height: AppSpacing.s4),
        Wrap(
          spacing: AppSpacing.s3,
          runSpacing: AppSpacing.s3,
          children: [
            for (final (name, radius) in _radii)
              Column(
                children: [
                  Container(
                    width: 48,
                    height: 48,
                    decoration: BoxDecoration(
                      color: AppColors.surfaceCard,
                      borderRadius: BorderRadius.circular(radius),
                      border: Border.all(color: AppColors.borderStrong),
                    ),
                  ),
                  const SizedBox(height: 4),
                  Text(
                    name,
                    style: AppTypography.mono(
                      const TextStyle(
                        fontSize: 10,
                        color: AppColors.textTertiary,
                      ),
                    ),
                  ),
                ],
              ),
          ],
        ),
      ],
    );
  }
}

class _ButtonsDemo extends StatelessWidget {
  const _ButtonsDemo();

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        AppButton(onPressed: () {}, child: const Text('Permitir')),
        const SizedBox(height: AppSpacing.s3),
        AppButton(
          variant: AppButtonVariant.warning,
          onPressed: () {},
          child: const Text('Abrir ajustes'),
        ),
        const SizedBox(height: AppSpacing.s3),
        AppButton(
          variant: AppButtonVariant.secondary,
          onPressed: () {},
          child: const Text('Procurar novamente'),
        ),
        const SizedBox(height: AppSpacing.s3),
        AppButton(
          variant: AppButtonVariant.tonal,
          onPressed: () {},
          icon: const AppIcon(AppIconData.estrela, size: 18),
          child: const Text('Adicionar ao painel'),
        ),
        const SizedBox(height: AppSpacing.s3),
        AppButton(
          variant: AppButtonVariant.link,
          onPressed: () {},
          child: const Text('Saiba mais'),
        ),
      ],
    );
  }
}

class _StatusDemo extends StatelessWidget {
  const _StatusDemo();

  @override
  Widget build(BuildContext context) {
    return const Wrap(
      spacing: AppSpacing.s3,
      runSpacing: AppSpacing.s3,
      children: [
        StatusBadge(label: 'AO VIVO', pulse: true),
        StatusBadge(label: 'OK', tone: StatusTone.ok),
        StatusBadge(label: 'AVISO', tone: StatusTone.warning),
        StatusBadge(label: 'ALERTA', tone: StatusTone.alert),
      ],
    );
  }
}

class _GaugesDemo extends StatelessWidget {
  const _GaugesDemo({
    required this.rpm,
    required this.speed,
    required this.temp,
    required this.onSimulate,
  });

  final double rpm;
  final double speed;
  final double temp;
  final VoidCallback onSimulate;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        Wrap(
          spacing: AppSpacing.s7,
          runSpacing: AppSpacing.s7,
          children: [
            Gauge(value: rpm, max: 10000, size: 150),
            Gauge(
              value: speed,
              max: 220,
              colorByZone: false,
              label: 'KM/H',
              size: 150,
              variant: GaugeVariant.arc270,
            ),
            const Gauge(
              value: 105,
              max: 130,
              label: '',
              unit: '°',
              size: 150,
              variant: GaugeVariant.arc180,
              warningThreshold: 70 / 130,
              alertThreshold: 105 / 130,
            ),
            Gauge(
              value: speed,
              max: 220,
              label: 'KM/H',
              size: 150,
              weight: 4,
            ),
          ],
        ),
        const SizedBox(height: AppSpacing.s5),
        AppButton(
          variant: AppButtonVariant.tonal,
          fullWidth: false,
          onPressed: onSimulate,
          icon: const AppIcon(AppIconData.recarregar, size: 18),
          child: const Text('Simular leitura'),
        ),
      ],
    );
  }
}

class _StatCardsDemo extends StatelessWidget {
  const _StatCardsDemo({
    required this.speed,
    required this.load,
    required this.temp,
  });

  final double speed;
  final double load;
  final double temp;

  @override
  Widget build(BuildContext context) {
    return Column(
      children: [
        // value + progress — mesma altura, com ou sem barra.
        Row(
          children: [
            Expanded(
              child: StatCard.value(
                label: 'Velocidade',
                value: speed.round(),
                unit: 'km/h',
              ),
            ),
            const SizedBox(width: AppSpacing.s3),
            Expanded(
              child: StatCard.progress(
                label: 'Carga',
                value: load.round(),
                unit: '%',
                pct: load,
                colorByZone: true,
              ),
            ),
          ],
        ),
        const SizedBox(height: AppSpacing.s3),
        // info — fato estático.
        const StatCard.info(
          label: 'Protocolo',
          value: 'ISO 15765-4 CAN',
        ),
        const SizedBox(height: AppSpacing.s3),
        // gauge — qualquer GaugeVariant pode ser passado como widget.
        Row(
          children: [
            Expanded(
              child: StatCard.gauge(
                label: 'Temp. arrefecimento',
                value: temp.round(),
                unit: '°C',
                gauge: Gauge(
                  value: temp,
                  max: 130,
                  label: '',
                  size: 64,
                  weight: 9,
                  warningThreshold: 100 / 130,
                  alertThreshold: 115 / 130,
                ),
              ),
            ),
            const SizedBox(width: AppSpacing.s3),
            const Expanded(
              child: StatCard.gauge(
                label: 'Rotação',
                value: 5200,
                unit: 'rpm',
                gauge: Gauge(
                  value: 5200,
                  max: 10000,
                  label: '',
                  size: 64,
                  variant: GaugeVariant.arc270,
                ),
              ),
            ),
          ],
        ),
      ],
    );
  }
}

class _StatGraphCardDemo extends StatefulWidget {
  const _StatGraphCardDemo();

  @override
  State<_StatGraphCardDemo> createState() => _StatGraphCardDemoState();
}

class _StatGraphCardDemoState extends State<_StatGraphCardDemo> {
  // Curva do mock: cai, estabiliza e sobe terminando em 9.5.
  static const List<double> _seed = [
    6.2,
    4.1,
    2.8,
    2.1,
    2.4,
    3.0,
    3.4,
    4.8,
    7.1,
    9.8,
    12.6,
    11.9,
    10.4,
    9.5,
  ];

  /// Quantas amostras a janela deslizante mantém. Comprimento fixo, para o
  /// gráfico atualizar ao vivo sem re-disparar a animação de entrada.
  static const int _window = 32;

  final Random _random = Random();
  late List<double> _history = _fill(_seed);
  Timer? _timer;

  /// Preenche a janela com [_window] amostras, repetindo o seed por um passeio
  /// aleatório suave para começar já cheia (sem "crescer" na tela).
  List<double> _fill(List<double> seed) {
    final out = <double>[];
    var v = seed.first;
    for (var i = 0; i < _window; i++) {
      v = i < seed.length ? seed[i] : _nextSample(v);
      out.add(v);
    }
    return out;
  }

  /// Próxima amostra por passeio aleatório, numa faixa plausível de MAF.
  double _nextSample(double last) {
    final delta = (_random.nextDouble() - 0.5) * 3;
    return (last + delta).clamp(1.5, 16.0);
  }

  bool get _streaming => _timer != null;

  void _toggle() {
    setState(() {
      if (_streaming) {
        _timer?.cancel();
        _timer = null;
      } else {
        _timer = Timer.periodic(
          const Duration(milliseconds: 700),
          (_) => setState(() {
            _history = [
              ..._history.skip(1),
              _nextSample(_history.last),
            ];
          }),
        );
      }
    });
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        StatGraphCard(
          label: 'Fluxo de ar (MAF)',
          history: _history,
          unit: 'g/s',
        ),
        const SizedBox(height: AppSpacing.s5),
        AppButton(
          variant: AppButtonVariant.tonal,
          fullWidth: false,
          onPressed: _toggle,
          icon: AppIcon(
            _streaming ? AppIconData.desconectar : AppIconData.recarregar,
            size: 18,
          ),
          child: Text(_streaming ? 'Pausar ao vivo' : 'Simular ao vivo'),
        ),
      ],
    );
  }
}

class _SensorsDemo extends StatelessWidget {
  const _SensorsDemo();

  @override
  Widget build(BuildContext context) {
    return const Column(
      children: [
        CardButton(
          icon: AppIconData.estrela,
          title: 'Rotação do motor',
          subtitle: '01 0C',
          value: 5200,
          unit: 'rpm',
          iconColor: AppColors.cyan500,
        ),
        SizedBox(height: AppSpacing.s2),
        CardButton(
          icon: AppIconData.estrela,
          title: 'Temperatura do líquido',
          subtitle: '01 05',
          value: 89,
          unit: '°C',
        ),
        SizedBox(height: AppSpacing.s2),
        CardButton(
          icon: AppIconData.estrela,
          title: 'Pressão do coletor',
          subtitle: '01 0B',
        ),
      ],
    );
  }
}

class _IconsDemo extends StatelessWidget {
  const _IconsDemo();

  @override
  Widget build(BuildContext context) {
    return Wrap(
      spacing: AppSpacing.s3,
      runSpacing: AppSpacing.s3,
      children: [
        for (final icon in AppIconData.values)
          Container(
            width: 56,
            height: 56,
            decoration: BoxDecoration(
              color: AppColors.surfaceCard,
              borderRadius: AppRadii.brMd,
              border: Border.all(color: AppColors.borderHairline),
            ),
            child: Center(child: AppIcon(icon)),
          ),
      ],
    );
  }
}
