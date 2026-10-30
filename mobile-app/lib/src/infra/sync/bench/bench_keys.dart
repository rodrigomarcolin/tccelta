import 'dart:typed_data';

/// Chaves do servidor de bancada. Os padrões são as chaves fixas de
/// desenvolvimento do firmware (`SECURITY_DEV_FIXED_KEYS`) e nunca valem para
/// um dongle de produção.
class BenchKeys {
  /// Cria o conjunto de chaves.
  const BenchKeys({required this.kAuth, required this.kRec});

  /// Padrão: chaves fixas de bancada.
  factory BenchKeys.defaults() => BenchKeys(
    kAuth: _fromHex(_defaultKAuth),
    kRec: _fromHex(_defaultKRec),
  );

  /// Lê `SYNC_BENCH_KAUTH` e `SYNC_BENCH_KREC` (hex de 64 dígitos) de
  /// `--dart-define`; ausentes, valem as chaves padrão.
  factory BenchKeys.fromEnvironment() {
    const kAuth = String.fromEnvironment('SYNC_BENCH_KAUTH');
    const kRec = String.fromEnvironment('SYNC_BENCH_KREC');
    return BenchKeys(
      kAuth: _fromHex(kAuth.isEmpty ? _defaultKAuth : kAuth),
      kRec: _fromHex(kRec.isEmpty ? _defaultKRec : kRec),
    );
  }

  static const String _defaultKAuth =
      '0102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F20';
  static const String _defaultKRec =
      '2122232425262728292A2B2C2D2E2F303132333435363738393A3B3C3D3E3F40';

  /// Chave do HMAC das mensagens de handshake.
  final Uint8List kAuth;

  /// Chave do HMAC dos blocos.
  final Uint8List kRec;

  static Uint8List _fromHex(String hex) {
    if (hex.length != 64) {
      throw FormatException('chave deve ter 64 dígitos hex: $hex');
    }
    return Uint8List.fromList([
      for (var i = 0; i < hex.length; i += 2)
        int.parse(hex.substring(i, i + 2), radix: 16),
    ]);
  }
}
