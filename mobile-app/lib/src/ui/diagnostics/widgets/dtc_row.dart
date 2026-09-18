import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_code.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_status.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/diagnostics/dtc_severity_color.dart';

/// Uma linha de DTC: código + nome, faixa de severidade à esquerda quando
/// ativo, pílula de status (confirmado/pendente) à direita. Usada tanto na
/// vista "Ativos" quanto nas seções da vista "Todos por componente" — nesta
/// última, um código inativo aparece esmaecido e sem faixa/pílula.
class DtcRow extends StatelessWidget {
  /// Cria a linha para [dtc].
  const DtcRow({required this.dtc, required this.onTap, super.key});

  /// O código exibido.
  final DtcCode dtc;

  /// Toque na linha — abre o detalhe.
  final VoidCallback onTap;

  @override
  Widget build(BuildContext context) {
    final status = dtc.status;
    return CardButton(
      title: dtc.code,
      titleMono: true,
      titleColor: dtc.isActive ? dtcSeverityColor(dtc.severity) : null,
      subtitle: dtc.name,
      subtitleMono: false,
      showValue: false,
      trailing: status == null ? null : dtcStatusBadge(status),
      accentColor: dtc.isActive ? dtcSeverityColor(dtc.severity) : null,
      dimmed: !dtc.isActive,
      onTap: onTap,
    );
  }
}

/// Pílula de status ("CONFIRMADO"/"PENDENTE") para um [DtcCode] ativo —
/// reaproveita [StatusBadge] (mesma fonte de significado→cor do resto do
/// app), compartilhada entre [DtcRow] e o sheet de detalhe.
StatusBadge dtcStatusBadge(DtcStatus status) => StatusBadge(
  label: status == DtcStatus.confirmed ? 'CONFIRMADO' : 'PENDENTE',
  tone: status == DtcStatus.confirmed ? StatusTone.alert : StatusTone.warning,
);
