import 'dart:async';
import 'dart:io';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_adapter_state.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/connection/view_model/permissions_view_model.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_state_view.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// 1.1 — Pedir permissões de Bluetooth (e Localização no Android antigo).
///
/// "Permitir" dispara o pedido real de permissões; concedido, segue para a
/// busca (ou para o estado de BT desligado, conforme o adaptador).
class PermissionsScreen extends ConsumerWidget {
  /// Cria a tela de permissões.
  const PermissionsScreen({super.key});

  Future<void> _onAllow(BuildContext context, WidgetRef ref) async {
    final granted =
        await ref.read(permissionsViewModelProvider.notifier).request();
    if (!context.mounted) return;
    if (!granted) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Permissão de Bluetooth necessária para continuar.'),
        ),
      );
      return;
    }
    // Decide entre busca e "BT desligado" conforme o estado atual do adaptador.
    final adapter = await ref.read(dongleRepositoryProvider).adapterState.first;
    if (!context.mounted) return;
    unawaited(
      context.push(
        adapter == BleAdapterState.on ? AppRoutes.scan : AppRoutes.bluetoothOff,
      ),
    );
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final requesting = ref.watch(permissionsViewModelProvider) ==
        PermissionFlowState.requesting;

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
        onPressed: requesting ? null : () => _onAllow(context, ref),
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
