import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_code.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_component.dart';
import 'package:tccelta_mobile/src/ui/diagnostics/widgets/dtc_row.dart';

/// Uma seção de componente na vista "Todos por componente": cabeçalho (dot +
/// nome + descrição + contagem) seguido de uma [DtcRow] por código.
class DtcGroupSection extends StatelessWidget {
  /// Cria a seção de [component] com seus [codes].
  const DtcGroupSection({
    required this.component,
    required this.codes,
    required this.onOpen,
    super.key,
  });

  /// Componente exibido.
  final DtcComponent component;

  /// Códigos do catálogo deste componente (ativos e inativos).
  final List<DtcCode> codes;

  /// Chamado com o código tocado.
  final ValueChanged<DtcCode> onOpen;

  @override
  Widget build(BuildContext context) {
    final activeCount = codes.where((c) => c.isActive).length;
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Container(
          padding: const EdgeInsets.only(bottom: AppSpacing.s3),
          decoration: const BoxDecoration(
            border: Border(bottom: BorderSide(color: AppColors.borderHairline)),
          ),
          child: Row(
            children: [
              Container(
                width: 7,
                height: 7,
                decoration: BoxDecoration(
                  color: activeCount > 0
                      ? AppColors.red500
                      : AppColors.neutral700,
                  shape: BoxShape.circle,
                  boxShadow: activeCount > 0
                      ? const [BoxShadow(color: AppColors.red32, blurRadius: 7)]
                      : null,
                ),
              ),
              const SizedBox(width: AppSpacing.s3),
              Expanded(
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  mainAxisSize: MainAxisSize.min,
                  children: [
                    Text(
                      component.label,
                      style: AppTypography.label.copyWith(
                        fontWeight: FontWeight.w700,
                      ),
                    ),
                    Text(
                      component.hint,
                      style: AppTypography.ui(
                        const TextStyle(
                          fontSize: 11,
                          color: AppColors.textTertiary,
                        ),
                      ),
                    ),
                  ],
                ),
              ),
              Text(
                activeCount > 0
                    ? '$activeCount de ${codes.length} ativos'
                    : '${codes.length} códigos',
                style: AppTypography.mono(
                  TextStyle(
                    fontSize: 11,
                    fontWeight: FontWeight.w700,
                    color: activeCount > 0
                        ? AppColors.red500
                        : AppColors.textTertiary,
                  ),
                ),
              ),
            ],
          ),
        ),
        const SizedBox(height: AppSpacing.s3),
        for (var i = 0; i < codes.length; i++) ...[
          if (i > 0) const SizedBox(height: AppSpacing.s2),
          DtcRow(dtc: codes[i], onTap: () => onOpen(codes[i])),
        ],
      ],
    );
  }
}
