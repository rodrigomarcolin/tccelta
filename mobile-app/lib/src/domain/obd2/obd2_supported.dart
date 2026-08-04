/// Decodificação do bitmask de PIDs suportados (Serviço 0x01, PID 0x00/0x20/…).
///
/// Regra de negócio PURA, no mesmo espírito de `Obd2Pid.decode`: interpreta os
/// 4 bytes crus (já extraídos pelo datasource) e diz **quais PIDs** estão
/// ligados. A varredura de múltiplos ranges (enviar `0120`, `0140`…) é
/// orquestração de fio e vive no datasource; aqui entra só a matemática de
/// bits.
library;

/// Números de PID suportados dentro de um range de 32, a partir de [base].
///
/// Os 4 [bytes] formam 32 bits MSB-first: o bit mais significativo do 1º byte
/// representa o PID `base+1`, o menos significativo do 4º byte o PID
/// `base+0x20`.
/// Ex.: `base=0x00`, `bytes=[0x18,0x1E,0x80,0x00]` → {0x04, 0x05, 0x0C, 0x0D,
/// 0x0E, 0x0F, 0x11}. Retorna vazio se vierem menos de 4 bytes.
Set<int> supportedPidNumbersFromBitmap(int base, List<int> bytes) {
  if (bytes.length < 4) return const {};
  final result = <int>{};
  for (var byteIndex = 0; byteIndex < 4; byteIndex++) {
    final b = bytes[byteIndex];
    for (var bit = 0; bit < 8; bit++) {
      // bit 7 (MSB) do byte 0 = 1º PID do range.
      final isSet = (b & (0x80 >> bit)) != 0;
      if (isSet) result.add(base + byteIndex * 8 + bit + 1);
    }
  }
  return result;
}

/// `true` se o bitmask sinaliza suporte ao PID `base+0x20` — o PID-meta que
/// pede o próximo range (`0120`, `0140`…). É o bit menos significativo do 4º
/// byte. Retorna `false` se vierem menos de 4 bytes.
bool bitmapHasNextRange(List<int> bytes) {
  if (bytes.length < 4) return false;
  return (bytes[3] & 0x01) != 0;
}
