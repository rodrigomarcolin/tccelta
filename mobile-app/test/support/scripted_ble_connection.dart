import 'dart:async';
import 'dart:convert' show ascii;

import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';

/// [BleConnection] falsa e roteirizável para exercitar a camada ELM327/OBD-II
/// sem hardware.
///
/// Modo automático (padrão): a cada [write], procura o comando em [responses]
/// (normalizado — maiúsculas, sem espaços nem `\r`) e emite a resposta
/// correspondente na TX no microtask seguinte, imitando o dongle.
///
/// Modo manual ([autoRespond] `false`): [write] só registra o comando; o teste
/// controla a TX chamando [emit] (útil para simular chunks fragmentados).
class ScriptedBleConnection implements BleConnection {
  /// Cria a conexão com o mapa [responses] comando→resposta (texto completo,
  /// incluindo o prompt `\r>`).
  ScriptedBleConnection({
    this.responses = const {},
    this.autoRespond = true,
    this.ready = true,
  });

  /// Mapa comando (normalizado) → resposta ELM327 crua (com prompt).
  final Map<String, String> responses;

  /// Se responde automaticamente aos [write]. @default true
  final bool autoRespond;

  /// Valor de [isReady]. @default true
  final bool ready;

  /// Comandos escritos (normalizados), na ordem — para asserir serialização.
  final List<String> written = [];

  final StreamController<List<int>> _incoming =
      StreamController<List<int>>.broadcast();

  @override
  Stream<List<int>> get incoming => _incoming.stream;

  /// `true` enquanto houver assinante da TX — para asserir teardown do cliente.
  bool get hasIncomingListener => _incoming.hasListener;

  @override
  bool get isReady => ready;

  @override
  Future<void> write(List<int> bytes) async {
    final cmd = String.fromCharCodes(
      bytes,
    ).replaceAll('\r', '').replaceAll(' ', '').toUpperCase();
    written.add(cmd);
    if (!autoRespond) return;
    final resp = responses[cmd];
    if (resp == null) return;
    scheduleMicrotask(() => emit(resp));
  }

  /// Emite [text] (ASCII) na TX, se ainda aberta.
  void emit(String text) {
    if (!_incoming.isClosed) _incoming.add(ascii.encode(text));
  }

  @override
  Stream<BleConnectionPhase> get phase =>
      Stream<BleConnectionPhase>.value(BleConnectionPhase.ready);

  @override
  BleConnectionPhase get currentPhase => BleConnectionPhase.ready;

  @override
  Future<void> disconnect() async {
    if (!_incoming.isClosed) await _incoming.close();
  }
}
