import 'package:meta/meta.dart';

/// Um dispositivo BLE visto durante o scan.
///
/// Modelo de domínio puro (sem `flutter_blue_plus`): o adapter converte o
/// `ScanResult` da lib nisto antes de subir para as camadas superiores.
@immutable
class BleDevice {
  /// Cria um dispositivo com [id], [name] e [rssi].
  const BleDevice({required this.id, required this.name, required this.rssi});

  /// Id opaco de plataforma (remoteId) — usado para (re)conectar.
  ///
  /// No Android é o MAC (`06:E5:28:3B:FD:E0`); no iOS é um UUID de 128 bits.
  final String id;

  /// Nome anunciado pelo dispositivo (ex.: `OBD2Dongle`).
  final String name;

  /// Intensidade do sinal em dBm (para ordenar/mostrar barras). Mais próximo
  /// de zero = sinal mais forte.
  final int rssi;

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is BleDevice &&
          other.id == id &&
          other.name == name &&
          other.rssi == rssi;

  @override
  int get hashCode => Object.hash(id, name, rssi);
}
