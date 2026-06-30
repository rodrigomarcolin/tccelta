import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_background.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// Flag mock: na 1ª tentativa do tour a conexão "cai" (leva ao estado de
/// Conexão perdida); nas seguintes conecta. Torna o ramo de erro alcançável
/// sem a camada BLE. Trivial de remover quando o estado vier do dongle real.
bool _firstConnectDropped = false;

/// Rótulos dos passos honestos do handshake, em ordem.
const List<(String, String?)> _stepLabels = [
  ('Conectando', null),
  ('Otimizando conexão', '· MTU'),
  ('Preparando adaptador', '· init ELM327'),
  ('Lendo capacidades do veículo', null),
];

/// 1.4 — Handshake de conexão em andamento.
///
/// Mostra um spinner e os passos avançando (mockados por timer). Ao concluir,
/// navega para o estado seguinte ([_firstConnectDropped] decide entre Conexão
/// perdida e Conectado).
class ConnectingScreen extends StatefulWidget {
  /// Cria a tela de conexão em andamento.
  const ConnectingScreen({super.key});

  @override
  State<ConnectingScreen> createState() => _ConnectingScreenState();
}

class _ConnectingScreenState extends State<ConnectingScreen> {
  Timer? _timer;
  int _current = 0;

  @override
  void initState() {
    super.initState();
    _timer = Timer.periodic(const Duration(milliseconds: 700), (timer) {
      if (_current >= _stepLabels.length) {
        timer.cancel();
        _finish();
        return;
      }
      setState(() => _current++);
    });
  }

  void _finish() {
    if (!mounted) return;
    if (!_firstConnectDropped) {
      _firstConnectDropped = true;
      context.pushReplacement(AppRoutes.connectionLost);
    } else {
      context.pushReplacement(AppRoutes.connected);
    }
  }

  @override
  void dispose() {
    _timer?.cancel();
    super.dispose();
  }

  List<ConnectStep> get _steps => [
        for (var i = 0; i < _stepLabels.length; i++)
          ConnectStep(
            _stepLabels[i].$1,
            i < _current
                ? ConnectStepState.done
                : i == _current
                    ? ConnectStepState.active
                    : ConnectStepState.pending,
            sublabel: _stepLabels[i].$2,
          ),
      ];

  @override
  Widget build(BuildContext context) {
    return ConnectionBackground(
      child: Padding(
        padding: const EdgeInsets.all(AppSpacing.s7),
        child: Column(
          children: [
            const SizedBox(height: AppSpacing.s7),
            const _BleSpinner(),
            const SizedBox(height: AppSpacing.s9),
            Text('Conectando ao dongle', style: AppTypography.title),
            const SizedBox(height: AppSpacing.s2),
            Text(
              'OBD2Dongle',
              style: AppTypography.mono(
                const TextStyle(
                  fontSize: 14,
                  fontWeight: FontWeight.w500,
                  color: AppColors.textSecondary,
                ),
              ),
            ),
            const SizedBox(height: AppSpacing.s9),
            StepList(_steps),
            const Spacer(),
            Text(
              'Mantenha o dongle plugado e o telefone próximo.',
              style: AppTypography.ui(
                const TextStyle(
                  fontSize: 12,
                  fontWeight: FontWeight.w400,
                  color: AppColors.textTertiary,
                ),
              ),
              textAlign: TextAlign.center,
            ),
          ],
        ),
      ),
    );
  }
}

/// Anel ciano girando com o rótulo "BLE" no centro.
class _BleSpinner extends StatefulWidget {
  const _BleSpinner();

  @override
  State<_BleSpinner> createState() => _BleSpinnerState();
}

class _BleSpinnerState extends State<_BleSpinner>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: AppMotion.durSpin,
  );

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    if (context.reduceMotion) {
      _controller.stop();
    } else if (!_controller.isAnimating) {
      unawaited(_controller.repeat());
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      width: 120,
      height: 120,
      child: Stack(
        alignment: Alignment.center,
        children: [
          RotationTransition(
            turns: _controller,
            child: const CustomPaint(
              size: Size(120, 120),
              painter: _RingPainter(),
            ),
          ),
          Text(
            'BLE',
            style: AppTypography.mono(
              const TextStyle(
                fontSize: 13,
                fontWeight: FontWeight.w600,
                color: AppColors.cyan500,
              ),
            ),
          ),
        ],
      ),
    );
  }
}

/// Trilha escura + um arco ciano (o "progresso" girando).
class _RingPainter extends CustomPainter {
  const _RingPainter();

  @override
  void paint(Canvas canvas, Size size) {
    final center = size.center(Offset.zero);
    final radius = size.width / 2 - 4;
    final rect = Rect.fromCircle(center: center, radius: radius);

    final track = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 4
      ..color = AppColors.neutral800;
    canvas.drawCircle(center, radius, track);

    final arc = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 4
      ..strokeCap = StrokeCap.round
      ..color = AppColors.cyan500;
    // Arco de ~80° a partir do topo.
    canvas.drawArc(rect, -math.pi / 2, math.pi / 2.2, false, arc);
  }

  @override
  bool shouldRepaint(_RingPainter oldDelegate) => false;
}
