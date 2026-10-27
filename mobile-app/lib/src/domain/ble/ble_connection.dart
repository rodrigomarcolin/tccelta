import 'package:tccelta_mobile/src/domain/ble/ble_link_error.dart';

/// Fases OBSERVÁVEIS pela camada BLE (só transporte e segurança do enlace).
///
/// NÃO inclui init do ELM327 nem leitura de capacidades (bitmask de PIDs):
/// isso roda em cima de uma [BleConnection] já `ready`.
enum BleConnectionPhase {
  /// Ainda não iniciada.
  idle,

  /// Abrindo o link GATT com o dispositivo.
  connecting,

  /// Estabelecendo o enlace cifrado; o SO pode mostrar o pedido de pareamento.
  securing,

  /// Negociando MTU (maior throughput por pacote).
  optimizingLink,

  /// Descobrindo serviços/características (o serviço NUS do dongle).
  discovering,

  /// Habilitando notify na característica TX.
  enablingNotify,

  /// Pronto: `write()` e `incoming` disponíveis.
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

  /// Motivo do último `failed` (nulo se não houve falha).
  BleLinkError? get lastError;

  /// Último valor de fase emitido (evita corrida ao assinar [phase] tarde).
  BleConnectionPhase get currentPhase;

  /// Bytes crus recebidos do dispositivo (característica TX, via notify).
  Stream<List<int>> get incoming;

  /// `true` quando RX/TX estão prontos para uso.
  bool get isReady;

  /// Escreve no canal RX ATUAL — sobrevive à troca de característica após uma
  /// reconexão (a característica antiga fica inválida quando o link cai).
  Future<void> write(List<int> bytes);

  /// Canal RX/TX adicional pedido no `connect` pelo [serviceUuid]; `null` se
  /// não foi pedido.
  BleChannel? channel(String serviceUuid);

  /// MTU ATT negociado (23 enquanto desconhecido).
  int get mtu;

  /// Encerra a conexão e libera os recursos (streams).
  Future<void> disconnect();
}

/// Canal RX/TX adicional de uma [BleConnection]. [incoming] é estável entre
/// reconexões (a mesma stream continua emitindo depois de `reconnecting`).
abstract interface class BleChannel {
  /// Bytes recebidos por notificação.
  Stream<List<int>> get incoming;

  /// `true` quando o serviço foi encontrado e RX/TX estão prontos.
  bool get isReady;

  /// Escreve no canal.
  Future<void> write(List<int> bytes);
}
