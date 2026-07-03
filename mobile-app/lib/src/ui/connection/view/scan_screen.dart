import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/connection/view_model/scan_view_model.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_background.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// 1.3 — Procurar dongles BLE próximos.
///
/// Escaneia dongles `OBD2Dongle` reais e lista os encontrados. Tocar num
/// dongle o seleciona e inicia a conexão; "Procurar novamente" reinicia a
/// varredura (e a animação do radar).
class ScanScreen extends ConsumerStatefulWidget {
  /// Cria a tela de busca.
  const ScanScreen({super.key});

  @override
  ConsumerState<ScanScreen> createState() => _ScanScreenState();
}

class _ScanScreenState extends ConsumerState<ScanScreen> {
  // Trocar a key remonta o radar (reinicia a varredura) ao buscar de novo.
  int _scanAttempt = 0;

  @override
  void initState() {
    super.initState();
    // Dispara o scan após o primeiro frame (evita mutar provider no build).
    WidgetsBinding.instance.addPostFrameCallback(
      (_) => ref.read(scanViewModelProvider.notifier).startScan(),
    );
  }

  void _rescan() {
    unawaited(ref.read(scanViewModelProvider.notifier).startScan());
    setState(() => _scanAttempt++);
  }

  Future<void> _select(BleDevice device) async {
    ref.read(selectedDongleProvider.notifier).select(device);
    await ref.read(scanViewModelProvider.notifier).stopScan();
    if (mounted) unawaited(context.push(AppRoutes.connecting));
  }

  /// Rótulo de intensidade a partir do RSSI (dBm; perto de 0 = mais forte).
  String _signalLabel(int rssi) {
    if (rssi >= -60) return 'sinal forte';
    if (rssi >= -75) return 'sinal médio';
    return 'sinal fraco';
  }

  @override
  Widget build(BuildContext context) {
    final state = ref.watch(scanViewModelProvider);

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
                  if (!state.adapterOn)
                    Text(
                      'Ligue o Bluetooth para procurar o dongle.',
                      style: AppTypography.ui(
                        const TextStyle(
                          fontSize: 13,
                          fontWeight: FontWeight.w500,
                          color: AppColors.amber500,
                        ),
                      ),
                    )
                  else if (state.failure != null)
                    Text(
                      'Falha na busca. Tente novamente.',
                      style: AppTypography.ui(
                        const TextStyle(
                          fontSize: 13,
                          fontWeight: FontWeight.w500,
                          color: AppColors.red200,
                        ),
                      ),
                    )
                  else
                    const _SearchingLabel(),
                  const SizedBox(height: AppSpacing.s5),
                  Center(child: RadarScanner(key: ValueKey(_scanAttempt))),
                  const SizedBox(height: AppSpacing.s7),
                  Text('ENCONTRADOS', style: AppTypography.overline),
                  const SizedBox(height: AppSpacing.s4),
                  if (state.devices.isEmpty)
                    Text(
                      'Nenhum dongle encontrado ainda…',
                      style: AppTypography.ui(
                        const TextStyle(
                          fontSize: 13,
                          fontWeight: FontWeight.w400,
                          color: AppColors.textTertiary,
                        ),
                      ),
                    )
                  else
                    for (final device in state.devices) ...[
                      CardButton(
                        leading: const IconTile(AppIconData.dongle),
                        title: device.displayName,
                        titleMono: true,
                        subtitle: 'ELM327 · ${_signalLabel(device.rssi)}',
                        subtitleColor: AppColors.cyan500,
                        subtitleMono: false,
                        showValue: false,
                        selected: true,
                        onTap: () => _select(device),
                      ),
                      const SizedBox(height: AppSpacing.s3),
                    ],
                ],
              ),
            ),
            const SizedBox(height: AppSpacing.s5),
            AppButton(
              variant: AppButtonVariant.secondary,
              onPressed: _rescan,
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
