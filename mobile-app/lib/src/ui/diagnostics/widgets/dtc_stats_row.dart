import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// Os 2 cards de resumo do topo da aba de Diagnóstico: contagem de códigos
/// ativos e estado da luz de injeção (MIL).
class DtcStatsRow extends StatelessWidget {
  /// Cria a linha de estatísticas.
  const DtcStatsRow({
    required this.activeCount,
    required this.totalCount,
    required this.milOn,
    super.key,
  });

  /// Nº de códigos ativos agora.
  final int activeCount;

  /// Nº total de códigos no catálogo do veículo.
  final int totalCount;

  /// Se a luz de injeção (MIL) está acesa.
  final bool milOn;

  @override
  Widget build(BuildContext context) {
    // `IntrinsicHeight` dá à `Row` uma altura finita (a do card mais alto)
    // antes de `stretch` esticar o outro até ela — sem isso, `stretch` tenta
    // esticar os cards à altura infinita que a `ListView` externa oferece.
    return IntrinsicHeight(
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Expanded(
            child: _ActiveCountCard(active: activeCount, total: totalCount),
          ),
          const SizedBox(width: AppSpacing.s3),
          Expanded(child: _MilStatusCard(milOn: milOn)),
        ],
      ),
    );
  }
}

class _ActiveCountCard extends StatelessWidget {
  const _ActiveCountCard({required this.active, required this.total});

  final int active;
  final int total;

  @override
  Widget build(BuildContext context) {
    return AppCard(
      border: Border.all(color: AppColors.red20),
      child: StatCard.value(
        label: 'Ativos agora',
        value: active,
        unit: 'de $total',
      ),
    );
  }
}

class _MilStatusCard extends StatelessWidget {
  const _MilStatusCard({required this.milOn});

  final bool milOn;

  @override
  Widget build(BuildContext context) {
    final tone = milOn ? StatusTone.warning : StatusTone.neutral;
    return AppCard(
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.start,
        mainAxisSize: MainAxisSize.min,
        children: [
          Text('LUZ DE INJEÇÃO', style: AppTypography.overline),
          const SizedBox(height: AppSpacing.s2),
          Row(
            children: [
              IconTile(AppIconData.motor, tone: tone, size: 30),
              const SizedBox(width: AppSpacing.s3),
              Expanded(
                child: Text(
                  milOn ? 'Acesa' : 'Apagada',
                  style: AppTypography.title.copyWith(color: tone.color),
                  overflow: TextOverflow.ellipsis,
                ),
              ),
            ],
          ),
        ],
      ),
    );
  }
}
