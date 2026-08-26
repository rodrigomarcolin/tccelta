import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/telemetry/panel.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/panel_view_model.dart';
import 'package:tccelta_mobile/src/ui/telemetry/widgets/new_panel_row.dart';
import 'package:tccelta_mobile/src/ui/telemetry/widgets/panel_manager_sheet.dart';

/// Abre o sheet de escolha de painel, empilhado sobre a tela atual: uma lista
/// de painéis (tocar devolve o `id` escolhido) e uma linha "Novo painel" ao
/// final. Painéis que já contêm [pid] aparecem marcados — ver
/// [PanelPickerSheet].
///
/// `null` (o pop "cru" — inclusive o de tocar fora do sheet) significa que o
/// usuário cancelou.
Future<String?> showPanelPickerSheet(
  BuildContext context, {
  required Obd2Pid pid,
}) {
  return showModalBottomSheet<String>(
    context: context,
    isScrollControlled: true,
    backgroundColor: Colors.transparent,
    builder: (context) => PanelPickerSheet(pid: pid),
  );
}

/// Conteúdo do sheet de escolha de painel — passo intermediário da aba
/// "Sensores" antes do `IndicatorFormatSheet`: em qual painel o indicador
/// selecionado deve entrar (ou já está).
///
/// Painéis que já contêm [pid] mostram a tag "JÁ ADICIONADO" no lugar do
/// chevron — escolhê-los abre o sheet de formato em modo de edição (mesmo
/// fluxo de tocar num card já presente no painel), em vez de adicionar.
class PanelPickerSheet extends ConsumerWidget {
  /// Cria o sheet de escolha de painel para [pid].
  const PanelPickerSheet({required this.pid, super.key});

  /// PID sendo adicionado/editado.
  final Obd2Pid pid;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final panels = ref.watch(panelViewModelProvider);
    final notifier = ref.read(panelViewModelProvider.notifier);

    return DecoratedBox(
      decoration: const BoxDecoration(
        color: AppColors.surfaceCard,
        borderRadius: BorderRadius.vertical(top: Radius.circular(26)),
      ),
      child: SafeArea(
        top: false,
        child: ConstrainedBox(
          constraints: BoxConstraints(
            maxHeight: MediaQuery.sizeOf(context).height * 0.92,
          ),
          child: Padding(
            padding: const EdgeInsets.fromLTRB(
              AppSpacing.s7,
              AppSpacing.s4,
              AppSpacing.s7,
              AppSpacing.s7,
            ),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                const _DragHandle(),
                const SizedBox(height: AppSpacing.s4),
                Text('Adicionar a qual painel?', style: AppTypography.title),
                const SizedBox(height: AppSpacing.s2),
                Text(
                  pid.label,
                  style: AppTypography.body.copyWith(
                    color: AppColors.textTertiary,
                  ),
                ),
                const SizedBox(height: AppSpacing.s4),
                Flexible(
                  child: SingleChildScrollView(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.stretch,
                      children: [
                        for (final panel in panels.panels) ...[
                          _PanelPickerRow(
                            key: ValueKey('panel_pick_${panel.id}'),
                            panel: panel,
                            added: panel.contains(pid),
                            onTap: () => Navigator.of(context).pop(panel.id),
                          ),
                          const SizedBox(height: AppSpacing.s2),
                        ],
                        NewPanelRow(
                          onTap: () {
                            notifier.createPanel();
                            Navigator.of(
                              context,
                            ).pop(ref.read(panelViewModelProvider).activeId);
                          },
                        ),
                      ],
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

/// Alça de arrastar visual do sheet — mesmo padrão do `PanelManagerSheet`.
class _DragHandle extends StatelessWidget {
  const _DragHandle();

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Container(
        width: 40,
        height: 4,
        decoration: const BoxDecoration(
          color: AppColors.borderStrong,
          borderRadius: AppRadii.brPill,
        ),
      ),
    );
  }
}

/// Uma linha de painel na lista de escolha: nome + contagem de indicadores,
/// com a tag "JÁ ADICIONADO" quando [added].
class _PanelPickerRow extends StatelessWidget {
  const _PanelPickerRow({
    required this.panel,
    required this.added,
    required this.onTap,
    super.key,
  });

  final Panel panel;
  final bool added;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return CardButton(
      title: panel.name,
      subtitle: indicatorCountLabel(panel.indicatorIds.length),
      showValue: false,
      showChevron: !added,
      trailing: added ? const _AlreadyAddedTag() : null,
      onTap: onTap,
    );
  }
}

/// Tag "JÁ ADICIONADO" — mesmo padrão visual da tag "EM USO" do
/// `PanelManagerSheet`.
class _AlreadyAddedTag extends StatelessWidget {
  const _AlreadyAddedTag();

  @override
  Widget build(BuildContext context) {
    return Text(
      'JÁ ADICIONADO',
      style: AppTypography.overline.copyWith(color: AppColors.cyan500),
    );
  }
}
