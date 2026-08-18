import 'package:flutter/material.dart';
import 'package:flutter_hooks/flutter_hooks.dart';
import 'package:go_router/go_router.dart';
import 'package:hooks_riverpod/hooks_riverpod.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/telemetry/indicator_display.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/panel_view_model.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/telemetry_view_model.dart';
import 'package:tccelta_mobile/src/ui/telemetry/widgets/indicator_format_sheet.dart';

/// Tela empilhada (`context.push`) com a lista de sensores: busca por nome/PID
/// + lista cuja linha abre o [IndicatorFormatSheet] (direto, sem tela de
/// detalhe/gráfico no meio) — para adicionar (ainda não presente) ou editar
/// (já presente); o ícone quadrado à direita só remove (com confirmação),
/// quando já adicionado.
class SensorPickerScreen extends HookConsumerWidget {
  /// Cria a tela de sensores.
  const SensorPickerScreen({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final query = useState('');
    final telemetry = ref.watch(telemetryViewModelProvider);
    final panel = ref.watch(panelViewModelProvider);
    final byPid = {for (final r in telemetry.readings) r.pid: r};

    final results = _filter(Obd2Pid.values, query.value);

    return Scaffold(
      backgroundColor: AppColors.bgScreen,
      appBar: AppBar(
        backgroundColor: AppColors.bgScreen,
        elevation: 0,
        leading: IconButton(
          icon: const Icon(Icons.arrow_back_ios_new, size: 18),
          color: AppColors.textSecondary,
          onPressed: context.pop,
        ),
        title: Text('Sensores', style: AppTypography.heading),
        centerTitle: false,
      ),
      body: SafeArea(
        top: false,
        child: Padding(
          padding: const EdgeInsets.fromLTRB(
            AppSpacing.s7,
            AppSpacing.s2,
            AppSpacing.s7,
            AppSpacing.s7,
          ),
          child: Column(
            children: [
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
                            onOpen: () => _openFormat(
                              context,
                              ref,
                              pid,
                              added ? panel.displayFor(pid) : null,
                            ),
                            onRemove: () => _confirmRemove(context, ref, pid),
                          );
                        },
                      ),
              ),
              const SizedBox(height: AppSpacing.s4),
              AppButton(onPressed: context.pop, child: const Text('OK')),
            ],
          ),
        ),
      ),
    );
  }

  /// Abre o sheet de formato para [pid]: [initial] nulo é o fluxo de
  /// adicionar (a linha ainda não está no painel); não-nulo é o fluxo de
  /// editar (pré-preenchido com a customização atual). Direto — sem tela de
  /// detalhe/gráfico no meio.
  Future<void> _openFormat(
    BuildContext context,
    WidgetRef ref,
    Obd2Pid pid,
    IndicatorDisplay? initial,
  ) async {
    final result = await showIndicatorFormatSheet(
      context,
      pid: pid,
      initial: initial,
    );
    if (result == null || !context.mounted) return;
    final notifier = ref.read(panelViewModelProvider.notifier);
    if (result.remove) {
      notifier.removeIndicator(pid);
    } else if (result.display != null) {
      if (initial == null) {
        notifier.addIndicator(pid, result.display!);
      } else {
        notifier.updateDisplay(pid, result.display!);
      }
    }
  }

  /// Confirmação de remoção — só disparada pelo toque específico no ícone
  /// quadrado de alternância (nunca pelo toque na linha, que abre a edição).
  Future<void> _confirmRemove(
    BuildContext context,
    WidgetRef ref,
    Obd2Pid pid,
  ) async {
    final confirmed = await showConfirmDialog(
      context,
      title: 'Remover ${pid.label}?',
      message: 'Esse indicador vai sair do painel.',
      confirmLabel: 'Remover',
    );
    if (confirmed ?? false) {
      ref.read(panelViewModelProvider.notifier).removeIndicator(pid);
    }
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
/// quando já adicionado, `+` quando não.
///
/// A linha inteira ([onOpen]) sempre abre o sheet de formato — para
/// adicionar (ainda não presente) ou editar (já presente), pré-preenchida. Só
/// o toque específico no ícone quadrado ([onRemove]) — quando já adicionado —
/// pede confirmação para remover; sem adição prévia, o ícone tem o mesmo
/// efeito da linha (abrir o sheet de formato).
class _SensorTile extends StatelessWidget {
  const _SensorTile({
    required this.pid,
    required this.added,
    required this.valueStr,
    required this.onOpen,
    required this.onRemove,
  });

  final Obd2Pid pid;
  final bool added;
  final String valueStr;
  final VoidCallback onOpen;
  final VoidCallback onRemove;

  @override
  Widget build(BuildContext context) {
    return CardButton(
      key: ValueKey('sensor_row_${pid.name}'),
      title: pid.label,
      subtitle: pid.command,
      value: valueStr,
      unit: pid.unit,
      showChevron: false,
      trailing: _ToggleSquare(
        key: ValueKey('sensor_toggle_${pid.name}'),
        added: added,
        onTap: added ? onRemove : onOpen,
      ),
      onTap: onOpen,
    );
  }
}

/// Indicador quadrado com toque próprio — separado do toque na linha (ver
/// [_SensorTile]).
class _ToggleSquare extends StatelessWidget {
  const _ToggleSquare({required this.added, required this.onTap, super.key});

  final bool added;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      behavior: HitTestBehavior.opaque,
      onTap: onTap,
      child: Container(
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
      ),
    );
  }
}
