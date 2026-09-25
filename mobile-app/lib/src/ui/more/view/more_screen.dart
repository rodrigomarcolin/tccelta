import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/router/app_tab_navigation.dart';
import 'package:tccelta_mobile/src/ui/connection/actions/forget_dongle_action.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/diagnostics/view_model/dtc_view_model.dart';
import 'package:tccelta_mobile/src/ui/diagnostics/widgets/dtc_tab_badge.dart';
import 'package:tccelta_mobile/src/ui/settings/settings_providers.dart';

/// Tela de opções — a aba "Mais" da tab bar.
///
/// Lista as opções do app; hoje só "Criptografia", que abre a tela de
/// configuração da PSK (rota [AppRoutes.settings]).
class MoreScreen extends ConsumerWidget {
  /// Cria a tela de opções.
  const MoreScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final pskAsync = ref.watch(pskProvider);
    final isActive = (pskAsync.asData?.value ?? '').isNotEmpty;
    final activeDtcCount = ref.watch(
      dtcViewModelProvider.select((s) => s.activeCount),
    );

    return Scaffold(
      body: SafeArea(
        bottom: false,
        child: ListView(
          padding: const EdgeInsets.fromLTRB(
            AppSpacing.s7,
            AppSpacing.s7,
            AppSpacing.s7,
            AppSpacing.s9,
          ),
          children: [
            Text('Mais', style: AppTypography.heading),
            const SizedBox(height: AppSpacing.s2),
            Text(
              'Opções e configurações do app',
              style: AppTypography.body.copyWith(
                color: AppColors.textTertiary,
              ),
            ),
            const SizedBox(height: AppSpacing.s7),
            CardButton(
              icon: AppIconData.cadeado,
              iconColor: isActive ? AppColors.cyan500 : AppColors.neutral600,
              title: 'Criptografia',
              subtitle: isActive
                  ? 'AES-256-GCM · chave configurada'
                  : 'Comunicação em texto puro',
              showValue: false,
              onTap: () => context.push(AppRoutes.settings),
            ),
            const SizedBox(height: AppSpacing.s3),
            CardButton(
              icon: AppIconData.desconectar,
              iconColor: AppColors.red500,
              title: 'Esquecer dispositivo',
              subtitle:
                  ref.watch(selectedDongleProvider)?.displayName ??
                  'Nenhum dispositivo',
              subtitleMono: false,
              showValue: false,
              onTap: () => confirmAndForgetDongle(context, ref),
            ),
          ],
        ),
      ),
      bottomNavigationBar: AppTabBar(
        active: 'mais',
        tabs: tabsWithDtcBadge(activeDtcCount),
        onChanged: (key) => goToAppTab(context, key),
      ),
    );
  }
}
