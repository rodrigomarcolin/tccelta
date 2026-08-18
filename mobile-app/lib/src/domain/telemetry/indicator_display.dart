import 'package:flutter/foundation.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';

/// Formato de exibição escolhido para um indicador do Painel.
enum IndicatorFormat {
  /// Número simples, meia coluna do grid — o tratamento padrão de hoje.
  number,

  /// Número simples, linha inteira do grid (mesma altura, largura dobrada).
  numberFull,

  /// Gauge (anel, arco 270° ou ponteiro) — ver [IndicatorGaugeStyle].
  gauge,

  /// Número + barra de progresso horizontal.
  bar,

  /// Histórico em gráfico (sparkline) das últimas leituras.
  history,
}

/// Formato do arco do gauge, quando `format == IndicatorFormat.gauge`.
///
/// Mantido separado de `GaugeVariant` (que é da camada `ui/core/widgets`) —
/// o domínio não depende da UI; a tela que renderiza o gauge mapeia este
/// enum para `GaugeVariant` na hora de montar o widget.
enum IndicatorGaugeStyle {
  /// Anel de progresso fechado.
  ring,

  /// Arco clássico de velocímetro (270°).
  arc270,

  /// Arco de 180° com zonas baixo/médio/alto + ponteiro.
  needle,
}

/// Tamanho do gauge no grid do Painel: [small] ocupa uma célula normal
/// ("2x2"), [large] ocupa a linha inteira e uma célula mais alta ("4x4").
/// Só se aplica a `format == IndicatorFormat.gauge` — os demais formatos têm
/// tamanho fixo.
enum IndicatorGaugeSize {
  /// Célula normal do grid (meia largura).
  small,

  /// Linha inteira do grid, célula mais alta.
  large,
}

/// Quantidade mínima/máxima de pontos exibidos no formato [IndicatorFormat.history].
abstract final class HistoryPointsRange {
  /// Mínimo de pontos configurável.
  static const int min = 10;

  /// Máximo de pontos configurável.
  static const int max = 50;

  /// Passo do stepper de configuração.
  static const int step = 5;

  /// Quantidade padrão ao adicionar um indicador de histórico pela primeira
  /// vez.
  static const int defaultCount = 20;
}

/// Customização de exibição de um indicador do Painel: formato, escala e —
/// quando aplicável — estilo/tamanho do gauge ou quantidade de pontos do
/// histórico.
///
/// Modelo de domínio puro (`@immutable`, sem `freezed`/`json_serializable` —
/// mesmo estilo de `domain/ble/*`): não há transporte de rede envolvido, só a
/// necessidade de serializar para uma futura persistência local, coberta por
/// [toJson]/[fromJson].
@immutable
class IndicatorDisplay {
  /// Cria a customização de exibição.
  const IndicatorDisplay({
    required this.format,
    required this.min,
    required this.max,
    required this.lowMax,
    required this.highMin,
    this.gaugeStyle = IndicatorGaugeStyle.ring,
    this.gaugeSize = IndicatorGaugeSize.small,
    this.historyPoints = HistoryPointsRange.defaultCount,
  });

  /// Monta um display inicial sensato para [pid]: formato número simples,
  /// escala e zonas a partir dos defaults do PID (`Obd2Pid.defaultMin` etc.),
  /// gauge em anel/pequeno e histórico com a contagem padrão de pontos —
  /// tudo pronto para o usuário customizar a partir daqui.
  factory IndicatorDisplay.defaultFor(Obd2Pid pid) => IndicatorDisplay(
    format: IndicatorFormat.number,
    min: pid.defaultMin,
    max: pid.defaultMax,
    lowMax: pid.defaultLowMax,
    highMin: pid.defaultHighMin,
  );

  /// Reconstrói um [IndicatorDisplay] a partir de [json] (mesmo formato
  /// produzido por [toJson]).
  factory IndicatorDisplay.fromJson(
    Map<String, dynamic> json,
  ) => IndicatorDisplay(
    format: IndicatorFormat.values.byName(json['format'] as String),
    min: (json['min'] as num).toDouble(),
    max: (json['max'] as num).toDouble(),
    gaugeStyle: IndicatorGaugeStyle.values.byName(json['gaugeStyle'] as String),
    gaugeSize: IndicatorGaugeSize.values.byName(json['gaugeSize'] as String),
    lowMax: (json['lowMax'] as num).toDouble(),
    highMin: (json['highMin'] as num).toDouble(),
    historyPoints: json['historyPoints'] as int,
  );

  /// Formato escolhido para exibir o indicador.
  final IndicatorFormat format;

  /// Valor mínimo da escala (gauge/barra). @default 0 (via [defaultFor])
  final double min;

  /// Valor máximo da escala (gauge/barra).
  final double max;

  /// Formato do arco, quando [format] é [IndicatorFormat.gauge].
  final IndicatorGaugeStyle gaugeStyle;

  /// Tamanho no grid, quando [format] é [IndicatorFormat.gauge].
  final IndicatorGaugeSize gaugeSize;

  /// Limite superior da zona "baixo" (ciano) do gauge ponteiro, em valor
  /// absoluto — só usado com [IndicatorGaugeStyle.needle].
  final double lowMax;

  /// Limite inferior da zona "alto" (vermelho) do gauge ponteiro, em valor
  /// absoluto — só usado com [IndicatorGaugeStyle.needle].
  final double highMin;

  /// Quantidade de amostras exibidas no histórico, quando [format] é
  /// [IndicatorFormat.history]. Sempre entre [HistoryPointsRange.min] e
  /// [HistoryPointsRange.max].
  final int historyPoints;

  /// Cópia com os campos sobrescritos.
  IndicatorDisplay copyWith({
    IndicatorFormat? format,
    double? min,
    double? max,
    IndicatorGaugeStyle? gaugeStyle,
    IndicatorGaugeSize? gaugeSize,
    double? lowMax,
    double? highMin,
    int? historyPoints,
  }) => IndicatorDisplay(
    format: format ?? this.format,
    min: min ?? this.min,
    max: max ?? this.max,
    gaugeStyle: gaugeStyle ?? this.gaugeStyle,
    gaugeSize: gaugeSize ?? this.gaugeSize,
    lowMax: lowMax ?? this.lowMax,
    highMin: highMin ?? this.highMin,
    historyPoints: historyPoints ?? this.historyPoints,
  );

  /// Serializa para um `Map` codificável em JSON (enums por nome) — a peça
  /// que deixa o estado do Painel pronto para uma futura persistência local.
  Map<String, dynamic> toJson() => {
    'format': format.name,
    'min': min,
    'max': max,
    'gaugeStyle': gaugeStyle.name,
    'gaugeSize': gaugeSize.name,
    'lowMax': lowMax,
    'highMin': highMin,
    'historyPoints': historyPoints,
  };

  @override
  bool operator ==(Object other) =>
      identical(this, other) ||
      (other is IndicatorDisplay &&
          other.format == format &&
          other.min == min &&
          other.max == max &&
          other.gaugeStyle == gaugeStyle &&
          other.gaugeSize == gaugeSize &&
          other.lowMax == lowMax &&
          other.highMin == highMin &&
          other.historyPoints == historyPoints);

  @override
  int get hashCode => Object.hash(
    format,
    min,
    max,
    gaugeStyle,
    gaugeSize,
    lowMax,
    highMin,
    historyPoints,
  );
}
