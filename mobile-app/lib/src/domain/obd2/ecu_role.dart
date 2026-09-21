/// Papel lógico de uma ECU no barramento CAN, mapeado pro identificador de
/// resposta ISO 15765-4 real (convenção confirmada no simulador de ECUs,
/// `simulador/ECUSim/ECUSim.h`, e em `HANDOFF_DTC.md`): ECM responde em
/// `0x7E8`, TCM em `0x7E9`. Não existe endereçamento de requisição por ECU
/// (o dongle sempre manda um broadcast funcional `0x7DF`) — isso serve só
/// pra filtrar, do lado da resposta, qual `ElmResponse.ecuId` pertence a
/// qual papel.
enum EcuRole {
  /// Módulo de gerenciamento do motor (Engine Control Module).
  ecm(0x7E8),

  /// Módulo de gerenciamento do câmbio (Transmission Control Module).
  tcm(0x7E9);

  const EcuRole(this.responseId);

  /// Identificador CAN (11 bits) da resposta desta ECU.
  final int responseId;
}
