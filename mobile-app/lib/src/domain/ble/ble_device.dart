import 'package:meta/meta.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_signal_level.dart';

/// Um dispositivo BLE visto durante o scan.
///
/// Modelo de domínio puro (sem `flutter_blue_plus`): o adapter converte o
/// `ScanResult` da lib nisto antes de subir para as camadas superiores.
@immutable
class BleDevice {
  /// Cria um dispositivo com [id], [name] e [rssi].
  ///
  /// [signal] tem default [BleSignalLevel.weak] apenas para o adapter poder
  /// construir sem conhecer a regra de classificação — o datasource sobrescreve
  /// com o nível real (`BleSignalLevel.fromRssi`) antes de subir para a UI.
  const BleDevice({
    required this.id,
    required this.name,
    required this.rssi,
    this.signal = BleSignalLevel.weak,
  });

  /// Id opaco de plataforma (remoteId) — usado para (re)conectar.
  ///
  /// No Android é o MAC (`06:E5:28:3B:FD:E0`); no iOS é um UUID de 128 bits.
  final String id;

  /// Nome anunciado pelo dispositivo (ex.: `OBD2Dongle`).
  final String name;

  /// Intensidade do sinal em dBm (para ordenar/mostrar barras). Mais próximo
  /// de zero = sinal mais forte.
  final int rssi;

  /// Nível de sinal classificado a partir do [rssi] (preenchido no datasource).
  final BleSignalLevel signal;

  /// Rótulo para exibição: o [name] anunciado ou, se vazio, o [id]
  /// (MAC no Android, UUID no iOS) — nunca vazio.
  String get displayName => name.isNotEmpty ? name : id;

  /// Cópia com campos sobrescritos.
  BleDevice copyWith({
    String? id,
    String? name,
    int? rssi,
    BleSignalLevel? signal,
  }) => BleDevice(
    id: id ?? this.id,
    name: name ?? this.name,
    rssi: rssi ?? this.rssi,
    signal: signal ?? this.signal,
  );

  // `signal` é derivado de `rssi`, então fica FORA de ==/hashCode: dois
  // dispositivos com mesmo id/name/rssi são iguais (e têm o mesmo nível).
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
