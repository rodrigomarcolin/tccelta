import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_state_view.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// 1.5 — Conectado ao dongle.
///
/// Estado de sucesso: badge pulsante, identidade do dongle e infos do
/// protocolo. "Abrir painel" leva ao painel ao vivo (hoje a `ShowcaseScreen`).
class ConnectedScreen extends StatelessWidget {
  /// Cria a tela de conectado.
  const ConnectedScreen({super.key});

  @override
  Widget build(BuildContext context) {
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
              'OBD2Dongle · ELM327 v1.5',
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
              label: 'Protocolo',
              value: 'ISO 15765-4 CAN',
            ),
            const SizedBox(height: AppSpacing.s3),
            const StatCard.info(
              label: 'Sensores disponíveis',
              value: '12 PIDs',
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
