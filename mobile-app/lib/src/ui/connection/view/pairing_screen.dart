import 'dart:async';

import 'package:app_settings/app_settings.dart';
import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:hooks_riverpod/hooks_riverpod.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/view/connecting_screen.dart';
import 'package:tccelta_mobile/src/ui/connection/view_model/connecting_view_model.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_state_view.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// Orientação de pareamento do dongle: segurar o botão, janela expirada,
/// pareamento recusado e, no iOS, esquecer o dispositivo nos Ajustes.
///
/// É só outra *view* do estado do `ConnectingViewModel`; quando o fluxo volta a
/// conectar, retorna para a tela de conexão.
class PairingScreen extends ConsumerWidget {
  /// Cria a tela de pareamento.
  const PairingScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(connectingViewModelProvider);
    final viewModel = ref.read(connectingViewModelProvider.notifier);

    ref.listen(connectingViewModelProvider, (_, next) {
      if (!context.mounted) return;
      if (next.security == LinkSecurityStep.failed &&
          next.phase == BleConnectionPhase.failed) {
        context.pushReplacement(AppRoutes.connectionLost);
      } else if (!needsUserAction(next.security) &&
          next.security != LinkSecurityStep.idle) {
        context.pushReplacement(AppRoutes.connecting);
      }
    });

    Future<void> cancel() async {
      await viewModel.cancel();
      if (context.mounted) context.go(AppRoutes.scan);
    }

    final cancelButton = AppButton(
      variant: AppButtonVariant.link,
      onPressed: () => unawaited(cancel()),
      child: const Text('Cancelar'),
    );

    switch (state.security) {
      case LinkSecurityStep.windowExpired:
        return ConnectionStateView(
          tone: StatusTone.warning,
          icon: AppIconData.desconectar,
          title: 'O tempo para parear acabou',
          description:
              'A janela de pareamento do dongle fechou antes de o '
              'pareamento terminar. Segure o botão do dongle por 5 segundos '
              'e tente de novo.',
          primaryAction: AppButton(
            onPressed: () => unawaited(viewModel.retry()),
            child: const Text('Segurar o botão de novo'),
          ),
          secondaryAction: cancelButton,
        );
      case LinkSecurityStep.rejected:
        return ConnectionStateView(
          tone: StatusTone.alert,
          icon: AppIconData.desconectar,
          title: 'Pareamento recusado',
          description:
              'O pareamento não foi concluído. Confirme o pedido do '
              'sistema quando ele aparecer.',
          primaryAction: AppButton(
            onPressed: () => unawaited(viewModel.retry()),
            child: const Text('Tentar de novo'),
          ),
          secondaryAction: cancelButton,
        );
      case LinkSecurityStep.forgetOnIos:
        return ConnectionStateView(
          tone: StatusTone.warning,
          icon: AppIconData.bluetooth,
          title: 'Este iPhone guarda um pareamento antigo',
          description: 'O dongle não reconhece mais este iPhone.',
          bottomExtra: const StepList([
            ConnectStep('Abra Ajustes › Bluetooth', ConnectStepState.pending),
            ConnectStep(
              'Toque em (i) ao lado de TCCeltaDongle',
              ConnectStepState.pending,
            ),
            ConnectStep(
              'Toque em Esquecer este dispositivo',
              ConnectStepState.pending,
            ),
            ConnectStep(
              'Volte e segure o botão do dongle por 5 s',
              ConnectStepState.pending,
            ),
          ]),
          primaryAction: AppButton(
            onPressed: () => unawaited(
              AppSettings.openAppSettings(type: AppSettingsType.bluetooth),
            ),
            child: const Text('Abrir Ajustes'),
          ),
          secondaryAction: AppButton(
            variant: AppButtonVariant.link,
            onPressed: viewModel.confirmForgotten,
            child: const Text('Já esqueci'),
          ),
        );
      case LinkSecurityStep.waitingButton:
      case LinkSecurityStep.idle:
      case LinkSecurityStep.planning:
      case LinkSecurityStep.pairing:
      case LinkSecurityStep.restoring:
      case LinkSecurityStep.secured:
      case LinkSecurityStep.removingBond:
      case LinkSecurityStep.failed:
        final status = state.dongleSeen
            ? 'Dongle encontrado — aguardando o botão'
            : (state.watching ? 'Procurando o dongle…' : 'Dongle não visto');
        return ConnectionStateView(
          tone: StatusTone.warning,
          icon: AppIconData.dongle,
          title: 'Coloque o dongle em modo de pareamento',
          centerExtra: StatusBadge(
            label: status.toUpperCase(),
            tone: state.dongleSeen ? StatusTone.ok : StatusTone.warning,
            pulse: state.watching,
          ),
          bottomExtra: const StepList([
            ConnectStep(
              'Com o dongle ligado, segure o botão por 5 segundos',
              ConnectStepState.active,
            ),
            ConnectStep(
              'Solte; o dongle fica pareável por 45 segundos',
              ConnectStepState.pending,
            ),
            ConnectStep(
              'O pareamento começa sozinho e o sistema pode pedir confirmação',
              ConnectStepState.pending,
            ),
          ]),
          primaryAction: state.watching
              ? cancelButton
              : AppButton(
                  onPressed: () => unawaited(viewModel.retry()),
                  child: const Text('Tentar de novo'),
                ),
          secondaryAction: state.canManageBonds
              ? (state.watching ? null : cancelButton)
              : AppButton(
                  variant: AppButtonVariant.link,
                  onPressed: () => unawaited(viewModel.assumePaired()),
                  child: const Text('Já pareei este iPhone'),
                ),
        );
    }
  }
}
