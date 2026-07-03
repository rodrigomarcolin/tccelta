import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/router/app_router.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/view_model/connecting_view_model.dart';

/// Guarda de sessão BLE de escopo global.
///
/// Montado uma única vez no topo (via o `builder` do `MaterialApp.router`),
/// observa a fase da conexão em QUALQUER tela — inclusive no painel, onde a
/// `ConnectedScreen` (e seu antigo listener local) não está mais montada.
///
/// Redireciona para [AppRoutes.connectionLost] quando a fase vira terminal
/// (`failed`/`disconnected`), EXCETO nas telas do próprio fluxo de conexão,
/// onde isso seria errado ou redundante ([_flowRoutes]):
/// - `connecting`: o desfecho do handshake é da `ConnectingScreen` (que já
///   navega para Conexão perdida em falha) — a queda de sessão real passa por
///   `ready -> reconnecting -> connecting -> failed`, então NÃO dá para
///   distinguir handshake de reconexão só pela fase; a rota atual resolve isso;
/// - `connectionLost`: já estamos lá (o "Esquecer dispositivo" segue para
///   permissões sem voltar para cá);
/// - `permissions`/`bluetoothOff`/`scan`: pré-conexão, sem sessão a perder.
///
/// Como é uma lista de EXCLUSÃO, qualquer tela pós-conexão futura (além de
/// `connected`/`painel`) já fica coberta automaticamente.
class ConnectionGuard extends ConsumerWidget {
  /// Cria o guard envolvendo [child] (a subárvore de rotas).
  const ConnectionGuard({required this.child, super.key});

  /// Subárvore renderizada abaixo do guard (o Navigator do go_router).
  final Widget child;

  /// Rotas do fluxo de conexão onde o redirecionamento global NÃO se aplica.
  static const Set<String> _flowRoutes = {
    AppRoutes.permissions,
    AppRoutes.bluetoothOff,
    AppRoutes.scan,
    AppRoutes.connecting,
    AppRoutes.connectionLost,
  };

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    ref.listen<BleConnectionPhase>(connectingViewModelProvider, (_, next) {
      final dropped = next == BleConnectionPhase.failed ||
          next == BleConnectionPhase.disconnected;
      if (!dropped) return;
      // Navegamos pela instância global do router: o `builder` fica ACIMA do
      // Navigator, então não há um `context` de rota utilizável aqui.
      if (!_flowRoutes.contains(appRouter.state.matchedLocation)) {
        appRouter.go(AppRoutes.connectionLost);
      }
    });

    return child;
  }
}
