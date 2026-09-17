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
    this.badgeCount,
  });

  /// Identificador único da aba.
  final String key;

  /// Rótulo exibido.
  final String label;

  /// Ícone outline da aba.
  final AppIconData icon;

  /// Contagem exibida num círculo vermelho sobreposto ao ícone (ex.: nº de
  /// DTCs ativos). `null` ou `0` = sem badge.
  /// @default null
  final int? badgeCount;

  /// Cópia com [badgeCount] sobrescrito — os demais campos são fixos por
  /// aba, então não há necessidade de sobrescrevê-los aqui.
  AppTab withBadgeCount(int? badgeCount) =>
      AppTab(key: key, label: label, icon: icon, badgeCount: badgeCount);
}

/// Navegação inferior do shell do app.
///
/// Quatro abas por padrão (Painel, Sensores, DTCs, Mais). A aba ativa é
/// ciano, as inativas neutral-400; a barra é um escuro translúcido borrado
/// com um hairline no topo. Fixe-a na base da tela. Espelha o componente
/// `TabBar`.
///
/// "DTCs" pode exibir [AppTab.badgeCount] (nº de códigos ativos).
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
    AppTab(key: 'dtc', label: 'DTCs', icon: AppIconData.motor),
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
    final color = selected ? AppColors.cyan500 : AppColors.neutral400;
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          Stack(
            clipBehavior: Clip.none,
            alignment: Alignment.center,
            children: [
              AppIcon(tab.icon, color: color),
              if ((tab.badgeCount ?? 0) > 0)
                Positioned(
                  top: -4,
                  right: -9,
                  child: _TabBadge(count: tab.badgeCount!),
                ),
            ],
          ),
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
        ],
      ),
    );
  }
}

/// Círculo vermelho com a contagem de [AppTab.badgeCount], sobreposto ao
/// ícone da aba (ex.: nº de DTCs ativos).
class _TabBadge extends StatelessWidget {
  const _TabBadge({required this.count});

  final int count;

  @override
  Widget build(BuildContext context) {
    return Container(
      constraints: const BoxConstraints(minWidth: 16),
      height: 16,
      padding: const EdgeInsets.symmetric(horizontal: 4),
      alignment: Alignment.center,
      decoration: const BoxDecoration(
        color: AppColors.red500,
        borderRadius: AppRadii.brPill,
      ),
      child: Text(
        '$count',
        style: AppTypography.mono(
          const TextStyle(
            fontSize: 10,
            fontWeight: FontWeight.w700,
            color: AppColors.textPrimary,
            height: 1,
          ),
        ),
      ),
    );
  }
}
