import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/icons/app_icon.dart';

/// Uma linha de PID na lista de sensores buscável.
///
/// Pareia um nome humano com o código PID bruto, o valor + unidade ao vivo e
/// uma estrela indicando promoção ao dashboard. Com [available] `false`, o
/// valor escurece para um traço. Tocar abre o detalhe do PID. Espelha o
/// componente `SensorRow`.
class SensorRow extends StatelessWidget {
  /// Cria uma linha para o sensor [name] de código [pid].
  const SensorRow({
    required this.name,
    required this.pid,
    this.value,
    this.unit,
    this.promoted = false,
    this.available = true,
    this.onTap,
    super.key,
  });

  /// Nome legível do sensor.
  final String name;

  /// Código PID OBD-II bruto, ex.: "01 0C".
  final String pid;

  /// Valor atual (já formatado).
  final Object? value;

  /// Sufixo de unidade, ex.: "rpm".
  final String? unit;

  /// Promovido ao dashboard (estrela preenchida). @default false
  final bool promoted;

  /// Exposto pelo veículo. @default true
  final bool available;

  /// Toque na linha (abre o detalhe do PID).
  final VoidCallback? onTap;

  @override
  Widget build(BuildContext context) {
    return GestureDetector(
      onTap: onTap,
      behavior: HitTestBehavior.opaque,
      child: Container(
        constraints: const BoxConstraints(minHeight: 56),
        padding: const EdgeInsets.symmetric(
          horizontal: AppSpacing.s5,
          vertical: AppSpacing.s4,
        ),
        decoration: BoxDecoration(
          color: AppColors.surfaceList,
          borderRadius: AppRadii.brMd,
          border: Border.all(color: AppColors.borderHairline),
        ),
        child: Row(
          children: [
            AppIcon(
              AppIconData.estrela,
              size: 18,
              color: promoted ? AppColors.cyan500 : AppColors.neutral600,
            ),
            const SizedBox(width: AppSpacing.s4),
            Expanded(child: _nameAndPid()),
            const SizedBox(width: AppSpacing.s3),
            _value(),
            const SizedBox(width: AppSpacing.s2),
            const AppIcon(
              AppIconData.chevron,
              size: 16,
              color: AppColors.neutral600,
            ),
          ],
        ),
      ),
    );
  }

  Widget _nameAndPid() {
    return Column(
      crossAxisAlignment: CrossAxisAlignment.start,
      mainAxisSize: MainAxisSize.min,
      children: [
        Text(
          name,
          style: AppTypography.label.copyWith(
            color: available ? AppColors.textPrimary : AppColors.textTertiary,
          ),
          overflow: TextOverflow.ellipsis,
        ),
        const SizedBox(height: 2),
        Text(
          pid,
          style: AppTypography.mono(
            const TextStyle(
              fontSize: 11,
              fontWeight: FontWeight.w500,
              color: AppColors.textTertiary,
            ),
          ),
        ),
      ],
    );
  }

  Widget _value() {
    if (!available) {
      return Text(
        '—',
        style: AppTypography.mono(
          const TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.w700,
            color: AppColors.neutral600,
          ),
        ),
      );
    }
    return Row(
      mainAxisSize: MainAxisSize.min,
      crossAxisAlignment: CrossAxisAlignment.baseline,
      textBaseline: TextBaseline.alphabetic,
      children: [
        Text(
          '${value ?? '—'}',
          style: AppTypography.mono(
            const TextStyle(
              fontSize: 16,
              fontWeight: FontWeight.w700,
              color: AppColors.textPrimary,
            ),
          ),
        ),
        if (unit != null && unit!.isNotEmpty) ...[
          const SizedBox(width: 3),
          Text(
            unit!,
            style: AppTypography.mono(
              const TextStyle(
                fontSize: 11,
                fontWeight: FontWeight.w500,
                color: AppColors.textTertiary,
              ),
            ),
          ),
        ],
      ],
    );
  }
}
