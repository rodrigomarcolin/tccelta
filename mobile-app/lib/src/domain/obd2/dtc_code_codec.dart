/// Decodifica um DTC cru de 16 bits (como os Modos 03/07/0A/02 devolvem) no
/// formato padrão SAE J2012 (ex.: `0x0301` → `"P0301"`).
///
/// Regra pura, testável, sem tocar em BLE — mesmo espírito de
/// `Obd2Pid.decode`. Os 2 bits mais altos do byte alto escolhem a letra do
/// sistema (`00`=P poderoso/powertrain, `01`=C chassis, `10`=B body,
/// `11`=U network), os 2 bits seguintes escolhem o primeiro dígito (0–3), e
/// os 3 nibbles restantes são hex literal.
String dtcCodeFromRaw(int raw) {
  const letters = ['P', 'C', 'B', 'U'];
  final letter = letters[(raw >> 14) & 0x3];
  final firstDigit = (raw >> 12) & 0x3;
  final rest = (raw & 0xFFF).toRadixString(16).toUpperCase().padLeft(3, '0');
  return '$letter$firstDigit$rest';
}
