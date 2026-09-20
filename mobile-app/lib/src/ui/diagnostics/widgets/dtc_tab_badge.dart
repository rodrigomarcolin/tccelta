import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// Cópia de [AppTabBar.defaultTabs] com o `badgeCount` da aba "DTCs"
/// preenchido com [activeCount] (`0` = sem badge).
///
/// Compartilhado por toda tela que monta a [AppTabBar] (`PainelScreen`,
/// `SensorPickerScreen`, `MoreScreen`, `DtcScreen`) — cada uma lê
/// `dtcViewModelProvider` e chama isto para o parâmetro `tabs:`, já que a
/// tab bar em si não conhece o view model.
List<AppTab> tabsWithDtcBadge(int activeCount) => [
  for (final tab in AppTabBar.defaultTabs)
    tab.key == 'dtc' ? tab.withBadgeCount(activeCount) : tab,
];
