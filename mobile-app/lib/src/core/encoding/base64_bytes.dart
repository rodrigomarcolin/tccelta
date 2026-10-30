import 'dart:convert';
import 'dart:typed_data';

/// Base64 padrão (RFC 4648 §4) com *padding* — o formato aceito pelo servidor.
abstract final class Base64Bytes {
  static final RegExp _alphabet = RegExp(r'^[A-Za-z0-9+/]*={0,2}$');

  /// Codifica [bytes] em base64 com *padding*.
  static String encode(Uint8List bytes) => base64.encode(bytes);

  /// Decodifica [text]; lança [FormatException] se não for base64 padrão com
  /// *padding* (o `base64.decode` do Dart aceita também `-` e `_`).
  static Uint8List decode(String text) {
    if (text.length % 4 != 0 || !_alphabet.hasMatch(text)) {
      throw const FormatException('base64 inválido');
    }
    return base64.decode(text);
  }
}
