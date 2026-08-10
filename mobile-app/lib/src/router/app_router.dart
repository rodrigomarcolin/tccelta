import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/view/bluetooth_off_screen.dart';
import 'package:tccelta_mobile/src/ui/connection/view/connected_screen.dart';
import 'package:tccelta_mobile/src/ui/connection/view/connecting_screen.dart';
import 'package:tccelta_mobile/src/ui/connection/view/connection_lost_screen.dart';
import 'package:tccelta_mobile/src/ui/connection/view/permissions_screen.dart';
import 'package:tccelta_mobile/src/ui/connection/view/scan_screen.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view/painel_screen.dart';
import 'package:tccelta_mobile/src/ui/settings/view/psk_settings_screen.dart';

/// Roteador do app. O fluxo de conexão (`01 FLUXO DE CONEXÃO`) é a entrada;
/// `/painel` abre o painel de telemetria OBD-II (`PainelScreen`).
final GoRouter appRouter = GoRouter(
  initialLocation: AppRoutes.permissions,
  routes: [
    GoRoute(
      path: AppRoutes.permissions,
      builder: (context, state) => const PermissionsScreen(),
    ),
    GoRoute(
      path: AppRoutes.bluetoothOff,
      builder: (context, state) => const BluetoothOffScreen(),
    ),
    GoRoute(
      path: AppRoutes.scan,
      builder: (context, state) => const ScanScreen(),
    ),
    GoRoute(
      path: AppRoutes.connecting,
      builder: (context, state) => const ConnectingScreen(),
    ),
    GoRoute(
      path: AppRoutes.connected,
      builder: (context, state) => const ConnectedScreen(),
    ),
    GoRoute(
      path: AppRoutes.connectionLost,
      builder: (context, state) => const ConnectionLostScreen(),
    ),
    GoRoute(
      path: AppRoutes.painel,
      builder: (context, state) => const PainelScreen(),
    ),
    GoRoute(
      path: AppRoutes.settings,
      builder: (context, state) => const PskSettingsScreen(),
    ),
  ],
);
