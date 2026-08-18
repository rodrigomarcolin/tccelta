import 'package:flutter/material.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/app_button.dart';

/// Abre um [ConfirmDialog] e resolve `true` quando o usuário confirma,
/// `false`/`null` quando cancela ou descarta o diálogo.
Future<bool?> showConfirmDialog(
  BuildContext context, {
  required String title,
  required String message,
  String confirmLabel = 'Confirmar',
  String cancelLabel = 'Cancelar',
  bool destructive = true,
}) {
  return showDialog<bool>(
    context: context,
    builder: (context) => ConfirmDialog(
      title: title,
      message: message,
      confirmLabel: confirmLabel,
      cancelLabel: cancelLabel,
      destructive: destructive,
    ),
  );
}

/// Diálogo de confirmação genérico do design system: título + mensagem e
/// dois botões (cancelar/confirmar) com espaço visível entre eles.
///
/// [destructive] (padrão `true`) estiliza o botão de confirmação em
/// [AppButtonVariant.danger] (ação irreversível, ex.: remover); `false` usa
/// [AppButtonVariant.primary] (ação comum, não destrutiva, ex.: confirmar uma
/// escolha). Prefira [showConfirmDialog] para abrir — este widget existe
/// separado para ser testável isoladamente (sem depender do fluxo de
/// `showDialog`).
class ConfirmDialog extends StatelessWidget {
  /// Cria o diálogo de confirmação.
  const ConfirmDialog({
    required this.title,
    required this.message,
    this.confirmLabel = 'Confirmar',
    this.cancelLabel = 'Cancelar',
    this.destructive = true,
    super.key,
  });

  /// Título do diálogo.
  final String title;

  /// Mensagem de apoio (explica a consequência da ação).
  final String message;

  /// Rótulo do botão de confirmação. @default 'Confirmar'
  final String confirmLabel;

  /// Rótulo do botão de cancelamento. @default 'Cancelar'
  final String cancelLabel;

  /// `true` = ação destrutiva (botão vermelho); `false` = ação comum (botão
  /// ciano). @default true
  final bool destructive;

  @override
  Widget build(BuildContext context) {
    return AlertDialog(
      backgroundColor: AppColors.surfaceCard,
      shape: const RoundedRectangleBorder(borderRadius: AppRadii.brLg),
      title: Text(
        title,
        style: AppTypography.ui(
          const TextStyle(
            fontSize: 16,
            fontWeight: FontWeight.w700,
            color: AppColors.textPrimary,
          ),
        ),
      ),
      content: Text(
        message,
        style: AppTypography.body.copyWith(color: AppColors.textTertiary),
      ),
      actions: [
        Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            AppButton(
              variant: AppButtonVariant.secondary,
              fullWidth: false,
              onPressed: () => Navigator.of(context).pop(false),
              child: Text(cancelLabel),
            ),
            const SizedBox(width: AppSpacing.s3),
            AppButton(
              variant: destructive
                  ? AppButtonVariant.danger
                  : AppButtonVariant.primary,
              fullWidth: false,
              onPressed: () => Navigator.of(context).pop(true),
              child: Text(confirmLabel),
            ),
          ],
        ),
      ],
    );
  }
}
