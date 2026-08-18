import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_hooks/flutter_hooks.dart';
import 'package:go_router/go_router.dart';
import 'package:hooks_riverpod/hooks_riverpod.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_device.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_signal_level.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/connection/view_model/scan_view_model.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_background.dart';
import 'package:tccelta_mobile/src/ui/core/hooks/hooks.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// 1.3 — Procurar dongles BLE próximos.
///
/// Escaneia dongles `OBD2Dongle` reais e lista os encontrados. Tocar num
/// dongle o seleciona e inicia a conexão; "Procurar novamente" reinicia a
/// varredura (e a animação do radar).
class ScanScreen extends HookConsumerWidget {
  /// Cria a tela de busca.
  const ScanScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // Trocar a key remonta o radar (reinicia a varredura) ao buscar de novo.
    final scanAttempt = useState(0);

    // Dispara o scan uma única vez ao montar. O callback do useEffect roda
    // DURANTE o build, então adiamos a mutação do provider para o pós-frame
    // (equivale ao antigo initState + addPostFrameCallback).
    useEffect(
      () {
        WidgetsBinding.instance.addPostFrameCallback((_) {
          unawaited(ref.read(scanViewModelProvider.notifier).startScan());
        });
        return null;
      },
      const [],
    );

    Future<void> rescan() async {
      unawaited(ref.read(scanViewModelProvider.notifier).startScan());
      scanAttempt.value++;
    }

    Future<void> select(BleDevice device) async {
      ref.read(selectedDongleProvider.notifier).select(device);
      await ref.read(scanViewModelProvider.notifier).stopScan();
      if (context.mounted) unawaited(context.push(AppRoutes.pskSetup));
    }

    final state = ref.watch(scanViewModelProvider);

    return Scaffold(
      backgroundColor: AppColors.bgScreen,
      appBar: AppBar(
        backgroundColor: AppColors.bgScreen,
        elevation: 0,
        title: Text(
          'OBD-II Dongle',
          style: AppTypography.ui(
            const TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w600,
              color: AppColors.textPrimary,
            ),
          ),
        ),
        centerTitle: false,
      ),
      body: ConnectionBackground(
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
                    else if (state.isScanning)
                      const _SearchingLabel()
                    else
                      const SizedBox.shrink(),
                    const SizedBox(height: AppSpacing.s5),
                    Center(
                      child: RadarScanner(
                        key: ValueKey(scanAttempt.value),
                        active: state.isScanning,
                      ),
                    ),
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
                          subtitle: 'ELM327 · ${device.signal.label}',
                          subtitleColor: AppColors.cyan500,
                          subtitleMono: false,
                          showValue: false,
                          selected: true,
                          // BT desligado: nada a conectar — desabilita o toque
                          // (onTap nulo) e esmaece o card.
                          dimmed: !state.adapterOn,
                          onTap: state.adapterOn ? () => select(device) : null,
                        ),
                        const SizedBox(height: AppSpacing.s3),
                      ],
                  ],
                ),
              ),
              const SizedBox(height: AppSpacing.s5),
              AppButton(
                variant: AppButtonVariant.secondary,
                onPressed: !state.isScanning && state.adapterOn ? rescan : null,
                child: const Text('Procurar novamente'),
              ),
              const SizedBox(height: AppSpacing.s7),
            ],
          ),
        ),
      ),
    );  }
}

/// Rótulo pt-BR de exibição do nível de sinal (a classificação vem do domain,
/// aplicada no datasource — aqui a tela só traduz para texto).
extension on BleSignalLevel {
  String get label => switch (this) {
    BleSignalLevel.strong => 'sinal forte',
    BleSignalLevel.medium => 'sinal médio',
    BleSignalLevel.weak => 'sinal fraco',
  };
}

/// Rótulo "Procurando…" com três pontinhos piscando em sequência.
class _SearchingLabel extends HookWidget {
  const _SearchingLabel();

  @override
  Widget build(BuildContext context) {
    final controller = useLoopController(const Duration(milliseconds: 1200));
    return Row(
      children: [
        for (var i = 0; i < 3; i++) ...[
          if (i > 0) const SizedBox(width: 3),
          _Dot(controller: controller, phase: i * 0.2),
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
