/// Tipo (`T`) de uma mensagem TLV do serviço SYNC.
enum SyncMessageType {
  /// Desafio do servidor, repassado ao dongle.
  challenge(0x01),

  /// Resposta do dongle ao desafio.
  response(0x02),

  /// Bloco de dados (ou marcador) com tag.
  block(0x03),

  /// Confirmação (ACK) do servidor.
  ack(0x04),

  /// Pedido de lote de blocos.
  batchRequest(0x05),

  /// Erro (Tabela 5).
  error(0x06),

  /// Aviso de novo bloco.
  notice(0x07);

  const SyncMessageType(this.code);

  /// Valor do byte `T`.
  final int code;

  /// Tipo com o byte [code], ou `null` se desconhecido.
  static SyncMessageType? fromCode(int code) {
    for (final t in values) {
      if (t.code == code) return t;
    }
    return null;
  }
}

/// Códigos de erro da Tabela 5.
enum SyncErrorCode {
  /// Tag inválida.
  invalidTag(0x01),

  /// Desafio com contador antigo.
  staleChallenge(0x02),

  /// `ACK_seq` inválido.
  invalidAckSeq(0x03),

  /// Mensagem incompleta.
  incompleteMessage(0x04),

  /// Sem sessão aberta.
  noSession(0x05),

  /// Reenviar a partir da fronteira (`ACK + 1`).
  resendFromBoundary(0x07);

  const SyncErrorCode(this.code);

  /// Valor do byte de erro.
  final int code;

  /// Código com o byte [code], ou `null` se desconhecido.
  static SyncErrorCode? fromCode(int code) {
    for (final e in values) {
      if (e.code == code) return e;
    }
    return null;
  }
}

/// Maior `L` aceito num TLV (bloco de 1024 B + tag de 64 B).
const int kMaxSyncValueLength = 1088;

/// Máximo de blocos por POST.
const int kMaxBlocksPerPost = 32;

/// Tempo parado numa mensagem fragmentada que a torna incompleta.
const Duration kFragmentTimeout = Duration(seconds: 2);

/// Espera por cada passo BLE do handshake.
const Duration kHandshakeStepTimeout = Duration(seconds: 5);

/// Espera por blocos depois de um pedido de lote.
const Duration kBatchStallTimeout = Duration(seconds: 3);

/// Margem de validade do token abaixo da qual se renova o handshake.
const Duration kTokenRenewMargin = Duration(minutes: 5);

/// Menor espera entre tentativas.
const Duration kRetryMin = Duration(seconds: 1);

/// Maior espera entre tentativas.
const Duration kRetryMax = Duration(minutes: 5);

/// Tamanho do `V` do Desafio sem diagnóstico.
const int kChallengeLength = 45;

/// Tamanho do `V` do Desafio com diagnóstico.
const int kChallengeDiagLength = 47;

/// Tamanho do `V` da Resposta.
const int kResponseLength = 50;

/// Tamanho do `V` do ACK.
const int kAckLength = 25;

/// Tamanho do `V` do aviso de novo bloco.
const int kNoticeLength = 13;

/// Tamanho do cabeçalho da área de dados de um bloco.
const int kBlockHeaderSize = 12;
