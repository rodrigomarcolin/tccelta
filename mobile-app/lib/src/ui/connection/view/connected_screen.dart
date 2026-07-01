import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/connection/view_model/connecting_view_model.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_state_view.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// 1.5 — Conectado ao dongle (transporte BLE pronto).
///
/// Fase 1: confirma o link BLE. Protocolo e PIDs disponíveis são preenchidos na
/// Fase 2 (ELM327), por isso aparecem como placeholder. Se o link cair aqui,
/// segue para Conexão perdida.
class ConnectedScreen extends ConsumerWidget {
  /// Cria a tela de conectado.
  const ConnectedScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final device = ref.watch(selectedDongleProvider);

    // Perda de conexão em sessão ativa -> Conexão perdida. Só reagimos ao
    // desfecho TERMINAL: o adapter tenta se reconectar sozinho (fase
    // `reconnecting`) e, se conseguir, volta a `ready` sem sair desta tela.
    ref.listen(connectingViewModelProvider, (_, next) {
      if (!context.mounted) return;
      if (next == BleConnectionPhase.failed ||
          next == BleConnectionPhase.disconnected) {
        context.pushReplacement(AppRoutes.connectionLost);
      }
    });

    return ConnectionStateView(
      icon: AppIconData.check,
      badgeShape: IconBadgeShape.circle,
      pulse: true,
      title: 'Conectado',
      centerExtra: SizedBox(
        width: double.infinity,
        child: Column(
          children: [
            Text(
              device?.name ?? 'OBD2Dongle',
              style: AppTypography.mono(
                const TextStyle(
                  fontSize: 15,
                  fontWeight: FontWeight.w500,
                  color: AppColors.textSecondary,
                ),
              ),
            ),
            const SizedBox(height: AppSpacing.s7),
            const StatCard.info(
              label: 'Canal',
              value: 'Nordic UART (BLE)',
            ),
            const SizedBox(height: AppSpacing.s3),
            // Preenchidos na Fase 2 (init ELM327 + leitura de capacidades).
            const StatCard.info(
              label: 'Protocolo',
              value: '—',
            ),
            const SizedBox(height: AppSpacing.s3),
            const StatCard.info(
              label: 'Sensores disponíveis',
              value: '—',
            ),
          ],
        ),
      ),
      primaryAction: AppButton(
        onPressed: () => context.go(AppRoutes.painel),
        child: const Text('Abrir painel'),
      ),
    );
  }
}
