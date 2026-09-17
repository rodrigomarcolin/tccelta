import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// Estado vazio da vista "Ativos": nenhum código ativo (no filtro atual).
///
/// Espelha `_EmptyPanel` do `PainelScreen` — mesma receita `IconTile` grande
/// + título + subtítulo já usada na feature irmã.
class DtcEmptyState extends StatelessWidget {
  /// Cria o estado vazio.
  const DtcEmptyState({super.key});

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(vertical: AppSpacing.s9),
      child: Column(
        children: [
          const IconTile(AppIconData.check, size: 56),
          const SizedBox(height: AppSpacing.s4),
          Text(
            'Nenhum código ativo aqui',
            style: AppTypography.title,
            textAlign: TextAlign.center,
          ),
          const SizedBox(height: AppSpacing.s2),
          Text(
            'Este componente não reportou falhas na última leitura.',
            style: AppTypography.body.copyWith(color: AppColors.textTertiary),
            textAlign: TextAlign.center,
          ),
        ],
      ),
    );
  }
}
