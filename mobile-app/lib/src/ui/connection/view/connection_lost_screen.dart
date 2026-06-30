import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_state_view.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// 1.6 — Conexão perdida (queda do link / dongle fora de alcance).
///
/// Mostra o último valor lido marcado como obsoleto. "Reconectar" reinicia o
/// handshake; "Esquecer dispositivo" volta ao início do fluxo.
class ConnectionLostScreen extends StatelessWidget {
  /// Cria a tela de conexão perdida.
  const ConnectionLostScreen({super.key});

  @override
  Widget build(BuildContext context) {
    return ConnectionStateView(
      tone: StatusTone.alert,
      icon: AppIconData.desconectar,
      title: 'Conexão perdida',
      description: 'O dongle saiu de alcance ou foi desplugado. Os valores '
          'abaixo são os últimos lidos.',
      centerExtra: const StatusBadge(
        label: 'RPM 2.480 · desatualizado',
        tone: StatusTone.alert,
      ),
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
        onPressed: () => context.go(AppRoutes.permissions),
        child: const Text('Esquecer dispositivo'),
      ),
    );
  }
}
