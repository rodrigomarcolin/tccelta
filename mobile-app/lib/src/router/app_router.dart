import 'package:go_router/go_router.dart';
import 'package:tccelta_mobile/src/router/app_routes.dart';
import 'package:tccelta_mobile/src/ui/connection/view/ble_permissions_screen.dart';
import 'package:tccelta_mobile/src/ui/connection/view/bluetooth_off_screen.dart';
import 'package:tccelta_mobile/src/ui/connection/view/connected_screen.dart';
import 'package:tccelta_mobile/src/ui/connection/view/connecting_screen.dart';
import 'package:tccelta_mobile/src/ui/connection/view/connection_lost_screen.dart';
import 'package:tccelta_mobile/src/ui/connection/view/scan_screen.dart';
import 'package:tccelta_mobile/src/ui/diagnostics/view/dtc_screen.dart';
import 'package:tccelta_mobile/src/ui/more/view/more_screen.dart';
import 'package:tccelta_mobile/src/ui/settings/view/camera_permissions_screen.dart';
import 'package:tccelta_mobile/src/ui/settings/view/psk_settings_screen.dart';
import 'package:tccelta_mobile/src/ui/settings/view/qr_scan_screen.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view/painel_screen.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view/sensor_picker_screen.dart';

/// Roteador do app. O fluxo de conexão (`01 FLUXO DE CONEXÃO`) é a entrada;
/// `/painel` abre o painel de telemetria OBD-II (`PainelScreen`).
final GoRouter appRouter = GoRouter(
  initialLocation: AppRoutes.blePermissions,
  routes: [
    GoRoute(
      path: AppRoutes.blePermissions,
      builder: (context, state) => const BlePermissionsScreen(),
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
      path: AppRoutes.dtc,
      builder: (context, state) => const DtcScreen(),
    ),
    GoRoute(
      path: AppRoutes.settings,
      builder: (context, state) => const PskSettingsScreen(),
    ),
    GoRoute(
      path: AppRoutes.more,
      builder: (context, state) => const MoreScreen(),
    ),
    GoRoute(
      path: AppRoutes.pskSetup,
      builder: (context, state) => const PskSettingsScreen(setupFlow: true),
    ),
    GoRoute(
      path: AppRoutes.cameraPermissions,
      builder: (context, state) => const CameraPermissionsScreen(),
    ),
    GoRoute(
      path: AppRoutes.pskScanQr,
      builder: (context, state) => const QrScanScreen(),
    ),
    GoRoute(
      path: AppRoutes.sensorPicker,
      builder: (context, state) =>
          SensorPickerScreen(fromTab: state.extra == true),
    ),
  ],
);
