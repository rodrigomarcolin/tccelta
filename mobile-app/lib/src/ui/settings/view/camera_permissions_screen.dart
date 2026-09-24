import 'dart:async';

import 'package:app_settings/app_settings.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_background.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_state_view.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/settings/view_model/camera_permissions_view_model.dart';

/// Pedir permissão de câmera antes de abrir [destination].
///
/// Ao iniciar, verifica se a permissão já foi concedida — se sim, a tela é
/// pulada e o fluxo avança direto para [destination]. Só quando ainda não há
/// permissão a UI é renderizada; "Permitir" dispara o pedido real e,
/// concedido, segue adiante.
///
/// Quem empilha esta tela usa `context.push<String>` e recebe de volta o
/// resultado devolvido por [destination] quando ele for fechado.
class CameraPermissionsScreen extends ConsumerWidget {
  /// Cria a tela de permissão de câmera, avançando para [destination]
  /// quando concedida.
  const CameraPermissionsScreen({required this.destination, super.key});

  /// Rota aberta assim que a câmera é concedida.
  final String destination;

  /// Abre [destination] e repassa o resultado devolvido por ele para quem
  /// empilhou esta tela.
  Future<void> _advance(BuildContext context) async {
    final result = await context.push<String>(destination);
    if (context.mounted) context.pop(result);
  }

  Future<void> _onAllow(BuildContext context, WidgetRef ref) async {
    final granted = await ref
        .read(cameraPermissionsViewModelProvider.notifier)
        .request();
    if (!context.mounted || granted) return;
    final locked =
        ref.read(cameraPermissionsViewModelProvider) ==
        CameraPermissionFlowState.permanentlyDenied;
    if (!locked) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text(
            'Permissão de câmera necessária para escanear o QR code.',
          ),
        ),
      );
    }
    // A navegação em caso de sucesso é feita pelo `ref.listen` no `build`
    // (mesmo caminho da checagem inicial "já concedido").
  }

  Future<void> _openAppSettings(BuildContext context) async {
    try {
      await AppSettings.openAppSettings(asAnotherTask: true);
    } on Object {
      if (!context.mounted) return;
      ScaffoldMessenger.of(
        context,
      ).showSnackBar(const SnackBar(content: Text('Abra os ajustes do app.')));
    }
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(cameraPermissionsViewModelProvider);

    // Concedido (pela checagem inicial ou após o pedido) => avança e sai.
    ref.listen<CameraPermissionFlowState>(cameraPermissionsViewModelProvider, (
      _,
      next,
    ) {
      if (next == CameraPermissionFlowState.granted) {
        unawaited(_advance(context));
      }
    });

    // Enquanto verifica (ou já concedeu, aguardando o avanço) não mostramos a
    // UI de permissão — evita "piscar" a tela para quem já concedeu.
    if (state == CameraPermissionFlowState.checking ||
        state == CameraPermissionFlowState.granted) {
      return const ConnectionBackground(
        child: Center(
          child: CircularProgressIndicator(color: AppColors.cyan500),
        ),
      );
    }

    final requesting = state == CameraPermissionFlowState.requesting;
    final locked = state == CameraPermissionFlowState.permanentlyDenied;

    return ConnectionStateView(
      tone: StatusTone.warning,
      icon: AppIconData.cadeado,
      title: 'Permitir câmera',
      description:
          'O app usa a câmera só para ler o QR code com a chave PSK do '
          'dongle.',
      primaryAction: locked
          ? AppButton(
              variant: AppButtonVariant.warning,
              onPressed: () => _openAppSettings(context),
              child: const Text('Abrir ajustes do app'),
            )
          : AppButton(
              onPressed: requesting ? null : () => _onAllow(context, ref),
              child: const Text('Permitir'),
            ),
      secondaryAction: AppButton(
        variant: AppButtonVariant.link,
        onPressed: () => context.pop(),
        child: const Text('Digitar a chave manualmente'),
      ),
    );
  }
}
