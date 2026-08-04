import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
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
    ref.read(selectedDongleProvider.notifier).clear();
    await ref.read(dongleRepositoryProvider).disconnect();
    if (context.mounted) context.go(AppRoutes.permissions);
  }

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    return ConnectionStateView(
      tone: StatusTone.alert,
      icon: AppIconData.desconectar,
      title: 'Conexão perdida',
      description: 'O dongle saiu de alcance ou foi desplugado. Toque em '
          'reconectar para tentar novamente.',
      primaryAction: AppButton(
        icon: const AppIcon(
          AppIconData.recarregar,
          size: 20,
          color: AppColors.accentOn,
        ),
        onPressed: () => context.pushReplacement(AppRoutes.connecting),
        child: const Text('Reconectar'),
      ),
      secondaryAction: AppButton(
        variant: AppButtonVariant.link,
        onPressed: () => _forget(context, ref),
        child: const Text('Esquecer dispositivo'),
      ),
    );
  }
}
