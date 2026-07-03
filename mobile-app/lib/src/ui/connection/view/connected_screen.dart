import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
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

    // A perda de conexão em sessão ativa -> Conexão perdida agora é tratada de
    // forma global pelo `ConnectionGuard` (montado no topo em `main.dart`), que
    // cobre também o painel e demais telas pós-conexão.
    return ConnectionStateView(
      icon: AppIconData.check,
      badgeShape: IconBadgeShape.circle,
      pulse: true,
      title: 'Conectado',
      centerExtra: SizedBox(
        width: double.infinity,
        child: Column(
          children: [
            if (device?.displayName.isNotEmpty ?? false) ...[
              Text(
                device!.displayName,
                style: AppTypography.mono(
                  const TextStyle(
                    fontSize: 15,
                    fontWeight: FontWeight.w500,
                    color: AppColors.textSecondary,
                  ),
                ),
              ),
              const SizedBox(height: AppSpacing.s7),
            ],
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
