/// Security protocol selected for the next dongle connection.
enum SecurityMode {
  staticPsk('Static PSK', 'static'),
  handshake('Handshake', 'handshake'),
  handshakeReplay('Handshake + Counter', 'handshake_replay');

  const SecurityMode(this.label, this.storageValue);

  final String label;
  final String storageValue;

  static SecurityMode fromStorage(String? value) =>
      SecurityMode.values.firstWhere(
        (mode) => mode.storageValue == value,
        orElse: () => SecurityMode.staticPsk,
      );
}
