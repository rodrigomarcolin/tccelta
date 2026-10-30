import 'dart:convert';
import 'dart:typed_data';

import 'package:crypto/crypto.dart';

/// Tag de handshake: `HMAC-SHA256_{K_auth}("MALCONG" ‖ tipo ‖ DevId ‖
/// payload)` truncado a 16 bytes.
Uint8List handshakeTag({
  required Uint8List kAuth,
  required int type,
  required Uint8List devId,
  required Uint8List payload,
}) => Uint8List.fromList(
  _mac(kAuth, type, devId, payload).sublist(0, handshakeTagLength),
);

/// Tag de bloco: `HMAC-SHA256_{K_rec}("MALCONG" ‖ 0x03 ‖ DevId ‖ dados)`,
/// 32 bytes, em que `dados` é a área de dados (cabeçalho + registros).
Uint8List blockTag({
  required Uint8List kRec,
  required Uint8List devId,
  required Uint8List data,
}) => _mac(kRec, 0x03, devId, data);

/// Tamanho da tag de handshake.
const int handshakeTagLength = 16;

/// Tamanho da tag de bloco no perfil `hmac`.
const int blockTagLength = 32;

/// Compara em tempo constante.
bool constantTimeEquals(List<int> a, List<int> b) {
  if (a.length != b.length) return false;
  var diff = 0;
  for (var i = 0; i < a.length; i++) {
    diff |= a[i] ^ b[i];
  }
  return diff == 0;
}

Uint8List _mac(Uint8List key, int type, Uint8List devId, Uint8List data) {
  final input = BytesBuilder(copy: false)
    ..add(ascii.encode('MALCONG'))
    ..addByte(type)
    ..add(devId)
    ..add(data);
  return Uint8List.fromList(
    Hmac(sha256, key).convert(input.toBytes()).bytes,
  );
}
