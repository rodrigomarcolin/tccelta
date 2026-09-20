import 'package:flutter/painting.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_severity.dart';

/// Mapeia [DtcSeverity] para a cor do design system usada para destacá-la
/// (código, faixa lateral do card, chip de severidade no detalhe).
///
/// Fica na UI (não no domain, que é puro/sem Flutter) — mesmo espírito de
/// `gaugeVariantFor` para `IndicatorGaugeStyle`.
Color dtcSeverityColor(DtcSeverity severity) => switch (severity) {
  DtcSeverity.high => AppColors.red500,
  DtcSeverity.medium => AppColors.amber500,
  DtcSeverity.low => AppColors.neutral200,
};

/// Rótulo pt-BR da severidade (ex.: para o chip "Severidade alta" no detalhe).
String dtcSeverityLabel(DtcSeverity severity) => switch (severity) {
  DtcSeverity.high => 'Severidade alta',
  DtcSeverity.medium => 'Severidade média',
  DtcSeverity.low => 'Severidade baixa',
};
