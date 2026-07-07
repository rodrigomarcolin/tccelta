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

  /// Quantos cards esqueleto mostrar antes de a descoberta revelar quais PIDs o
  /// veículo suporta (fase inicial, sem rótulos conhecidos ainda).
  static const int _skeletonCount = 6;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final state = ref.watch(telemetryViewModelProvider);
    final byPid = {for (final r in state.readings) r.pid: r};
    final supported = state.supportedPids;
    // Só os PIDs suportados (ordem estável do enum); se a descoberta ainda não
    // rodou, mostra todos os curados como fallback.
    final pids = Obd2Pid.values
        .where((p) => supported.isEmpty || supported.contains(p))
        .toList(growable: false);

    // "Inicializando" = ainda não houve nenhuma leitura e não há falha. Um `—`
    // aqui significaria "sem dado", que é ambíguo; então mostramos skeletons.
    // Uma falha com leituras vazias já é "sem dados" (não loading).
    final isInitializing = state.readings.isEmpty && state.failure == null;

    // Durante a inicialização mostramos skeletons em vez do conjunto de
    // fallback (todos os curados), evitando o "pulo" de layout quando a
    // descoberta encolhe a grade para os PIDs suportados: rótulos reais quando
    // já os conhecemos, senão um número fixo de placeholders neutros.
    final List<Widget> cards;
    if (isInitializing) {
      cards = supported.isEmpty
          ? [
              for (var i = 0; i < _skeletonCount; i++)
                const StatCard.value(label: '', loading: true),
            ]
          : [
              for (final pid in pids)
                StatCard.value(label: pid.label, loading: true),
            ];
    } else {
      cards = [
        for (final pid in pids)
          StatCard.value(
            label: pid.label,
            value: byPid[pid]?.value.round(),
            unit: pid.unit,
          ),
      ];
    }

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
                  CardGrid(children: cards),
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
