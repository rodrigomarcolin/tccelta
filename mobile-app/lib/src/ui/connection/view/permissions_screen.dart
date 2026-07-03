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
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_background.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_state_view.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// 1.1 — Pedir permissões de Bluetooth (e Localização no Android antigo).
///
/// Ao iniciar, o view model verifica se a permissão já foi concedida: se sim, a
/// tela é pulada e o fluxo avança direto (busca ou "BT desligado", conforme o
/// adaptador). Só quando ainda não há permissão a UI é renderizada; "Permitir"
/// dispara o pedido real e, concedido, segue adiante.
class PermissionsScreen extends ConsumerWidget {
  /// Cria a tela de permissões.
  const PermissionsScreen({super.key});

  /// Avança para a busca ou para "BT desligado" conforme o estado atual do
  /// adaptador. Usa `pushReplacement` para não deixar a tela de permissão na
  /// pilha (a permissão já foi resolvida).
  Future<void> _advance(BuildContext context, WidgetRef ref) async {
    final adapter = await ref.read(dongleRepositoryProvider).adapterState.first;
    if (!context.mounted) return;
    context.pushReplacement(
      adapter == BleAdapterState.on ? AppRoutes.scan : AppRoutes.bluetoothOff,
    );
  }

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
    }
    // A navegação em caso de sucesso é feita pelo `ref.listen` no `build`
    // (mesmo caminho da checagem inicial "já concedido").
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(permissionsViewModelProvider);

    // Concedido (pela checagem inicial ou após o pedido) => avança e sai.
    ref.listen<PermissionFlowState>(permissionsViewModelProvider, (_, next) {
      if (next == PermissionFlowState.granted) {
        unawaited(_advance(context, ref));
      }
    });

    // Enquanto verifica (ou já concedeu, aguardando o avanço) não mostramos a
    // UI de permissão — evita "piscar" a tela para quem já concedeu.
    if (state == PermissionFlowState.checking ||
        state == PermissionFlowState.granted) {
      return const ConnectionBackground(
        child: Center(
          child: CircularProgressIndicator(color: AppColors.cyan500),
        ),
      );
    }

    final requesting = state == PermissionFlowState.requesting;

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
