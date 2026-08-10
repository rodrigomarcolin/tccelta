import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/connection/view_model/connected_view_model.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_state_view.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// 1.5 — Conectado ao dongle (transporte BLE pronto).
///
/// Fase 1 confirma o link BLE; ao montar, o [ConnectedViewModel] sonda o
/// adaptador (Fase 2: init ELM327 + descoberta de capacidades) e preenche o
/// protocolo e o nº de sensores disponíveis. Se o link cair aqui, segue para
/// Conexão perdida.
class ConnectedScreen extends ConsumerWidget {
  /// Cria a tela de conectado.
  const ConnectedScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final device = ref.watch(selectedDongleProvider);
    final probe = ref.watch(connectedViewModelProvider);
    // Enquanto sonda, mostra '—'; ao terminar, o valor real (ou '—' ausente).
    final protocol = probe.probing ? '—' : (probe.info?.protocol ?? '—');
    final sensores = probe.probing ? '—' : '${probe.supported.length}';

    // A perda de conexão em sessão ativa -> Conexão perdida agora é tratada de
    // forma global pelo `ConnectionGuard` (montado no topo em `main.dart`), que
    // cobre também o painel e demais telas pós-conexão.
    return Scaffold(
      backgroundColor: AppColors.bgScreen,
      appBar: AppBar(
        backgroundColor: AppColors.bgScreen,
        elevation: 0,
        title: Text(
          'Conectado',
          style: AppTypography.ui(
            const TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w600,
              color: AppColors.textPrimary,
            ),
          ),
        ),
        centerTitle: false,
        actions: [
          IconButton(
            icon: const Icon(Icons.settings_outlined, size: 20),
            color: AppColors.textSecondary,
            tooltip: 'Criptografia',
            onPressed: () => context.push(AppRoutes.settings),
          ),
          const SizedBox(width: AppSpacing.s3),
        ],
      ),
      body: ConnectionStateView(
        icon: AppIconData.check,
        badgeShape: IconBadgeShape.circle,
        pulse: true,
        title: 'Conexão Estabelecida',
        centerExtra: SizedBox(
          width: double.infinity,
          child: Column(
            children: [
              if (device?.displayName.isNotEmpty ?? false) ...[
                Text(
                  device!.displayName,
                  style: AppTypography.mono(
                    const TextStyle(
                      fontSize: 15,
                      fontWeight: FontWeight.w500,
                      color: AppColors.textSecondary,
                    ),
                  ),
                ),
                const SizedBox(height: AppSpacing.s7),
              ],
              const StatCard.info(
                label: 'Canal',
                value: 'Nordic UART (BLE)',
              ),
              const SizedBox(height: AppSpacing.s3),
              StatCard.info(
                label: 'Protocolo',
                value: protocol,
              ),
              const SizedBox(height: AppSpacing.s3),
              StatCard.info(
                label: 'Sensores disponíveis',
                value: sensores,
              ),
            ],
          ),
        ),
        primaryAction: AppButton(
          onPressed: () => context.go(AppRoutes.painel),
          child: const Text('Abrir painel'),
        ),
      ),
    );
  }
}
