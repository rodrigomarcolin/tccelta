import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/actions/forget_dongle_action.dart';
import 'package:tccelta_mobile/src/ui/connection/view_model/connecting_view_model.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_state_view.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// 1.6 — Conexão perdida (queda do link / dongle fora de alcance).
///
/// "Reconectar" reinicia o handshake com o mesmo dongle; "Esquecer
/// dispositivo" limpa a seleção, encerra a conexão e volta ao início.
class ConnectionLostScreen extends ConsumerWidget {
  /// Cria a tela de conexão perdida.
  const ConnectionLostScreen({super.key});

  Future<void> _forget(BuildContext context, WidgetRef ref) async {
    await forgetDongle(ref);
    if (context.mounted) context.go(AppRoutes.blePermissions);
  }

  Future<void> _repair(BuildContext context, WidgetRef ref) async {
    final viewModel = ref.read(connectingViewModelProvider.notifier);
    await viewModel.repair();
    if (!context.mounted) return;
    final next = ref.read(connectingViewModelProvider).security;
    // Android: o bond antigo saiu e a conexão replaneja sozinha; iOS: orienta.
    context.pushReplacement(
      next == LinkSecurityStep.forgetOnIos
          ? AppRoutes.pairing
          : AppRoutes.connecting,
    );
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final error = ref.watch(
      connectingViewModelProvider.select((s) => s.linkError),
    );
    final pairingLost = error?.isPairingLost ?? false;
    return ConnectionStateView(
      tone: StatusTone.alert,
      icon: AppIconData.desconectar,
      title: pairingLost
          ? 'O dongle não reconhece mais este aparelho'
          : 'Conexão perdida',
      description: pairingLost
          ? 'O botão do dongle foi pressionado ou o pareamento foi apagado.'
          : 'O dongle saiu de alcance ou foi desplugado. Toque em '
                'reconectar para tentar novamente.',
      primaryAction: AppButton(
        icon: const AppIcon(
          AppIconData.recarregar,
          size: 20,
          color: AppColors.accentOn,
        ),
        onPressed: pairingLost
            ? () => unawaited(_repair(context, ref))
            : () => context.pushReplacement(AppRoutes.connecting),
        child: Text(pairingLost ? 'Parear novamente' : 'Reconectar'),
      ),
      secondaryAction: AppButton(
        variant: AppButtonVariant.link,
        onPressed: () => _forget(context, ref),
        child: const Text('Esquecer dispositivo'),
      ),
    );
  }
}
