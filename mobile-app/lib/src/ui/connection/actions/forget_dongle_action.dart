import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// Limpa a seleção em memória, desconecta e apaga o dongle persistido
/// ([forgetDongleUseCaseProvider]). Sem diálogo/navegação — cada ponto de
/// entrada (Configurações, faixa de status, conexão perdida) decide isso
/// conforme seu próprio contexto.
Future<void> forgetDongle(WidgetRef ref) async {
  ref.read(selectedDongleProvider.notifier).clear();
  await ref.read(forgetDongleUseCaseProvider).call();
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
  await forgetDongle(ref);
  if (context.mounted) context.go(AppRoutes.blePermissions);
}
