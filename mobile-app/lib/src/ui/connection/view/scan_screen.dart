import 'dart:async';

import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_background.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// 1.3 — Procurar dongles BLE próximos.
///
/// Lista (mockada) os dispositivos encontrados: o `OBD2Dongle` em destaque
/// (selecionável) e um desconhecido esmaecido. Tocar no dongle inicia a
/// conexão; "Procurar novamente" reinicia a animação do radar.
class ScanScreen extends StatefulWidget {
  /// Cria a tela de busca.
  const ScanScreen({super.key});

  @override
  State<ScanScreen> createState() => _ScanScreenState();
}

class _ScanScreenState extends State<ScanScreen> {
  // Trocar a key remonta o radar (reinicia a varredura) ao buscar de novo.
  int _scanAttempt = 0;

  @override
  Widget build(BuildContext context) {
    return ConnectionBackground(
      child: Padding(
        padding: const EdgeInsets.symmetric(horizontal: AppSpacing.s7),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.stretch,
          children: [
            Expanded(
              child: ListView(
                padding: const EdgeInsets.symmetric(vertical: AppSpacing.s7),
                children: [
                  Text('Procurar dongle', style: AppTypography.heading),
                  const SizedBox(height: AppSpacing.s2),
                  const _SearchingLabel(),
                  const SizedBox(height: AppSpacing.s5),
                  Center(child: RadarScanner(key: ValueKey(_scanAttempt))),
                  const SizedBox(height: AppSpacing.s7),
                  Text('ENCONTRADOS', style: AppTypography.overline),
                  const SizedBox(height: AppSpacing.s4),
                  CardButton(
                    leading: const IconTile(AppIconData.dongle),
                    title: 'OBD2Dongle',
                    titleMono: true,
                    subtitle: 'ELM327 · sinal forte',
                    subtitleColor: AppColors.cyan500,
                    subtitleMono: false,
                    showValue: false,
                    selected: true,
                    onTap: () => context.push(AppRoutes.connecting),
                  ),
                  const SizedBox(height: AppSpacing.s3),
                  const CardButton(
                    leading: IconTile(
                      AppIconData.dongle,
                      background: AppColors.track,
                      iconColor: AppColors.textTertiary,
                    ),
                    title: 'XX:4B:9A:1C',
                    titleMono: true,
                    subtitle: 'Desconhecido · sinal fraco',
                    subtitleMono: false,
                    showValue: false,
                    showChevron: false,
                    dimmed: true,
                  ),
                ],
              ),
            ),
            const SizedBox(height: AppSpacing.s5),
            AppButton(
              variant: AppButtonVariant.secondary,
              onPressed: () => setState(() => _scanAttempt++),
              child: const Text('Procurar novamente'),
            ),
            const SizedBox(height: AppSpacing.s7),
          ],
        ),
      ),
    );
  }
}

/// Rótulo "Procurando…" com três pontinhos piscando em sequência.
class _SearchingLabel extends StatefulWidget {
  const _SearchingLabel();

  @override
  State<_SearchingLabel> createState() => _SearchingLabelState();
}

class _SearchingLabelState extends State<_SearchingLabel>
    with SingleTickerProviderStateMixin {
  late final AnimationController _controller = AnimationController(
    vsync: this,
    duration: const Duration(milliseconds: 1200),
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
    return Row(
      children: [
        for (var i = 0; i < 3; i++) ...[
          if (i > 0) const SizedBox(width: 3),
          _Dot(controller: _controller, phase: i * 0.2),
        ],
        const SizedBox(width: AppSpacing.s2),
        Text(
          'Procurando dispositivos próximos…',
          style: AppTypography.ui(
            const TextStyle(
              fontSize: 13,
              fontWeight: FontWeight.w500,
              color: AppColors.cyan500,
            ),
          ),
        ),
      ],
    );
  }
}

class _Dot extends StatelessWidget {
  const _Dot({required this.controller, required this.phase});

  final AnimationController controller;
  final double phase;

  @override
  Widget build(BuildContext context) {
    return AnimatedBuilder(
      animation: controller,
      builder: (context, child) {
        // Onda triangular defasada: 0.2 -> 1 -> 0.2 ao longo do ciclo.
        final t = (controller.value + phase) % 1.0;
        final wave = 1 - (2 * t - 1).abs();
        final opacity = 0.2 + 0.8 * wave;
        return Opacity(opacity: opacity, child: child);
      },
      child: Container(
        width: 5,
        height: 5,
        decoration: const BoxDecoration(
          color: AppColors.cyan500,
          shape: BoxShape.circle,
        ),
      ),
    );
  }
}
