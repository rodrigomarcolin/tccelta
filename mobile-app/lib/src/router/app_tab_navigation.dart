import 'package:flutter/widgets.dart';
import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';

/// Navega para a rota correspondente à aba [key] da `AppTabBar`
/// (`ui/core/widgets/app_tab_bar.dart`).
///
/// Fonte única do mapeamento chave→rota: toda tela que monta a tab bar deve
/// chamar esta função no `onChanged`, em vez de reimplementar seu próprio
/// `if` por chave — foi exatamente por duplicar essa cadeia em cada tela que
/// uma delas ficou faltando o case `'sensores'` (a aba não respondia a toque
/// a partir de "Mais"). Navegar para a própria aba atual é um no-op seguro
/// no go_router, então nenhuma tela precisa excluir sua própria `key`.
void goToAppTab(BuildContext context, String key) {
  switch (key) {
    case 'painel':
      context.go(AppRoutes.painel);
    case 'sensores':
      context.go(AppRoutes.sensorPicker, extra: true);
    case 'dtc':
      context.go(AppRoutes.dtc);
    case 'mais':
      context.go(AppRoutes.more);
  }
}
