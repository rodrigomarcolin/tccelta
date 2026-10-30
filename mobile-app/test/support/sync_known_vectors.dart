import 'dart:typed_data';

/// Converte hexadecimal (espaços ignorados) em bytes.
Uint8List hexBytes(String hex) {
  final clean = hex.replaceAll(RegExp(r'\s'), '');
  return Uint8List.fromList([
    for (var i = 0; i < clean.length; i += 2)
      int.parse(clean.substring(i, i + 2), radix: 16),
  ]);
}

/// Vetores da spec 01 §12 (chaves fixas de bancada), transcritos à mão.
abstract final class SyncVectors {
  static final Uint8List devId = hexBytes('544343454C544101');
  static final Uint8List kAuth = hexBytes(
    '0102030405060708090A0B0C0D0E0F101112131415161718191A1B1C1D1E1F20',
  );
  static final Uint8List kRec = hexBytes(
    '2122232425262728292A2B2C2D2E2F303132333435363738393A3B3C3D3E3F40',
  );

  static const String nonce = '000102030405060708090A0B0C0D0E0F';

  /// Desafio: `c_S=7`, `ACK_seq=100`, `c_conf=6`, `flags=0` (V sem a tag).
  static final Uint8List challengePayload = hexBytes(
    '07000000 $nonce 64000000 06000000 00',
  );
  static final Uint8List challengeTag = hexBytes(
    '5E08174F9D21CAA313D52A39DCD31943',
  );

  /// Desafio com diagnóstico: `c_S=8`, `c_conf=7`, `flags=1`, `diag_min=30`.
  static final Uint8List challengeDiagPayload = hexBytes(
    '08000000 $nonce 64000000 07000000 01 1E00',
  );
  static final Uint8List challengeDiagTag = hexBytes(
    '2689763B1FF2BAD8FC7955D4C7ECDEB3',
  );

  /// Resposta: `c_S=7`, `TAIL=65`, `HEAD=130`, `boot_id=3`, `t_ms=123456`.
  static final Uint8List responsePayload = hexBytes(
    '07000000 $nonce 41000000 82000000 0300 40E20100',
  );
  static final Uint8List responseTag = hexBytes(
    '3A93F9DC93EDA9E7DBAD035426EEB061',
  );

  /// ACK: `ACK_seq=130`, `c_conf=7`, `flags=0`.
  static final Uint8List ackPayload = hexBytes('82000000 07000000 00');
  static final Uint8List ackTag = hexBytes('84EB2C1203B2720E492DB89E04F1FAE7');

  /// Aviso: `HEAD=130`, `flags=3`.
  static final Uint8List noticeValue = hexBytes('82000000 03 544343454C544101');

  /// BOOT `seq=192`, `boot_id=4`, `t_ms=812`, `[131,191]`.
  static final Uint8List bootData = hexBytes(
    'C0000000 0400 2C030000 1700 FE 0000 83000000 BF000000',
  );
  static final Uint8List bootTag = hexBytes(
    'E55ECDF18FE15DDDF31661086B87DF51544C089829BD9DF2B44B278BAC333D8D',
  );

  /// LOSS `seq=140`, `[140,141]`.
  static final Uint8List lossData = hexBytes(
    '8C000000 0000 00000000 1700 FD 0000 8C000000 8D000000',
  );
  static final Uint8List lossTag = hexBytes(
    '558FF3EEA60111C3ED541CD7BA3D1BB42CCB5A81CE6ABF71835DA7F747C0FF9C',
  );

  /// Resposta de 53 B em três fragmentos (MTU 23), spec 04 §5.
  static final List<Uint8List> responseFragments = [
    hexBytes('80 02320007000000000102030405060708090A0B'),
    hexBytes('01 0C0D0E0F4100000082000000030040E201003A'),
    hexBytes('02 93F9DC93EDA9E7DBAD035426EEB061'),
  ];
}

/// Concatena [parts].
Uint8List concatBytes(List<List<int>> parts) =>
    Uint8List.fromList([for (final p in parts) ...p]);
