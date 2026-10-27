/// Motivo classificado de uma falha de segurança/enlace ao conectar ao dongle.
enum BleLinkError {
  /// O usuário (ou o dongle) recusou o pareamento.
  pairingRejected,

  /// O pareamento não terminou no prazo (diálogo do SO sem resposta).
  pairingTimeout,

  /// O dongle não autorizou este aparelho (janela fechada ou sem bond).
  notAuthorized,

  /// O dongle perdeu o bond deste aparelho (Android: dá para refazer sozinho).
  bondLost,

  /// Idem [bondLost] no iOS: é preciso esquecer o dispositivo nos Ajustes.
  bondLostIos,

  /// O dongle derrubou tudo (botão de pareamento apertado).
  remoteReset,

  /// O dongle recusou de forma transitória (já há uma sessão autorizada).
  busy,

  /// O connect não completou no prazo.
  timeout,

  /// Qualquer outra falha.
  other;

  /// `true` quando o motivo indica que o pareamento com o dongle deixou de
  /// valer e é preciso refazê-lo.
  bool get isPairingLost =>
      this == bondLost ||
      this == bondLostIos ||
      this == notAuthorized ||
      this == remoteReset;
}
