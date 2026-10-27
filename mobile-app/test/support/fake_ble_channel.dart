import 'dart:async';

import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';

/// [BleChannel] falso: registra o que foi escrito e deixa o teste emitir
/// notificações.
class FakeBleChannel implements BleChannel {
  /// Cria o canal; [ready] controla [isReady].
  FakeBleChannel({this.ready = true});

  /// Valor de [isReady].
  bool ready;

  /// Escritas feitas pelo app, na ordem.
  final List<List<int>> written = [];

  /// Se não nulo, [write] falha com este erro.
  Exception? writeError;

  final StreamController<List<int>> _incoming =
      StreamController<List<int>>.broadcast();

  @override
  Stream<List<int>> get incoming => _incoming.stream;

  @override
  bool get isReady => ready;

  @override
  Future<void> write(List<int> bytes) async {
    final error = writeError;
    if (error != null) throw error;
    written.add(List<int>.of(bytes));
  }

  /// Emite [bytes] como se viessem do dispositivo.
  void emit(List<int> bytes) {
    if (!_incoming.isClosed) _incoming.add(bytes);
  }
}
