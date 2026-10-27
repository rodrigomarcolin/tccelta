import 'package:meta/meta.dart';

/// Dica de pareamento publicada no anúncio do dongle (dados de fabricante
/// `0xFFFF`, formato versão 1).
///
/// É só uma dica de UX: um atacante pode clonar o anúncio, então nenhuma
/// decisão destrutiva pode depender apenas dela.
@immutable
class DonglePairingHint {
  /// Cria a dica.
  const DonglePairingHint({required this.windowOpen, required this.hasBond});

  /// Company ID usado pelo firmware (reservado pelo SIG para uso interno).
  static const int companyId = 0xFFFF;

  /// Versão do formato do campo.
  static const int formatVersion = 0x01;

  /// A janela de pareamento do dongle está aberta.
  final bool windowOpen;

  /// O dongle guarda um bond.
  final bool hasBond;

  /// Lê a dica dos dados de fabricante do anúncio; `null` se o campo não existe
  /// (firmware antigo, outro aparelho) ou tem outra versão.
  static DonglePairingHint? fromManufacturerData(Map<int, List<int>> data) {
    final bytes = data[companyId];
    if (bytes == null || bytes.length < 2 || bytes[0] != formatVersion) {
      return null;
    }
    return DonglePairingHint(
      windowOpen: bytes[1] & 0x01 != 0,
      hasBond: bytes[1] & 0x02 != 0,
    );
  }

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is DonglePairingHint &&
          other.windowOpen == windowOpen &&
          other.hasBond == hasBond;

  @override
  int get hashCode => Object.hash(windowOpen, hasBond);
}
