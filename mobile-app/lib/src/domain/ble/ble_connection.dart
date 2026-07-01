/// Fases OBSERVÁVEIS pela camada BLE (Fase 1 — só transporte).
///
/// NÃO inclui init do ELM327 nem leitura de capacidades (bitmask de PIDs):
/// isso é Fase 2 e roda em cima de uma [BleConnection] já `ready`.
enum BleConnectionPhase {
  /// Ainda não iniciada.
  idle,

  /// Abrindo o link GATT com o dispositivo.
  connecting,

  /// Negociando MTU (maior throughput por pacote).
  optimizingLink,

  /// Descobrindo serviços/características (o serviço NUS do dongle).
  discovering,

  /// Habilitando notify na característica TX.
  enablingNotify,

  /// Pronto: `write()` e `incoming` disponíveis (fim da Fase 1).
  ready,

  /// O link caiu; refazendo discover + notify automaticamente.
  reconnecting,

  /// Desconectado (encerramento intencional).
  disconnected,

  /// Falha irrecuperável no handshake.
  failed,
}

/// Handle de uma conexão BLE ativa.
///
/// É o "contrato" que a Fase 2 (ELM327) consome:
/// ```dart
/// final elm = Elm327(conn.write);
/// conn.incoming.listen(elm.onBytes);
/// ```
abstract interface class BleConnection {
  /// Stream das transições de fase da conexão.
  Stream<BleConnectionPhase> get phase;

  /// Último valor de fase emitido (evita corrida ao assinar [phase] tarde).
  BleConnectionPhase get currentPhase;

  /// Bytes crus recebidos do dispositivo (característica TX, via notify).
  Stream<List<int>> get incoming;

  /// `true` quando RX/TX estão prontos para uso.
  bool get isReady;

  /// Escreve no canal RX ATUAL — sobrevive à troca de característica após uma
  /// reconexão (a característica antiga fica inválida quando o link cai).
  Future<void> write(List<int> bytes);

  /// Encerra a conexão e libera os recursos (streams).
  Future<void> disconnect();
}
