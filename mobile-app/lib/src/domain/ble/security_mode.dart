/// Security protocol selected for the next dongle connection.
enum SecurityMode {
  /// Static pre-shared key, no handshake.
  staticPsk('Static PSK', 'static'),

  /// PSK-derived session key negotiated via handshake.
  handshake('Handshake', 'handshake'),

  /// Handshake with a replay-protection counter.
  handshakeReplay('Handshake + Counter', 'handshake_replay');

  const SecurityMode(this.label, this.storageValue);

  /// Human-readable label for display.
  final String label;

  /// Value persisted to storage for this mode.
  final String storageValue;

  /// Resolves a [SecurityMode] from its persisted [value], defaulting to
  /// [staticPsk] when unset or unrecognized.
  static SecurityMode fromStorage(String? value) =>
      SecurityMode.values.firstWhere(
        (mode) => mode.storageValue == value,
        orElse: () => SecurityMode.staticPsk,
      );
}
