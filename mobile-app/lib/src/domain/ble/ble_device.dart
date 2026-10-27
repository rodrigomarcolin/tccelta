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
    this.manufacturerData = const {},
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

  /// Dados de fabricante do anúncio (Company ID -> bytes). Genérico: quem os
  /// interpreta é o chamador (ex.: `DonglePairingHint`).
  final Map<int, List<int>> manufacturerData;

  /// Rótulo para exibição: o [name] anunciado ou, se vazio, o [id]
  /// (MAC no Android, UUID no iOS) — nunca vazio.
  String get displayName => name.isNotEmpty ? name : id;

  /// Cópia com campos sobrescritos.
  BleDevice copyWith({
    String? id,
    String? name,
    int? rssi,
    BleSignalLevel? signal,
    Map<int, List<int>>? manufacturerData,
  }) => BleDevice(
    id: id ?? this.id,
    name: name ?? this.name,
    rssi: rssi ?? this.rssi,
    signal: signal ?? this.signal,
    manufacturerData: manufacturerData ?? this.manufacturerData,
  );

  // `signal` é derivado de `rssi`, então fica FORA de ==/hashCode. Os dados de
  // fabricante entram: o estado da janela de pareamento muda sem mudar
  // id/nome/rssi.
  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is BleDevice &&
          other.id == id &&
          other.name == name &&
          other.rssi == rssi &&
          _sameManufacturerData(other.manufacturerData, manufacturerData);

  @override
  int get hashCode => Object.hash(id, name, rssi, manufacturerData.length);

  static bool _sameManufacturerData(
    Map<int, List<int>> a,
    Map<int, List<int>> b,
  ) {
    if (a.length != b.length) return false;
    for (final entry in a.entries) {
      final other = b[entry.key];
      if (other == null || other.length != entry.value.length) return false;
      for (var i = 0; i < other.length; i++) {
        if (other[i] != entry.value[i]) return false;
      }
    }
    return true;
  }
}
