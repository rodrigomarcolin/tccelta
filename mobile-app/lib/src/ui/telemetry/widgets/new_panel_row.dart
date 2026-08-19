import 'package:flutter/material.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';

/// Linha tracejada "Novo painel" — cria um painel novo ao ser tocada.
///
/// Compartilhada entre `PanelManagerSheet` (fim da lista "Meus painéis") e
/// `PanelPickerSheet` (fim da lista de escolha de painel ao adicionar um
/// sensor pela aba "Sensores").
class NewPanelRow extends StatelessWidget {
  /// Cria a linha "Novo painel".
  const NewPanelRow({required this.onTap, super.key});

  /// Chamado ao tocar a linha.
  final VoidCallback onTap;

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
          border: Border.all(color: AppColors.borderStrong),
          borderRadius: AppRadii.brMd,
        ),
        child: Row(
          children: [
            const Icon(
              Icons.add_rounded,
              size: 18,
              color: AppColors.textTertiary,
            ),
            const SizedBox(width: AppSpacing.s3),
            Text(
              'Novo painel',
              style: AppTypography.label.copyWith(
                color: AppColors.textTertiary,
              ),
            ),
          ],
        ),
      ),
    );
  }
}
