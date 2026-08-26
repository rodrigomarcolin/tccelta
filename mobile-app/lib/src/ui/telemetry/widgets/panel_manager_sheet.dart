import 'package:flutter/material.dart';
import 'package:flutter_hooks/flutter_hooks.dart';
import 'package:hooks_riverpod/hooks_riverpod.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/telemetry/panel.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/panel_view_model.dart';
import 'package:tccelta_mobile/src/ui/telemetry/widgets/new_panel_row.dart';

/// Abre o sheet "Meus painéis", empilhado sobre a tela atual: uma lista de
/// painéis (tocar troca de painel, "⋯" abre a edição) e, por painel, uma
/// sub-tela de edição (renomear, duplicar, excluir).
Future<void> showPanelManagerSheet(BuildContext context) {
  return showModalBottomSheet<void>(
    context: context,
    isScrollControlled: true,
    backgroundColor: Colors.transparent,
    builder: (context) => const PanelManagerSheet(),
  );
}

/// Conteúdo do sheet "Meus painéis".
///
/// Diferente do sheet de formato do indicador (altura fixa, wizard linear),
/// este sheet dimensiona pela quantidade de painéis — poucos painéis não
/// deixam vão vazio embaixo — com um teto de 92% da altura da tela.
class PanelManagerSheet extends HookConsumerWidget {
  /// Cria o sheet de gerenciamento de painéis.
  const PanelManagerSheet({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final editingId = useState<String?>(null);
    final panels = ref.watch(panelViewModelProvider);
    final notifier = ref.read(panelViewModelProvider.notifier);

    // Empurra o sheet para cima do teclado — sem isso, o campo de renomear
    // (no passo de edição) fica escondido atrás dele ao ganhar foco.
    return Padding(
      padding: EdgeInsets.only(
        bottom: MediaQuery.viewInsetsOf(context).bottom,
      ),
      child: DecoratedBox(
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
                  Flexible(
                    child: SingleChildScrollView(
                      child: editingId.value == null
                          ? _PanelList(
                              panels: panels,
                              onPick: notifier.switchPanel,
                              onEdit: (id) => editingId.value = id,
                              onCreate: notifier.createPanel,
                              onClose: () => Navigator.of(context).pop(),
                            )
                          : _PanelEdit(
                              panel: panels.panels.firstWhere(
                                (p) => p.id == editingId.value,
                              ),
                              canDelete: panels.panels.length > 1,
                              onBack: () => editingId.value = null,
                              onRename: (name) => notifier.renamePanel(
                                editingId.value!,
                                name,
                              ),
                              onDuplicate: () {
                                notifier.duplicatePanel(editingId.value!);
                                editingId.value = null;
                              },
                              onDelete: () async {
                                final confirmed = await showConfirmDialog(
                                  context,
                                  title: 'Excluir painel?',
                                  message:
                                      'Os indicadores deste painel serão '
                                      'perdidos. Essa ação não pode ser '
                                      'desfeita.',
                                  confirmLabel: 'Excluir',
                                );
                                if (confirmed ?? false) {
                                  notifier.deletePanel(editingId.value!);
                                  editingId.value = null;
                                }
                              },
                            ),
                    ),
                  ),
                ],
              ),
            ),
          ),
        ),
      ),
    );
  }
}

/// Alça de arrastar visual do sheet — mesmo padrão do
/// `IndicatorFormatSheet`.
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

/// Rótulo "N indicador(es)" — singular/plural do PT-BR.
///
/// Compartilhado com `PanelPickerSheet`, que também lista painéis com sua
/// contagem de indicadores.
String indicatorCountLabel(int count) =>
    count == 1 ? '1 indicador' : '$count indicadores';

/// Lista de painéis: tocar troca de painel, "⋯" abre a edição do painel
/// tocado (nunca troca — evento próprio, sem propagar para a linha).
class _PanelList extends StatelessWidget {
  const _PanelList({
    required this.panels,
    required this.onPick,
    required this.onEdit,
    required this.onCreate,
    required this.onClose,
  });

  final PanelsState panels;
  final ValueChanged<String> onPick;
  final ValueChanged<String> onEdit;
  final VoidCallback onCreate;
  final VoidCallback onClose;

  @override
  Widget build(BuildContext context) {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Text('Meus painéis', style: AppTypography.title),
        const SizedBox(height: AppSpacing.s2),
        Text(
          'toque para alternar · ⋯ para editar',
          style: AppTypography.body.copyWith(color: AppColors.textTertiary),
        ),
        const SizedBox(height: AppSpacing.s4),
        for (final panel in panels.panels) ...[
          CardButton(
            key: ValueKey('panel_row_${panel.id}'),
            title: panel.name,
            subtitle: indicatorCountLabel(panel.indicatorIds.length),
            showValue: false,
            showChevron: false,
            selected: panel.id == panels.activeId,
            trailing: _PanelRowTrailing(
              key: ValueKey('panel_edit_${panel.id}'),
              active: panel.id == panels.activeId,
              onEdit: () => onEdit(panel.id),
            ),
            onTap: () => onPick(panel.id),
          ),
          const SizedBox(height: AppSpacing.s2),
        ],
        NewPanelRow(onTap: onCreate),
        const SizedBox(height: AppSpacing.s4),
        AppButton(onPressed: onClose, child: const Text('Concluir')),
      ],
    );
  }
}

/// Trailing de uma linha de painel: "EM USO" (só no ativo) + botão "⋯" de
/// editar, com toque próprio — separado do toque na linha, que troca de
/// painel.
class _PanelRowTrailing extends StatelessWidget {
  const _PanelRowTrailing({
    required this.active,
    required this.onEdit,
    super.key,
  });

  final bool active;
  final VoidCallback onEdit;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        if (active) ...[
          Text(
            'EM USO',
            style: AppTypography.overline.copyWith(color: AppColors.cyan500),
          ),
          const SizedBox(width: AppSpacing.s3),
        ],
        GestureDetector(
          behavior: HitTestBehavior.opaque,
          onTap: onEdit,
          child: Container(
            width: 32,
            height: 32,
            alignment: Alignment.center,
            decoration: BoxDecoration(
              borderRadius: AppRadii.brSm,
              color: AppColors.surfaceList,
              border: Border.all(color: AppColors.borderStrong),
            ),
            child: const Icon(
              Icons.more_horiz,
              size: 18,
              color: AppColors.textTertiary,
            ),
          ),
        ),
      ],
    );
  }
}

/// Edição de um painel: renomear (texto ao vivo), duplicar e — se não for o
/// único painel restante — excluir (com confirmação).
class _PanelEdit extends HookWidget {
  const _PanelEdit({
    required this.panel,
    required this.canDelete,
    required this.onBack,
    required this.onRename,
    required this.onDuplicate,
    required this.onDelete,
  });

  final Panel panel;
  final bool canDelete;
  final VoidCallback onBack;
  final ValueChanged<String> onRename;
  final VoidCallback onDuplicate;
  final VoidCallback onDelete;

  @override
  Widget build(BuildContext context) {
    final controller = useTextEditingController(text: panel.name);

    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      children: [
        Row(
          children: [
            IconButton(
              icon: const Icon(Icons.arrow_back_ios_new, size: 16),
              color: AppColors.textSecondary,
              padding: EdgeInsets.zero,
              constraints: const BoxConstraints(),
              onPressed: onBack,
            ),
            const SizedBox(width: AppSpacing.s2),
            Text('Editar painel', style: AppTypography.title),
          ],
        ),
        const SizedBox(height: AppSpacing.s2),
        Text(
          indicatorCountLabel(panel.indicatorIds.length),
          style: AppTypography.body.copyWith(color: AppColors.textTertiary),
        ),
        const SizedBox(height: AppSpacing.s7),
        Text('NOME', style: AppTypography.overline),
        const SizedBox(height: AppSpacing.s2),
        TextField(
          controller: controller,
          onChanged: onRename,
          maxLength: PanelNameLimits.max,
          style: AppTypography.ui(
            const TextStyle(
              fontSize: 15,
              fontWeight: FontWeight.w600,
              color: AppColors.textPrimary,
            ),
          ),
          decoration: const InputDecoration(
            filled: true,
            fillColor: AppColors.surfaceSunken,
            counterText: '',
            contentPadding: EdgeInsets.symmetric(
              horizontal: AppSpacing.s5,
              vertical: AppSpacing.s4,
            ),
            border: OutlineInputBorder(
              borderRadius: AppRadii.brMd,
              borderSide: BorderSide(color: AppColors.borderHairline),
            ),
            enabledBorder: OutlineInputBorder(
              borderRadius: AppRadii.brMd,
              borderSide: BorderSide(color: AppColors.borderHairline),
            ),
            focusedBorder: OutlineInputBorder(
              borderRadius: AppRadii.brMd,
              borderSide: BorderSide(color: AppColors.accent, width: 1.5),
            ),
          ),
        ),
        const SizedBox(height: AppSpacing.s4),
        CardButton(
          title: 'Duplicar painel',
          subtitle: 'cria uma cópia com os mesmos indicadores',
          showValue: false,
          showChevron: false,
          leading: const Icon(
            Icons.content_copy_rounded,
            size: 18,
            color: AppColors.neutral600,
          ),
          onTap: onDuplicate,
        ),
        if (canDelete) ...[
          const SizedBox(height: AppSpacing.s2),
          _DeletePanelRow(onTap: onDelete),
        ],
        const SizedBox(height: AppSpacing.s4),
        AppButton(onPressed: onBack, child: const Text('Concluir')),
      ],
    );
  }
}

/// Linha destrutiva "Excluir painel" — estilo vermelho translúcido, mesmo
/// tom de significado usado nos botões `danger`.
class _DeletePanelRow extends StatelessWidget {
  const _DeletePanelRow({required this.onTap});

  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        constraints: const BoxConstraints(minHeight: 56),
        padding: const EdgeInsets.symmetric(
          horizontal: AppSpacing.s5,
          vertical: AppSpacing.s4,
        ),
        decoration: BoxDecoration(
          color: AppColors.red08,
          border: Border.all(color: AppColors.red20),
          borderRadius: AppRadii.brMd,
        ),
        child: Row(
          children: [
            const Icon(
              Icons.delete_outline_rounded,
              size: 18,
              color: AppColors.danger,
            ),
            const SizedBox(width: AppSpacing.s3),
            Text(
              'Excluir painel',
              style: AppTypography.label.copyWith(color: AppColors.danger),
            ),
          ],
        ),
      ),
    );
  }
}
