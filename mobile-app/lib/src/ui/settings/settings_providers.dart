import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/domain/ble/security_mode.dart';
import 'package:tccelta_mobile/src/services/settings/settings_service.dart';

/// Shared, lazily-instantiated [SettingsService].
final Provider<SettingsService> settingsServiceProvider =
    Provider<SettingsService>((_) => SettingsService());

/// The stored PSK hex string — `null` means secure connection unavailable.
///
/// Refreshes whenever [pskNotifierProvider] writes a new value.
final FutureProvider<String?> pskProvider = FutureProvider<String?>(
  (ref) => ref.watch(settingsServiceProvider).getPsk(),
);

/// Mutable notifier for the PSK — use [PskNotifier.save] or
/// [PskNotifier.clear] from the UI.
final AsyncNotifierProvider<PskNotifier, String?> pskNotifierProvider =
    AsyncNotifierProvider<PskNotifier, String?>(PskNotifier.new);

/// Persisted secure transport mode used by the next dongle connection.
final AsyncNotifierProvider<SecurityModeNotifier, SecurityMode>
securityModeNotifierProvider =
    AsyncNotifierProvider<SecurityModeNotifier, SecurityMode>(
      SecurityModeNotifier.new,
    );

/// Manages reading / writing the PSK.
class PskNotifier extends AsyncNotifier<String?> {
  @override
  Future<String?> build() => ref.watch(settingsServiceProvider).getPsk();

  /// Persists [hex] (must be 64 valid hex chars) and refreshes state.
  Future<void> save(String hex) async {
    await ref.read(settingsServiceProvider).setPsk(hex);
    state = AsyncData(hex.isEmpty ? null : hex);
  }

  /// Wipes the stored key (plaintext mode).
  Future<void> clear() async {
    await ref.read(settingsServiceProvider).clearPsk();
    state = const AsyncData(null);
  }
}

/// Manages the selected static/session-key security variant.
class SecurityModeNotifier extends AsyncNotifier<SecurityMode> {
  @override
  Future<SecurityMode> build() =>
      ref.watch(settingsServiceProvider).getSecurityMode();

  /// Persists [mode] and refreshes state.
  Future<void> save(SecurityMode mode) async {
    await ref.read(settingsServiceProvider).setSecurityMode(mode);
    state = AsyncData(mode);
  }
}
