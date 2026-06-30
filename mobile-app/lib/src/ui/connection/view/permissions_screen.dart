import 'dart:io';

import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_state_view.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// 1.1 — Pedir permissões de Bluetooth (e Localização no Android antigo).
///
/// Conceder leva ao próximo passo do fluxo. Como a camada BLE ainda é mockada,
/// "Permitir" segue para a checagem de Bluetooth (no tour, mostra o estado de
/// BT desligado).
class PermissionsScreen extends StatelessWidget {
  /// Cria a tela de permissões.
  const PermissionsScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return ConnectionStateView(
      icon: AppIconData.bluetooth,
      title: 'Permitir Bluetooth',
      description: 'O app usa Bluetooth para encontrar e conversar com o seu '
          'dongle OBD2. No Android antigo, o sistema pode exigir também a '
          'localização.',
      bottomExtra: Column(
        children: [
          const CardButton(
            leading: IconTile(AppIconData.bluetooth),
            title: 'Bluetooth',
            subtitle: 'Localizar e conectar ao dongle',
            subtitleMono: false,
            showValue: false,
            showChevron: false,
          ),
          // A localização só é exigida pelo Android para escanear BLE.
          if (Platform.isAndroid) ...[
            const SizedBox(height: AppSpacing.s3),
            const CardButton(
              leading: IconTile(
                AppIconData.local,
                background: AppColors.track,
                iconColor: AppColors.textSecondary,
              ),
              title: 'Localização',
              subtitle: 'Exigida pelo sistema p/ escanear BLE',
              subtitleMono: false,
              showValue: false,
              showChevron: false,
            ),
          ],
        ],
      ),
      primaryAction: AppButton(
        onPressed: () => context.push(AppRoutes.bluetoothOff),
        child: const Text('Permitir'),
      ),
      secondaryAction: AppButton(
        variant: AppButtonVariant.link,
        onPressed: () {},
        child: const Text('Por que isso é necessário?'),
      ),
    );
  }
}
