import 'package:meta/meta.dart';

/// O dongle do último handshake BLE bem sucedido, persistido para reconectar
/// sozinho na próxima abertura do app.
///
/// Modelo de domínio puro e enxuto: diferente de `BleDevice`, não carrega
/// `rssi`/`signal` — não fazem sentido para um dispositivo fora de alcance,
/// só o necessário para (re)conectar (`id`) e exibir (`name`) é persistido.
@immutable
class LastDongle {
  /// Cria o registro do último dongle com [id] e [name].
  const LastDongle({required this.id, required this.name});

  /// Reconstrói a partir do JSON salvo por [toJson].
  factory LastDongle.fromJson(Map<String, dynamic> json) => LastDongle(
    id: json['id'] as String,
    name: json['name'] as String,
  );

  /// Id opaco de plataforma (remoteId) — usado para reconectar.
  final String id;

  /// Nome anunciado pelo dispositivo no momento em que foi salvo.
  final String name;

  /// Serializa para persistência local (JSON).
  Map<String, dynamic> toJson() => {'id': id, 'name': name};

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      other is LastDongle && other.id == id && other.name == name;

  @override
  int get hashCode => Object.hash(id, name);
}
