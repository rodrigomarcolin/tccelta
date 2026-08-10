import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/services/settings/settings_service.dart';

/// Shared, lazily-instantiated [SettingsService].
final Provider<SettingsService> settingsServiceProvider =
    Provider<SettingsService>((_) => SettingsService());

/// The stored PSK hex string — `null` means no key (plaintext mode).
///
/// Refreshes whenever [pskNotifierProvider] writes a new value.
final FutureProvider<String?> pskProvider = FutureProvider<String?>(
  (ref) => ref.watch(settingsServiceProvider).getPsk(),
);

/// Mutable notifier for the PSK — use [save] or [clear] from the UI.
final AsyncNotifierProvider<PskNotifier, String?> pskNotifierProvider =
    AsyncNotifierProvider<PskNotifier, String?>(PskNotifier.new);

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
