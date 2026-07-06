import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/telemetry_view_model.dart';
import 'package:tccelta_mobile/src/ui/telemetry/widgets/telemetry_status_band.dart';

/// Painel de telemetria OBD-II — a tela `/painel`.
///
/// View "burra": observa o [telemetryViewModelProvider] e renderiza cada PID
/// lido num [StatCard.value]. PIDs ainda sem leitura mostram `—`
/// (o [StatCard] já trata `value` nulo). O `ConnectionGuard` global protege a
/// rota; se o link cair, ele redireciona para "Conexão perdida".
class PainelScreen extends ConsumerWidget {
  /// Cria o painel.
  const PainelScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(telemetryViewModelProvider);
    final byPid = {for (final r in state.readings) r.pid: r};

    return Scaffold(
      body: SafeArea(
        bottom: false,
        child: Column(
          children: [
            const TelemetryStatusBand(),
            Expanded(
              child: ListView(
                padding: const EdgeInsets.fromLTRB(
                  AppSpacing.s7,
                  AppSpacing.s7,
                  AppSpacing.s7,
                  AppSpacing.s9,
                ),
                children: [
                  Text('Painel', style: AppTypography.heading),
                  const SizedBox(height: AppSpacing.s2),
                  Text(
                    'Leituras OBD-II em tempo real',
                    style: AppTypography.body.copyWith(
                      color: AppColors.textTertiary,
                    ),
                  ),
                  const SizedBox(height: AppSpacing.s7),
                  CardGrid(
                    children: [
                      for (final pid in Obd2Pid.values)
                        StatCard.value(
                          label: pid.label,
                          value: byPid[pid]?.value.round(),
                          unit: pid.unit,
                        ),
                    ],
                  ),
                ],
              ),
            ),
          ],
        ),
      ),
      bottomNavigationBar: const AppTabBar(),
    );
  }
}
