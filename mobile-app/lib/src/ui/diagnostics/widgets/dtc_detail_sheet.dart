import 'package:flutter/material.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_code.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/diagnostics/dtc_severity_color.dart';
import 'package:tccelta_mobile/src/ui/diagnostics/widgets/dtc_row.dart';

/// Abre o sheet de detalhe de um [DtcCode]: código, status, causas prováveis
/// e o congelamento de dados (freeze frame) no momento da falha, quando
/// disponível.
///
/// Sem resultado (mesmo padrão de `showPanelManagerSheet`) — o sheet é só
/// informativo, "Fechar" apenas descarta.
Future<void> showDtcDetailSheet(BuildContext context, {required DtcCode dtc}) {
  return showModalBottomSheet<void>(
    context: context,
    isScrollControlled: true,
    backgroundColor: Colors.transparent,
    builder: (context) => DtcDetailSheet(dtc: dtc),
  );
}

/// Conteúdo do sheet de detalhe de DTC.
class DtcDetailSheet extends StatelessWidget {
  /// Cria o sheet de detalhe para [dtc].
  const DtcDetailSheet({required this.dtc, super.key});

  /// O código detalhado.
  final DtcCode dtc;

  @override
  Widget build(BuildContext context) {
    final color = dtcSeverityColor(dtc.severity);
    return DecoratedBox(
      decoration: const BoxDecoration(
        color: AppColors.surfaceCard,
        borderRadius: BorderRadius.vertical(top: Radius.circular(26)),
      ),
      child: SafeArea(
        top: false,
        child: ConstrainedBox(
          constraints: BoxConstraints(
            maxHeight: MediaQuery.sizeOf(context).height * 0.92,
          ),
          child: Padding(
            padding: const EdgeInsets.fromLTRB(
              AppSpacing.s7,
              AppSpacing.s4,
              AppSpacing.s7,
              AppSpacing.s7,
            ),
            child: Column(
              mainAxisSize: MainAxisSize.min,
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                const _DragHandle(),
                const SizedBox(height: AppSpacing.s4),
                Flexible(
                  child: SingleChildScrollView(
                    child: Column(
                      crossAxisAlignment: CrossAxisAlignment.start,
                      mainAxisSize: MainAxisSize.min,
                      children: [
                        Row(
                          children: [
                            Text(
                              dtc.code,
                              style: AppTypography.mono(
                                TextStyle(
                                  fontSize: 22,
                                  fontWeight: FontWeight.w700,
                                  color: color,
                                ),
                              ),
                            ),
                            const SizedBox(width: AppSpacing.s3),
                            if (dtc.status == null)
                              const StatusBadge(
                                label: 'INATIVO',
                                tone: StatusTone.neutral,
                              )
                            else
                              dtcStatusBadge(dtc.status!),
                          ],
                        ),
                        const SizedBox(height: AppSpacing.s3),
                        Text(
                          dtc.name,
                          style: AppTypography.title.copyWith(height: 1.35),
                        ),
                        const SizedBox(height: AppSpacing.s3),
                        Row(
                          children: [
                            _InfoChip(text: dtc.component.label),
                            const SizedBox(width: AppSpacing.s2),
                            _InfoChip(
                              text: dtcSeverityLabel(dtc.severity),
                              color: color,
                            ),
                          ],
                        ),
                        if (dtc.detectedLabel != null) ...[
                          const SizedBox(height: AppSpacing.s3),
                          Text(
                            'detectado ${dtc.detectedLabel}',
                            style: AppTypography.mono(
                              const TextStyle(
                                fontSize: 12,
                                color: AppColors.textTertiary,
                              ),
                            ),
                          ),
                        ],
                        if (dtc.causes.isNotEmpty) ...[
                          const SizedBox(height: AppSpacing.s7),
                          Text(
                            'POSSÍVEIS CAUSAS',
                            style: AppTypography.overline,
                          ),
                          const SizedBox(height: AppSpacing.s3),
                          BulletList(items: dtc.causes),
                        ],
                        if (dtc.freezeFrame.isNotEmpty) ...[
                          const SizedBox(height: AppSpacing.s7),
                          Text(
                            'CONGELAMENTO NO MOMENTO DA FALHA',
                            style: AppTypography.overline,
                          ),
                          const SizedBox(height: AppSpacing.s3),
                          Wrap(
                            spacing: AppSpacing.s2,
                            runSpacing: AppSpacing.s2,
                            children: [
                              for (final f in dtc.freezeFrame)
                                SizedBox(
                                  width:
                                      (MediaQuery.sizeOf(context).width -
                                          2 * AppSpacing.s7 -
                                          AppSpacing.s2) /
                                      2,
                                  child: StatCard.value(
                                    label: f.label,
                                    value: f.value,
                                  ),
                                ),
                            ],
                          ),
                        ],
                        const SizedBox(height: AppSpacing.s7),
                        AppButton(
                          onPressed: Navigator.of(context).pop,
                          child: const Text('Fechar'),
                        ),
                      ],
                    ),
                  ),
                ),
              ],
            ),
          ),
        ),
      ),
    );
  }
}

class _InfoChip extends StatelessWidget {
  const _InfoChip({required this.text, this.color});

  final String text;
  final Color? color;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(
        horizontal: AppSpacing.s3,
        vertical: 5,
      ),
      decoration: BoxDecoration(
        color: AppColors.surfaceSunken,
        borderRadius: AppRadii.brSm,
        border: Border.all(color: AppColors.borderHairline),
      ),
      child: Text(
        text,
        style: AppTypography.ui(
          TextStyle(
            fontSize: 11,
            fontWeight: FontWeight.w600,
            color: color ?? AppColors.textSecondary,
          ),
        ),
      ),
    );
  }
}

class _DragHandle extends StatelessWidget {
  const _DragHandle();

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Container(
        width: 40,
        height: 4,
        decoration: const BoxDecoration(
          color: Color(0x2EFFFFFF),
          borderRadius: AppRadii.brPill,
        ),
      ),
    );
  }
}
