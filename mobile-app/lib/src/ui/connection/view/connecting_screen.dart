import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/connection/view_model/connecting_view_model.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_background.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// Mapeia a fase BLE nos 4 passos honestos do handshake.
///
/// Decisão de escopo (Fase 1): só "Conectando" e "Otimizando · MTU" são
/// dirigidos pelo BLE real; os passos de init do ELM327 e leitura de
/// capacidades são Fase 2 e permanecem pendentes por ora.
List<ConnectStep> connectingStepsFor(BleConnectionPhase phase) {
  final ConnectStepState connectState;
  final ConnectStepState optimizeState;
  switch (phase) {
    case BleConnectionPhase.idle:
    case BleConnectionPhase.connecting:
    case BleConnectionPhase.reconnecting:
      connectState = ConnectStepState.active;
      optimizeState = ConnectStepState.pending;
    case BleConnectionPhase.optimizingLink:
    case BleConnectionPhase.discovering:
    case BleConnectionPhase.enablingNotify:
      connectState = ConnectStepState.done;
      optimizeState = ConnectStepState.active;
    case BleConnectionPhase.ready:
      connectState = ConnectStepState.done;
      optimizeState = ConnectStepState.done;
    case BleConnectionPhase.disconnected:
    case BleConnectionPhase.failed:
      connectState = ConnectStepState.pending;
      optimizeState = ConnectStepState.pending;
  }
  return [
    ConnectStep('Conectando', connectState),
    ConnectStep('Otimizando conexão', optimizeState, sublabel: '· MTU'),
    // Fase 2 — ainda não executados pela camada BLE.
    const ConnectStep(
      'Preparando adaptador',
      ConnectStepState.pending,
      sublabel: '· init ELM327',
    ),
    const ConnectStep('Lendo capacidades do veículo', ConnectStepState.pending),
  ];
}

/// 1.4 — Handshake de conexão em andamento.
///
/// Dirige os passos a partir das fases REAIS do BLE (Fase 1): "Conectando" e
/// "Otimizando · MTU". Os passos de init do ELM327 e leitura de capacidades
/// são Fase 2 e ficam pendentes por ora. Ao ficar `ready`, segue para
/// Conectado; em falha, para Conexão perdida.
class ConnectingScreen extends ConsumerStatefulWidget {
  /// Cria a tela de conexão em andamento.
  const ConnectingScreen({super.key});

  @override
  ConsumerState<ConnectingScreen> createState() => _ConnectingScreenState();
}

class _ConnectingScreenState extends ConsumerState<ConnectingScreen> {
  @override
  void initState() {
    super.initState();
    WidgetsBinding.instance.addPostFrameCallback((_) {
      final device = ref.read(selectedDongleProvider);
      if (device == null) {
        // Sem dongle selecionado (ex.: acesso direto à rota): volta à busca.
        context.go(AppRoutes.scan);
        return;
      }
      unawaited(
        ref.read(connectingViewModelProvider.notifier).connect(device.id),
      );
    });
  }

  @override
  Widget build(BuildContext context) {
    final phase = ref.watch(connectingViewModelProvider);
    final device = ref.watch(selectedDongleProvider);

    // Navega conforme o desfecho do handshake.
    ref.listen(connectingViewModelProvider, (_, next) {
      if (!context.mounted) return;
      if (next == BleConnectionPhase.ready) {
        context.pushReplacement(AppRoutes.connected);
      } else if (next == BleConnectionPhase.failed) {
        context.pushReplacement(AppRoutes.connectionLost);
      }
    });

    return ConnectionBackground(
      child: Padding(
        padding: const EdgeInsets.all(AppSpacing.s7),
        child: Column(
          children: [
            const SizedBox(height: AppSpacing.s7),
            const _BleSpinner(),
            const SizedBox(height: AppSpacing.s9),
            Text('Conectando ao dongle', style: AppTypography.title),
            if (device?.displayName.isNotEmpty ?? false) ...[
              const SizedBox(height: AppSpacing.s2),
              Text(
                device!.displayName,
                style: AppTypography.mono(
                  const TextStyle(
                    fontSize: 14,
                    fontWeight: FontWeight.w500,
                    color: AppColors.textSecondary,
                  ),
                ),
              ),
            ],
            const SizedBox(height: AppSpacing.s9),
            StepList(connectingStepsFor(phase)),
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
