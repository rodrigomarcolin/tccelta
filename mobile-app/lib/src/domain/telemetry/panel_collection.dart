import 'package:flutter/foundation.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/telemetry/indicator_display.dart';
import 'package:tccelta_mobile/src/domain/telemetry/panel.dart';

/// Estado observável do conjunto de painéis do usuário e qual está ativo.
///
/// Expõe `indicatorIds`/`displays`/`indicators`/`contains`/`displayFor` como
/// atalhos que delegam para [active] — quem só precisa ler/exibir o painel
/// corrente (o grid do Painel, o ciclo rápido de polling) não muda nada ao
/// ganhar suporte a múltiplos painéis.
///
/// Modelo de domínio puro (`@immutable`, mesmo estilo de [Panel] e
/// [IndicatorDisplay]): é a peça que a camada `data`/`application` persiste
/// (via [toJson]/[PanelsState.fromJson]) — por isso vive no domínio, não na
/// `ui` (onde estava antes de a persistência entrar em escopo).
@immutable
class PanelsState {
  /// Cria o estado dos painéis. [panels] nunca deve ficar vazio — sempre há
  /// ao menos um painel.
  const PanelsState({required this.panels, required this.activeId});

  /// Reconstrói um [PanelsState] a partir de [json] (mesmo formato
  /// produzido por [toJson]).
  factory PanelsState.fromJson(Map<String, dynamic> json) => PanelsState(
    panels: (json['panels'] as List<dynamic>)
        .map((p) => Panel.fromJson(p as Map<String, dynamic>))
        .toList(),
    activeId: json['activeId'] as String,
  );

  /// Todos os painéis do usuário, na ordem de criação.
  final List<Panel> panels;

  /// `id` do painel exibido atualmente no Painel.
  final String activeId;

  /// O painel ativo.
  Panel get active => panels.firstWhere((p) => p.id == activeId);

  /// Atalho para `active.indicatorIds`.
  List<String> get indicatorIds => active.indicatorIds;

  /// Atalho para `active.displays`.
  Map<String, IndicatorDisplay> get displays => active.displays;

  /// Atalho para `active.indicators`.
  List<Obd2Pid> get indicators => active.indicators;

  /// Atalho para `active.contains`.
  bool contains(Obd2Pid pid) => active.contains(pid);

  /// Atalho para `active.displayFor`.
  IndicatorDisplay displayFor(Obd2Pid pid) => active.displayFor(pid);

  /// Cópia com os campos sobrescritos.
  PanelsState copyWith({List<Panel>? panels, String? activeId}) => PanelsState(
    panels: panels ?? this.panels,
    activeId: activeId ?? this.activeId,
  );

  /// Serializa para um `Map` codificável em JSON — a peça que deixa o
  /// conjunto de painéis pronto para a persistência local.
  Map<String, dynamic> toJson() => {
    'panels': panels.map((p) => p.toJson()).toList(),
    'activeId': activeId,
  };
}
