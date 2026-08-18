import 'package:flutter/material.dart';
import 'package:flutter_hooks/flutter_hooks.dart';
import 'package:hooks_riverpod/hooks_riverpod.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/panel_view_model.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/telemetry_view_model.dart';

/// Abre a lista de sensores empilhada sobre a tela atual (sem navegar para
/// uma rota separada) — um bottom sheet listando todos os PIDs curados, com
/// busca e toque para adicionar/remover do Painel.
Future<void> showSensorPickerSheet(BuildContext context) {
  return showModalBottomSheet<void>(
    context: context,
    isScrollControlled: true,
    backgroundColor: Colors.transparent,
    builder: (context) => const SensorPickerSheet(),
  );
}

/// Conteúdo do sheet de sensores: busca por nome/PID + lista com toque para
/// adicionar (ícone `+`) ou remover com confirmação (ícone de check).
class SensorPickerSheet extends HookConsumerWidget {
  /// Cria o sheet de sensores.
  const SensorPickerSheet({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final query = useState('');
    final telemetry = ref.watch(telemetryViewModelProvider);
    final panel = ref.watch(panelViewModelProvider);
    final byPid = {for (final r in telemetry.readings) r.pid: r};

    final results = _filter(Obd2Pid.values, query.value);

    return FractionallySizedBox(
      heightFactor: 0.88,
      child: DecoratedBox(
        decoration: const BoxDecoration(
          color: AppColors.surfaceCard,
          borderRadius: BorderRadius.vertical(top: Radius.circular(26)),
        ),
        child: SafeArea(
          top: false,
          child: Padding(
            padding: const EdgeInsets.fromLTRB(
              AppSpacing.s7,
              AppSpacing.s4,
              AppSpacing.s7,
              AppSpacing.s4,
            ),
            child: Column(
              children: [
                const _DragHandle(),
                const SizedBox(height: AppSpacing.s4),
                Text('Sensores', style: AppTypography.heading),
                const SizedBox(height: AppSpacing.s4),
                _SearchField(
                  value: query.value,
                  onChanged: (v) => query.value = v,
                ),
                const SizedBox(height: AppSpacing.s4),
                Expanded(
                  child: results.isEmpty
                      ? _NoResults(query: query.value)
                      : ListView.separated(
                          itemCount: results.length,
                          separatorBuilder: (_, _) =>
                              const SizedBox(height: AppSpacing.s2),
                          itemBuilder: (context, i) {
                            final pid = results[i];
                            final added = panel.contains(pid);
                            final reading = byPid[pid];
                            return _SensorTile(
                              pid: pid,
                              added: added,
                              valueStr: reading == null
                                  ? '—'
                                  : reading.value.round().toString(),
                              onToggle: () =>
                                  _onToggle(context, ref, pid, added),
                            );
                          },
                        ),
                ),
                const SizedBox(height: AppSpacing.s4),
                AppButton(
                  onPressed: () => Navigator.of(context).pop(),
                  child: const Text('OK'),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }

  Future<void> _onToggle(
    BuildContext context,
    WidgetRef ref,
    Obd2Pid pid,
    bool added,
  ) async {
    final notifier = ref.read(panelViewModelProvider.notifier);
    if (!added) {
      notifier.addIndicator(pid);
      return;
    }
    final confirmed = await showConfirmDialog(
      context,
      title: 'Remover ${pid.label}?',
      message: 'Esse indicador vai sair do painel.',
      confirmLabel: 'Remover',
    );
    if (confirmed ?? false) notifier.removeIndicator(pid);
  }

  static List<Obd2Pid> _filter(List<Obd2Pid> pids, String query) {
    final q = _normalize(query);
    if (q.isEmpty) return pids;
    return pids
        .where(
          (p) =>
              p.label.toLowerCase().contains(query.toLowerCase()) ||
              _normalize(p.command).contains(q),
        )
        .toList(growable: false);
  }

  /// Normaliza para comparação de código hex: minúsculas, sem espaços.
  static String _normalize(String s) =>
      s.toLowerCase().replaceAll(' ', '').trim();
}

class _DragHandle extends StatelessWidget {
  const _DragHandle();

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 40,
      height: 4,
      decoration: const BoxDecoration(
        color: AppColors.borderStrong,
        borderRadius: AppRadii.brPill,
      ),
    );
  }
}

class _SearchField extends StatelessWidget {
  const _SearchField({required this.value, required this.onChanged});

  final String value;
  final ValueChanged<String> onChanged;

  @override
  Widget build(BuildContext context) {
    return TextField(
      onChanged: onChanged,
      style: AppTypography.ui(
        const TextStyle(fontSize: 14, color: AppColors.textPrimary),
      ),
      decoration: InputDecoration(
        hintText: 'Buscar sensor ou PID…',
        hintStyle: AppTypography.ui(
          const TextStyle(fontSize: 14, color: AppColors.textTertiary),
        ),
        prefixIcon: const Padding(
          padding: EdgeInsets.all(AppSpacing.s3),
          child: AppIcon(
            AppIconData.buscar,
            size: 18,
            color: AppColors.textTertiary,
          ),
        ),
        filled: true,
        fillColor: AppColors.surfaceList,
        contentPadding: const EdgeInsets.symmetric(
          vertical: AppSpacing.s4,
        ),
        border: const OutlineInputBorder(
          borderRadius: AppRadii.brMd,
          borderSide: BorderSide(color: AppColors.borderHairline),
        ),
        enabledBorder: const OutlineInputBorder(
          borderRadius: AppRadii.brMd,
          borderSide: BorderSide(color: AppColors.borderHairline),
        ),
        focusedBorder: const OutlineInputBorder(
          borderRadius: AppRadii.brMd,
          borderSide: BorderSide(color: AppColors.accent, width: 1.5),
        ),
      ),
    );
  }
}

class _NoResults extends StatelessWidget {
  const _NoResults({required this.query});

  final String query;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Text(
        'Nenhum sensor encontrado para "$query".',
        textAlign: TextAlign.center,
        style: AppTypography.body.copyWith(color: AppColors.textTertiary),
      ),
    );
  }
}

/// Linha de um sensor na lista: nome + código (via [CardButton]), última
/// leitura + unidade, e um alternador quadrado à direita — check (ciano)
/// quando já adicionado, `+` quando não. O card inteiro é o alvo de toque
/// (adiciona, ou pede confirmação para remover).
class _SensorTile extends StatelessWidget {
  const _SensorTile({
    required this.pid,
    required this.added,
    required this.valueStr,
    required this.onToggle,
  });

  final Obd2Pid pid;
  final bool added;
  final String valueStr;
  final VoidCallback onToggle;

  @override
  Widget build(BuildContext context) {
    return CardButton(
      key: ValueKey('sensor_toggle_${pid.name}'),
      title: pid.label,
      subtitle: pid.command,
      value: valueStr,
      unit: pid.unit,
      showChevron: false,
      trailing: _ToggleSquare(added: added),
      onTap: onToggle,
    );
  }
}

/// Indicador quadrado puramente visual — o toque é tratado pelo `onTap` do
/// [CardButton] que o contém (a linha inteira é o alvo de toque).
class _ToggleSquare extends StatelessWidget {
  const _ToggleSquare({required this.added});

  final bool added;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 32,
      height: 32,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        borderRadius: AppRadii.brSm,
        color: added ? AppColors.cyan14 : AppColors.surfaceSunken,
        border: Border.all(
          color: added ? AppColors.cyan28 : AppColors.borderStrong,
        ),
      ),
      child: added
          ? const AppIcon(
              AppIconData.check,
              size: 16,
              color: AppColors.cyan500,
            )
          : const Icon(
              Icons.add_rounded,
              size: 18,
              color: AppColors.textSecondary,
            ),
    );
  }
}
