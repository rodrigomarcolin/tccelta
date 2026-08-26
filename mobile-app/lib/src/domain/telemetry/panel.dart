import 'package:flutter/foundation.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/telemetry/indicator_display.dart';

/// Limite de caracteres do nome de um [Panel] — mesmo teto usado pelo campo
/// de renomear (`_PanelEdit`) e pela validação do `PanelViewModel`.
abstract final class PanelNameLimits {
  /// Máximo de caracteres.
  static const int max = 30;
}

/// Um painel de indicadores do Painel de telemetria: um nome e o conjunto de
/// indicadores exibidos, em que ordem e com que customização.
///
/// Modelo de domínio puro (`@immutable`, sem `freezed`/`json_serializable` —
/// mesmo estilo de `IndicatorDisplay`): só a necessidade de serializar para
/// uma futura persistência local, coberta por [toJson].
///
/// [indicatorIds] guarda `Obd2Pid.name` em vez do enum diretamente — trivial
/// de serializar — e sua ordem é a própria ordem de exibição no grid.
/// [displays] guarda a customização de cada indicador, também chaveada por
/// `Obd2Pid.name`. [id] é a identidade estável do painel — não muda via
/// [copyWith].
@immutable
class Panel {
  /// Cria um painel. [indicatorIds]/[displays] vazios = painel sem
  /// indicadores ainda.
  const Panel({
    required this.id,
    required this.name,
    this.indicatorIds = const [],
    this.displays = const {},
  });

  /// Identidade estável do painel, gerada ao criá-lo. Não muda via
  /// [copyWith].
  final String id;

  /// Nome exibido do painel (editável pelo usuário).
  final String name;

  /// Identificadores (`Obd2Pid.name`) dos indicadores adicionados, na ordem
  /// de exibição.
  final List<String> indicatorIds;

  /// Customização de exibição de cada indicador adicionado, por
  /// `Obd2Pid.name`.
  final Map<String, IndicatorDisplay> displays;

  /// Os PIDs adicionados, na ordem de exibição.
  List<Obd2Pid> get indicators =>
      indicatorIds.map(Obd2Pid.values.byName).toList(growable: false);

  /// Se [pid] já foi adicionado a este painel.
  bool contains(Obd2Pid pid) => indicatorIds.contains(pid.name);

  /// Customização de exibição de [pid] — os defaults do PID quando ele ainda
  /// não tem uma customização salva (não deveria acontecer para um
  /// indicador já adicionado, mas mantém o getter total).
  IndicatorDisplay displayFor(Obd2Pid pid) =>
      displays[pid.name] ?? IndicatorDisplay.defaultFor(pid);

  /// Cópia com os campos sobrescritos. [id] nunca muda.
  Panel copyWith({
    String? name,
    List<String>? indicatorIds,
    Map<String, IndicatorDisplay>? displays,
  }) => Panel(
    id: id,
    name: name ?? this.name,
    indicatorIds: indicatorIds ?? this.indicatorIds,
    displays: displays ?? this.displays,
  );

  /// Serializa para um `Map` codificável em JSON — a peça que deixa o painel
  /// pronto para uma futura persistência local (ainda fora de escopo).
  Map<String, dynamic> toJson() => {
    'id': id,
    'name': name,
    'indicatorIds': indicatorIds,
    'displays': displays.map((id, d) => MapEntry(id, d.toJson())),
  };
}
