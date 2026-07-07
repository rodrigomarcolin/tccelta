import 'dart:async';
import 'dart:io' show Platform;
import 'dart:math' as math;

import 'package:flutter/material.dart';
import 'package:flutter_hooks/flutter_hooks.dart';
import 'package:go_router/go_router.dart';
import 'package:hooks_riverpod/hooks_riverpod.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/connection/view_model/connecting_view_model.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_background.dart';
import 'package:tccelta_mobile/src/ui/core/hooks/hooks.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// Mapeia o estado do handshake nos 4 passos honestos.
///
/// Os 2 primeiros ("Conectando", "Otimizando · MTU") são dirigidos pela fase
/// BLE (Fase 1); os 2 últimos ("Preparando adaptador · init ELM327", "Lendo
/// capacidades do veículo") pela sub-fase de preparação OBD-II (Fase 2), que
/// roda após o BLE ficar `ready`.
List<ConnectStep> connectingStepsFor(ConnectingState state) {
  final phase = state.phase;
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

  final ConnectStepState prepareState;
  final ConnectStepState readState;
  switch (state.prep) {
    case ConnectingPrep.idle:
      prepareState = ConnectStepState.pending;
      readState = ConnectStepState.pending;
    case ConnectingPrep.preparing:
      prepareState = ConnectStepState.active;
      readState = ConnectStepState.pending;
    case ConnectingPrep.reading:
      prepareState = ConnectStepState.done;
      readState = ConnectStepState.active;
    case ConnectingPrep.done:
      prepareState = ConnectStepState.done;
      readState = ConnectStepState.done;
  }

  // O `requestMtu` explícito só ocorre no Android; no iOS o SO negocia o MTU
  // sozinho, então o sublabel "· MTU" não se aplica lá.
  final optimizeSublabel = Platform.isAndroid ? '· MTU' : null;

  return [
    ConnectStep('Conectando', connectState),
    ConnectStep(
      'Otimizando conexão',
      optimizeState,
      sublabel: optimizeSublabel,
    ),
    ConnectStep(
      'Preparando adaptador',
      prepareState,
      sublabel: '· init ELM327',
    ),
    ConnectStep('Lendo capacidades do veículo', readState),
  ];
}

/// 1.4 — Handshake de conexão em andamento.
///
/// Dirige os passos a partir das fases REAIS do BLE (Fase 1): "Conectando" e
/// "Otimizando · MTU". Os passos de init do ELM327 e leitura de capacidades
/// são Fase 2 e ficam pendentes por ora. Ao ficar `ready`, segue para
/// Conectado; em falha, para Conexão perdida.
class ConnectingScreen extends HookConsumerWidget {
  /// Cria a tela de conexão em andamento.
  const ConnectingScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // Inicia o handshake uma única vez ao montar. O callback do useEffect roda
    // DURANTE o build, então mutar um provider (connect() faz `state = ...`) ou
    // navegar aqui direto dispara "modify a provider while building". Adiamos
    // para o pós-frame (equivale ao antigo initState + addPostFrameCallback).
    useEffect(
      () {
        WidgetsBinding.instance.addPostFrameCallback((_) {
          if (!context.mounted) return;
          final device = ref.read(selectedDongleProvider);
          if (device == null) {
            // Sem dongle selecionado (ex.: acesso direto): volta à busca.
            context.go(AppRoutes.scan);
            return;
          }
          unawaited(
            ref.read(connectingViewModelProvider.notifier).connect(device.id),
          );
        });
        return null;
      },
      const [],
    );

    final state = ref.watch(connectingViewModelProvider);
    final device = ref.watch(selectedDongleProvider);

    // Navega conforme o desfecho do handshake. Só segue para "Conectado" quando
    // a preparação OBD-II terminou (protocolo + sensores já preenchidos).
    ref.listen(connectingViewModelProvider, (_, next) {
      if (!context.mounted) return;
      if (next.phase == BleConnectionPhase.ready &&
          next.prep == ConnectingPrep.done) {
        context.pushReplacement(AppRoutes.connected);
      } else if (next.phase == BleConnectionPhase.failed) {
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
            StepList(connectingStepsFor(state)),
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
class _BleSpinner extends HookWidget {
  const _BleSpinner();

  @override
  Widget build(BuildContext context) {
    final controller = useLoopController(AppMotion.durSpin);
    return SizedBox(
      width: 120,
      height: 120,
      child: Stack(
        alignment: Alignment.center,
        children: [
          RotationTransition(
            turns: controller,
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
