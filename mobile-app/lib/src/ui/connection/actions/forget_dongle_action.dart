import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/application/connection/forget_dongle_use_case.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// Limpa a seleção em memória, desconecta e apaga o dongle persistido
/// ([forgetDongleUseCaseProvider]). Sem diálogo/navegação — cada ponto de
/// entrada (Configurações, faixa de status, conexão perdida) decide isso
/// conforme seu próprio contexto.
Future<ForgetDongleResult> forgetDongle(WidgetRef ref) async {
  ref.read(selectedDongleProvider.notifier).clear();
  return ref.read(forgetDongleUseCaseProvider).call();
}

/// Confirma com o usuário (diálogo padrão do design system) e, se
/// confirmado, esquece o dongle e volta para a busca.
Future<void> confirmAndForgetDongle(BuildContext context, WidgetRef ref) async {
  final confirmed = await showConfirmDialog(
    context,
    title: 'Esquecer dispositivo?',
    message:
        'Você vai precisar buscar e parear o dongle novamente na próxima '
        'vez.',
    confirmLabel: 'Esquecer',
  );
  if (confirmed != true) return;
  final result = await forgetDongle(ref);
  if (!context.mounted) return;
  if (result.needsManualForget) {
    await showConfirmDialog(
      context,
      title: 'Esqueça também no iPhone',
      message:
          'Abra Ajustes › Bluetooth, toque em (i) ao lado do dongle e em '
          'Esquecer este dispositivo. Sem isso, o iPhone não consegue parear '
          'de novo.',
      confirmLabel: 'Entendi',
    );
  }
  if (context.mounted) context.go(AppRoutes.blePermissions);
}
