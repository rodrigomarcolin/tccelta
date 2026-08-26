import 'package:flutter/material.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/telemetry/panel.dart';

/// Linha horizontal rolável com uma aba por [Panel] do usuário — nome +
/// contagem de indicadores, destacada em ciano quando ativa — mais um botão
/// "+" ao final para criar um painel novo na hora.
///
/// Fica logo abaixo do cabeçalho do Painel, entre o nome do painel ativo e o
/// grid de indicadores.
class PanelTabsRow extends StatelessWidget {
  /// Cria a linha de abas dos painéis.
  const PanelTabsRow({
    required this.panels,
    required this.activeId,
    required this.onSelect,
    required this.onCreate,
    super.key,
  });

  /// Painéis do usuário, na ordem de exibição.
  final List<Panel> panels;

  /// `id` do painel ativo.
  final String activeId;

  /// Chamado com o `id` do painel tocado.
  final ValueChanged<String> onSelect;

  /// Chamado ao tocar o botão "+" de criar painel.
  final VoidCallback onCreate;

  /// Altura da linha — folga suficiente para o texto (13px) + o padding
  /// vertical da pill não cortar a base das letras com descendentes (ex.:
  /// "ç", "g"). Também o lado do botão "+" quadrado, para ficarem alinhados.
  static const double _height = 40;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: _height,
      child: ListView.separated(
        scrollDirection: Axis.horizontal,
        itemCount: panels.length + 1,
        separatorBuilder: (context, index) =>
            const SizedBox(width: AppSpacing.s2),
        itemBuilder: (context, index) {
          if (index == panels.length) {
            return _CreatePanelTab(
              key: const ValueKey('panel_tabs_create'),
              size: _height,
              onTap: onCreate,
            );
          }
          final panel = panels[index];
          return _PanelTab(
            key: ValueKey('panel_tab_${panel.id}'),
            panel: panel,
            selected: panel.id == activeId,
            height: _height,
            onTap: () => onSelect(panel.id),
          );
        },
      ),
    );
  }
}

/// Uma aba de painel: nome + contagem de indicadores.
class _PanelTab extends StatelessWidget {
  const _PanelTab({
    required this.panel,
    required this.selected,
    required this.height,
    required this.onTap,
    super.key,
  });

  final Panel panel;
  final bool selected;
  final double height;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final fg = selected ? AppColors.cyan500 : AppColors.textTertiary;
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        height: height,
        alignment: Alignment.center,
        padding: const EdgeInsets.symmetric(horizontal: AppSpacing.s5),
        decoration: BoxDecoration(
          color: selected ? AppColors.cyan14 : AppColors.surfaceList,
          borderRadius: AppRadii.brMd,
          border: Border.all(
            color: selected ? AppColors.cyan28 : AppColors.borderStrong,
          ),
        ),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(
              panel.name,
              style: AppTypography.ui(
                TextStyle(fontSize: 13, fontWeight: FontWeight.w600, color: fg),
              ),
            ),
            const SizedBox(width: AppSpacing.s2),
            Text(
              '${panel.indicatorIds.length}',
              style: AppTypography.mono(
                TextStyle(
                  fontSize: 11,
                  fontWeight: FontWeight.w600,
                  color: fg.withValues(alpha: 0.6),
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

/// Botão "+" ao final da linha de abas — cria um painel novo.
class _CreatePanelTab extends StatelessWidget {
  const _CreatePanelTab({required this.size, required this.onTap, super.key});

  final double size;
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        width: size,
        height: size,
        alignment: Alignment.center,
        decoration: BoxDecoration(
          border: Border.all(color: AppColors.borderStrong),
          borderRadius: AppRadii.brMd,
        ),
        child: const Icon(
          Icons.add_rounded,
          size: 18,
          color: AppColors.textTertiary,
        ),
      ),
    );
  }
}
