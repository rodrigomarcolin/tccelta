import 'dart:typed_data';

import 'package:meta/meta.dart';

/// Identificação BLE do simulador de GPS (periférico no PC).
abstract final class SimulatedGpsContract {
  /// Nome anunciado.
  static const String name = 'TCCelta-SIM';

  /// UUID do serviço GATT (vai no anúncio).
  static const String serviceUuid = '8f1d0001-7c2b-4e5a-9b3c-5a4e2d1c0b01';

  /// Característica de escrita (app → simulador); sem uso.
  static const String rxUuid = '8f1d0002-7c2b-4e5a-9b3c-5a4e2d1c0b01';

  /// Característica de notificação (simulador → app) com o payload.
  static const String txUuid = '8f1d0003-7c2b-4e5a-9b3c-5a4e2d1c0b01';
}

/// Uma notificação do simulador de GPS. Formato v1, 16 bytes little-endian:
/// `u8 version`, `u8 flags` (bit 0 = posição válida), `u16 seq`,
/// `i32 latE7`, `i32 lonE7`, `i32 altMm`.
@immutable
class SimulatedGpsPayload {
  /// Cria o payload já decodificado.
  const SimulatedGpsPayload({
    required this.seq,
    required this.valid,
    required this.latE7,
    required this.lonE7,
    required this.altMm,
  });

  /// Versão do formato aceita.
  static const int version = 1;

  /// Tamanho exato do payload, em bytes.
  static const int length = 16;

  static const int _validFlag = 0x01;
  static const int _maxLatE7 = 900000000;
  static const int _maxLonE7 = 1800000000;

  /// Decodifica [bytes]; `null` com tamanho ou versão diferentes do formato,
  /// ou com latitude/longitude fora das faixas.
  static SimulatedGpsPayload? tryDecode(List<int> bytes) {
    if (bytes.length != length) return null;
    final data = ByteData.sublistView(Uint8List.fromList(bytes));
    if (data.getUint8(0) != version) return null;
    final latE7 = data.getInt32(4, Endian.little);
    final lonE7 = data.getInt32(8, Endian.little);
    if (latE7.abs() > _maxLatE7 || lonE7.abs() > _maxLonE7) return null;
    return SimulatedGpsPayload(
      seq: data.getUint16(2, Endian.little),
      valid: (data.getUint8(1) & _validFlag) != 0,
      latE7: latE7,
      lonE7: lonE7,
      altMm: data.getInt32(12, Endian.little),
    );
  }

  /// Número de sequência (dá a volta em 65535).
  final int seq;

  /// O simulador tem uma posição válida.
  final bool valid;

  /// Latitude em graus × 10⁷.
  final int latE7;

  /// Longitude em graus × 10⁷.
  final int lonE7;

  /// Altitude em milímetros.
  final int altMm;

  @override
  bool operator ==(Object other) =>
      other is SimulatedGpsPayload &&
      other.seq == seq &&
      other.valid == valid &&
      other.latE7 == latE7 &&
      other.lonE7 == lonE7 &&
      other.altMm == altMm;

  @override
  int get hashCode => Object.hash(seq, valid, latE7, lonE7, altMm);
}
