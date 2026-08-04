import 'package:meta/meta.dart';

/// Identidade do adaptador OBD-II conectado — o que a top bar exibe além do
/// nome do dispositivo.
///
/// Modelo de domínio PURO e imutável. Ambos os campos são opcionais: a
/// [version] é lida na inicialização (resposta do `ATZ`) e o [protocol] só é
/// detectável após a primeira troca com o veículo, então podem chegar em
/// momentos diferentes (ou não chegar).
@immutable
class Obd2AdapterInfo {
  /// Cria a identidade com [version] e/ou [protocol].
  const Obd2AdapterInfo({this.version, this.protocol});

  /// Versão/identidade do adaptador ELM327 (ex.: `ELM327 v1.5`).
  final String? version;

  /// Protocolo do barramento em uso (ex.: `ISO 15765-4 (CAN 11/500)`).
  final String? protocol;

  @override
  bool operator ==(Object other) =>
      other is Obd2AdapterInfo &&
      other.version == version &&
      other.protocol == protocol;

  @override
  int get hashCode => Object.hash(version, protocol);

  @override
  String toString() =>
      'Obd2AdapterInfo(version: $version, protocol: $protocol)';
}
