import 'dart:async';
import 'dart:math' as math;

import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/icons/app_icon.dart';

/// Estado de um [ConnectStep] na checklist de conexão.
enum ConnectStepState {
  /// Concluído — círculo ciano sólido com check.
  done,

  /// Em andamento — anel ciano com mini-spinner girando.
  active,

  /// Pendente — círculo vazio neutro.
  pending,
}

/// Um passo "honesto" do handshake de conexão (ex.: "Preparando adaptador").
class ConnectStep {
  /// Cria um passo com [label], [state] e um [sublabel] opcional discreto.
  const ConnectStep(this.label, this.state, {this.sublabel});

  /// Texto do passo.
  final String label;

  /// Estado atual do passo.
  final ConnectStepState state;

  /// Detalhe técnico discreto inline (ex.: "· init ELM327"). @default null
  final String? sublabel;
}

/// Checklist vertical dos passos de conexão.
///
/// Cada passo mostra um nó (check / spinner / vazio) e seu rótulo. O nó ativo
/// gira um mini-spinner (`durSpin`), congelado sob "reduzir movimento".
class StepList extends StatefulWidget {
  /// Cria a checklist a partir de [steps], na ordem dada.
  const StepList(this.steps, {super.key});

  /// Passos, de cima para baixo.
  final List<ConnectStep> steps;

  @override
  State<StepList> createState() => _StepListState();
}

class _StepListState extends State<StepList>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: AppMotion.durSpin,
  );

  bool _reduceMotion = false;

  bool get _hasActive =>
      widget.steps.any((s) => s.state == ConnectStepState.active);

  bool get _shouldSpin => _hasActive && !_reduceMotion;

  @override
  void didChangeDependencies() {
    super.didChangeDependencies();
    _reduceMotion = context.reduceMotion;
    _syncAnimation();
  }

  @override
  void didUpdateWidget(StepList oldWidget) {
    super.didUpdateWidget(oldWidget);
    _syncAnimation();
  }

  void _syncAnimation() {
    if (_shouldSpin) {
      if (!_controller.isAnimating) unawaited(_controller.repeat());
    } else {
      _controller.stop();
    }
  }

  @override
  void dispose() {
    _controller.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      children: [
        for (var i = 0; i < widget.steps.length; i++) ...[
          if (i > 0) const SizedBox(height: AppSpacing.s7),
          _StepRow(step: widget.steps[i], spin: _controller),
        ],
      ],
    );
  }
}

class _StepRow extends StatelessWidget {
  const _StepRow({required this.step, required this.spin});

  final ConnectStep step;
  final Animation<double> spin;

  @override
  Widget build(BuildContext context) {
    final labelColor = step.state == ConnectStepState.pending
        ? AppColors.textTertiary
        : AppColors.textPrimary;
    final labelWeight = step.state == ConnectStepState.active
        ? FontWeight.w600
        : FontWeight.w400;

    return Row(
      children: [
        _node(),
        const SizedBox(width: AppSpacing.s5),
        Flexible(
          child: Text.rich(
            TextSpan(
              children: [
                TextSpan(
                  text: step.label,
                  style: AppTypography.ui(
                    TextStyle(
                      fontSize: 15,
                      fontWeight: labelWeight,
                      color: labelColor,
                    ),
                  ),
                ),
                if (step.sublabel != null)
                  TextSpan(
                    text: '  ${step.sublabel}',
                    style: AppTypography.ui(
                      const TextStyle(
                        fontSize: 11,
                        fontWeight: FontWeight.w400,
                        color: AppColors.textTertiary,
                      ),
                    ),
                  ),
              ],
            ),
          ),
        ),
      ],
    );
  }

  Widget _node() {
    const dim = 24.0;
    switch (step.state) {
      case ConnectStepState.done:
        return Container(
          width: dim,
          height: dim,
          decoration: const BoxDecoration(
            color: AppColors.cyan500,
            shape: BoxShape.circle,
          ),
          alignment: Alignment.center,
          child: const AppIcon(
            AppIconData.check,
            size: 13,
            color: AppColors.accentOn,
          ),
        );
      case ConnectStepState.active:
        return Container(
          width: dim,
          height: dim,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            border: Border.all(color: AppColors.cyan500, width: 2),
          ),
          alignment: Alignment.center,
          child: RotationTransition(
            turns: spin,
            child: const CustomPaint(
              size: Size(14, 14),
              painter: _SpinnerPainter(),
            ),
          ),
        );
      case ConnectStepState.pending:
        return Container(
          width: dim,
          height: dim,
          decoration: BoxDecoration(
            shape: BoxShape.circle,
            border: Border.all(color: AppColors.neutral700, width: 2),
          ),
        );
    }
  }
}

/// Mini-spinner: trilha fraca + um arco ciano de ~60°.
class _SpinnerPainter extends CustomPainter {
  const _SpinnerPainter();

  @override
  void paint(Canvas canvas, Size size) {
    final center = size.center(Offset.zero);
    final radius = size.width / 2 - 1.5;
    final rect = Rect.fromCircle(center: center, radius: radius);

    final track = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 2
      ..color = AppColors.cyan08;
    canvas.drawCircle(center, radius, track);

    final arc = Paint()
      ..style = PaintingStyle.stroke
      ..strokeWidth = 2
      ..strokeCap = StrokeCap.round
      ..color = AppColors.cyan500;
    // Arco de ~70° a partir do topo.
    canvas.drawArc(rect, -math.pi / 2, math.pi / 2.6, false, arc);
  }

  @override
  bool shouldRepaint(_SpinnerPainter oldDelegate) => false;
}
