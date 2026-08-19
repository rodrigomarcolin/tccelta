import 'dart:ui' show ImageFilter;

import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/icons/app_icon.dart';

/// Uma aba da [AppTabBar].
class AppTab {
  /// Cria uma aba com [key], [label] e [icon].
  const AppTab({
    required this.key,
    required this.label,
    required this.icon,
    this.comingSoon = false,
  });

  /// Identificador único da aba.
  final String key;

  /// Rótulo exibido.
  final String label;

  /// Ícone outline da aba.
  final AppIconData icon;

  /// Se `true`, a aba ainda não tem tela — o ícone fica sempre na cor
  /// inativa (nunca ciano, mesmo se `active` apontar pra ela) e ganha a
  /// legenda "EM BREVE" embaixo do rótulo. Não desativa o toque em si — a
  /// aba simplesmente não reage porque nenhuma tela trata essa `key` ainda.
  /// @default false
  final bool comingSoon;
}

/// Navegação inferior do shell do app.
///
/// Quatro abas por padrão (Painel, Sensores, Terminal, Mais). A aba ativa é
/// ciano, as inativas neutral-400; a barra é um escuro translúcido borrado com
/// um hairline no topo. Fixe-a na base da tela. Espelha o componente `TabBar`.
///
/// "Terminal" é `comingSoon` (ver [AppTab.comingSoon]): ainda sem tela, fica
/// sempre na cor inativa e ganha a legenda "EM BREVE".
class AppTabBar extends StatelessWidget {
  /// Cria a tab bar inferior, com [active] indicando a aba selecionada.
  const AppTabBar({
    this.active = 'painel',
    this.tabs = defaultTabs,
    this.onChanged,
    super.key,
  });

  /// Abas padrão do app.
  static const List<AppTab> defaultTabs = [
    AppTab(key: 'painel', label: 'Painel', icon: AppIconData.painel),
    AppTab(key: 'sensores', label: 'Sensores', icon: AppIconData.sensores),
    AppTab(
      key: 'terminal',
      label: 'Terminal',
      icon: AppIconData.terminal,
      comingSoon: true,
    ),
    AppTab(key: 'mais', label: 'Mais', icon: AppIconData.mais),
  ];

  /// Chave da aba ativa. @default "painel"
  final String active;

  /// Sobrescreve as quatro abas padrão.
  final List<AppTab> tabs;

  /// Chamado quando o usuário toca numa aba.
  final ValueChanged<String>? onChanged;

  @override
  Widget build(BuildContext context) {
    final bottomInset = MediaQuery.paddingOf(context).bottom;
    return ClipRect(
      child: BackdropFilter(
        filter: ImageFilter.blur(sigmaX: 12, sigmaY: 12),
        child: Container(
          decoration: const BoxDecoration(
            color: AppColors.tabBarSurface,
            border: Border(
              top: BorderSide(color: AppColors.borderHairline),
            ),
          ),
          padding: EdgeInsets.only(
            top: AppSpacing.s3,
            bottom: AppSpacing.s3 + bottomInset,
          ),
          child: Row(
            children: [
              for (final tab in tabs)
                Expanded(
                  child: _TabItem(
                    tab: tab,
                    selected: tab.key == active,
                    onTap: onChanged == null ? null : () => onChanged!(tab.key),
                  ),
                ),
            ],
          ),
        ),
      ),
    );
  }
}

class _TabItem extends StatelessWidget {
  const _TabItem({required this.tab, required this.selected, this.onTap});

  final AppTab tab;
  final bool selected;
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    final color = selected && !tab.comingSoon
        ? AppColors.cyan500
        : AppColors.neutral400;
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          AppIcon(tab.icon, color: color),
          const SizedBox(height: 4),
          Text(
            tab.label,
            style: AppTypography.ui(
              TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w600,
                color: color,
              ),
            ),
          ),
          if (tab.comingSoon) ...[
            const SizedBox(height: 2),
            Text(
              'EM BREVE',
              style: AppTypography.ui(
                const TextStyle(
                  fontSize: 8,
                  fontWeight: FontWeight.w700,
                  letterSpacing: 0.6,
                  color: AppColors.amber500,
                ),
              ),
            ),
          ],
        ],
      ),
    );
  }
}
