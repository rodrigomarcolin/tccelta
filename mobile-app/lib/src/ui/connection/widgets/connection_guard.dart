import 'package:flutter/widgets.dart';
import 'package:hooks_riverpod/hooks_riverpod.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_adapter_state.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/router/app_router.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/connection/view_model/connecting_view_model.dart';
import 'package:tccelta_mobile/src/ui/core/hooks/hooks.dart';

/// Guarda de sessão BLE de escopo global.
///
/// Montado uma única vez no topo (via o `builder` do `MaterialApp.router`),
/// observa a fase da conexão E o adaptador Bluetooth em QUALQUER tela —
/// inclusive no painel, onde a `ConnectedScreen` (e seu antigo listener local)
/// não está mais montada.
///
/// Faz dois redirecionamentos globais, por ordem de prioridade:
///
/// 1. **Bluetooth desligado** — quando o adaptador vira `off`, vai para
///    [AppRoutes.bluetoothOff], EXCETO nas telas pré-conexão que já tratam o
///    adaptador por conta própria ([_adapterOffExcludedRoutes]): `bluetoothOff`
///    (já estamos lá), `permissions` (o gating de permissão vem primeiro) e
///    `scan` (renderiza um aviso inline "ligue o Bluetooth"). É a causa de
///    maior prioridade: desligar o BT também derruba a sessão, então precisa
///    VENCER o redirecionamento de "Conexão perdida" abaixo — do contrário o
///    usuário veria "Conexão perdida" quando o certo é "Bluetooth desligado".
///
/// 2. **Conexão perdida** — quando a fase vira terminal (`failed`/
///    `disconnected`) E o adaptador segue `on`, vai para
///    [AppRoutes.connectionLost], EXCETO nas telas do próprio fluxo de conexão
///    ([_flowRoutes]). A decisão é adiada por [_connectionLostGrace]: desligar
///    o BT dispara a queda de fase e o evento `off` do adaptador em streams
///    SEPARADOS, que correm entre si — sem a janela, um `failed` observado
///    antes do `off` mandaria para "Conexão perdida" e só depois para
///    "Bluetooth desligado" (o flash relatado). A janela deixa o estado do
///    adaptador assentar; se o BT tiver caído, a navegação é abortada. O
///    atraso é imperceptível numa queda real (que já passou ~12 s em
///    `reconnecting`).
///    - `connecting`: o desfecho do handshake é da `ConnectingScreen` (que já
///      navega para Conexão perdida em falha) — a queda de sessão real passa
///      por `ready -> reconnecting -> connecting -> failed`, então NÃO dá para
///      distinguir handshake de reconexão só pela fase; a rota atual resolve;
///    - `connectionLost`: já estamos lá (o "Esquecer dispositivo" segue para
///      permissões sem voltar para cá);
///    - `permissions`/`bluetoothOff`/`scan`: pré-conexão, sem sessão a perder.
///
/// Como ambas são listas de EXCLUSÃO, qualquer tela pós-conexão futura (além de
/// `connected`/`painel`) já fica coberta automaticamente.
///
/// 3. **Permissão revogada** — o SO permite revogar a permissão de Bluetooth
///    pelos Ajustes enquanto o app está conectado, e o `permission_handler` não
///    emite stream dessa mudança (nem o `adapterState`, que colapsa
///    `unauthorized` em `unknown`). Como o usuário sempre passa pelos Ajustes
///    (o app vai a segundo plano e volta), re-checamos a permissão a cada
///    `resume`: se caiu, derrubamos a sessão e voltamos ao gate de permissões.
///    Só age fora do fluxo de conexão (mesma exclusão [_flowRoutes]).
class ConnectionGuard extends HookConsumerWidget {
  /// Cria o guard envolvendo [child] (a subárvore de rotas).
  const ConnectionGuard({required this.child, super.key});

  /// Subárvore renderizada abaixo do guard (o Navigator do go_router).
  final Widget child;

  /// Rotas do fluxo de conexão onde o redirecionamento de "Conexão perdida"
  /// NÃO se aplica.
  static const Set<String> _flowRoutes = {
    AppRoutes.permissions,
    AppRoutes.bluetoothOff,
    AppRoutes.scan,
    AppRoutes.connecting,
    AppRoutes.connectionLost,
  };

  /// Rotas onde o redirecionamento de "Bluetooth desligado" NÃO se aplica —
  /// telas pré-conexão que já lidam com o adaptador por conta própria.
  static const Set<String> _adapterOffExcludedRoutes = {
    AppRoutes.permissions,
    AppRoutes.bluetoothOff,
    AppRoutes.scan,
  };

  /// Janela para o estado do adaptador assentar antes de decidir por "Conexão
  /// perdida" — absorve a corrida entre a queda de fase e o evento `off` do
  /// adaptador (ver item 2 na doc da classe).
  static const Duration _connectionLostGrace = Duration(milliseconds: 700);

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    // (3) Permissão revogada em runtime: re-checa a cada retorno ao primeiro
    // plano (o usuário passou pelos Ajustes do SO). Se a permissão caiu,
    // derruba a sessão morta e volta ao gate de permissões — que re-checa e
    // re-avança sozinho.
    useOnAppResume(() async {
      // Só as telas pós-conexão têm sessão a proteger.
      if (_flowRoutes.contains(appRouter.state.matchedLocation)) return;
      final permissions = ref.read(permissionsRepositoryProvider);
      if (await permissions.hasBluetoothPermission()) return;
      await ref.read(dongleRepositoryProvider).disconnect();
      // Reconfirma a rota após o await (o usuário pode ter navegado no meio).
      if (_flowRoutes.contains(appRouter.state.matchedLocation)) return;
      appRouter.go(AppRoutes.permissions);
    });

    // (1) Bluetooth desligado: leva direto para a tela de BT desligado, de
    // qualquer tela pós-conexão. Tem prioridade sobre "Conexão perdida".
    ref
      ..listen(adapterStateProvider, (_, next) {
        if (next.asData?.value != BleAdapterState.off) return;
        if (!_adapterOffExcludedRoutes
            .contains(appRouter.state.matchedLocation)) {
          appRouter.go(AppRoutes.bluetoothOff);
        }
      })
      // (2) Conexão perdida: só quando o link cai por outra causa que não o BT
      // desligado (esse caso já foi tratado acima).
      ..listen<BleConnectionPhase>(
          connectingViewModelProvider.select((s) => s.phase), (_, next) {
        final dropped = next == BleConnectionPhase.failed ||
            next == BleConnectionPhase.disconnected;
        if (!dropped) return;
        // Adia a decisão para o estado do adaptador assentar (o `off` corre em
        // stream separado). Se o BT tiver caído, o listener (1) leva para
        // "Bluetooth desligado" e esta navegação é abortada aqui.
        Future<void>.delayed(_connectionLostGrace, () {
          // Só segue se o adaptador seguir CONFIRMADAMENTE ligado (o estado
          // `unknown` durante o desligamento também aborta).
          if (ref.read(adapterStateProvider).asData?.value !=
              BleAdapterState.on) {
            return;
          }
          // Navegamos pela instância global do router: o `builder` fica ACIMA
          // do Navigator, então não há um `context` de rota utilizável aqui.
          if (!_flowRoutes.contains(appRouter.state.matchedLocation)) {
            appRouter.go(AppRoutes.connectionLost);
          }
        });
      });

    return child;
  }
}
