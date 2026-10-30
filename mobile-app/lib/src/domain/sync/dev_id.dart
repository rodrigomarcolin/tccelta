import 'dart:typed_data';

import 'package:meta/meta.dart';

/// Identidade de 8 bytes do dongle.
@immutable
class DevId {
  /// Cria a partir de 8 [bytes] (copiados).
  DevId(List<int> bytes) : bytes = Uint8List.fromList(bytes) {
    if (bytes.length != length) {
      throw ArgumentError.value(bytes.length, 'bytes.length', 'deve ser 8');
    }
  }

  /// Cria a partir de 16 dígitos hexadecimais.
  factory DevId.fromHex(String hex) {
    if (hex.length != length * 2) {
      throw FormatException('DevId deve ter 16 dígitos hex: $hex');
    }
    return DevId([
      for (var i = 0; i < hex.length; i += 2)
        int.parse(hex.substring(i, i + 2), radix: 16),
    ]);
  }

  /// Tamanho em bytes.
  static const int length = 8;

  /// Os 8 bytes (não modificar).
  final Uint8List bytes;

  /// 16 dígitos hexadecimais maiúsculos.
  String get hex => bytes
      .map((b) => b.toRadixString(16).padLeft(2, '0'))
      .join()
      .toUpperCase();

  @override
  bool operator ==(Object other) => other is DevId && other.hex == hex;

  @override
  int get hashCode => hex.hashCode;

  @override
  String toString() => 'DevId($hex)';
}
