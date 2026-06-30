import 'package:app_settings/app_settings.dart';
import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_state_view.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// 1.2 — Bluetooth do telefone desligado.
///
/// "Abrir ajustes do sistema" abre direto a tela de Bluetooth do dispositivo,
/// onde o usuário pode religar o BT; "Tentar novamente" volta para a busca.
class BluetoothOffScreen extends StatelessWidget {
  /// Cria a tela de Bluetooth desligado.
  const BluetoothOffScreen({super.key});

  /// Abre a tela de Bluetooth do sistema. Em `asAnotherTask` para que os
  /// ajustes subam em uma task própria, sem prender a navegação do app. Se a
  /// chamada nativa falhar (ex.: emulador sem essa tela), cai num aviso.
  Future<void> _openBluetoothSettings(BuildContext context) async {
    try {
      await AppSettings.openAppSettings(
        type: AppSettingsType.bluetooth,
        asAnotherTask: true,
      );
    } on Object {
      if (!context.mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Abra os ajustes de Bluetooth do sistema.'),
        ),
      );
    }
  }

  @override
  Widget build(BuildContext context) {
    return ConnectionStateView(
      tone: StatusTone.warning,
      icon: AppIconData.bluetooth,
      title: 'Bluetooth está desligado',
      description:
          'Ligue o Bluetooth do telefone para procurar o seu dongle OBD2.',
      bottomExtra: const Callout(
        text: 'O dongle continua anunciando sozinho — assim que o BT voltar, '
            'ele reaparece na busca.',
      ),
      primaryAction: AppButton(
        variant: AppButtonVariant.warning,
        onPressed: () => _openBluetoothSettings(context),
        child: const Text('Abrir ajustes do sistema'),
      ),
      secondaryAction: AppButton(
        variant: AppButtonVariant.link,
        onPressed: () => context.push(AppRoutes.scan),
        child: const Text('Tentar novamente'),
      ),
    );
  }
}
