import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/core/icons/app_icon.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/status_badge.dart';

/// Caixa de aviso tonalizada: ícone do tom à esquerda + texto de apoio.
///
/// Usada para contextualizar um estado (ex.: o aviso âmbar "o dongle continua
/// anunciando" na tela de Bluetooth desligado). Fundo wash do [tone], borda do
/// tom, ícone do tom; o texto fica em [AppColors.textSecondary].
class Callout extends StatelessWidget {
  /// Cria um callout com [text] e [icon] no [tone] dado.
  const Callout({
    required this.text,
    this.icon = AppIconData.info,
    this.tone = StatusTone.warning,
    super.key,
  });

  /// Texto do aviso.
  final String text;

  /// Ícone à esquerda. @default [AppIconData.info]
  final AppIconData icon;

  /// Tom semântico. @default [StatusTone.warning]
  final StatusTone tone;

  @override
  Widget build(BuildContext context) {
    return Container(
      padding: const EdgeInsets.symmetric(
        horizontal: AppSpacing.s5,
        vertical: AppSpacing.s5,
      ),
      decoration: BoxDecoration(
        color: tone.wash,
        borderRadius: AppRadii.brLg,
        border: Border.all(color: tone.border),
      ),
      child: Row(
        crossAxisAlignment: CrossAxisAlignment.start,
        children: [
          AppIcon(icon, size: 20, color: tone.color),
          const SizedBox(width: AppSpacing.s4),
          Expanded(
            child: Text(
              text,
              style: AppTypography.ui(
                const TextStyle(
                  fontSize: 13,
                  fontWeight: FontWeight.w500,
                  height: 1.5,
                  color: AppColors.textSecondary,
                ),
              ),
            ),
          ),
        ],
      ),
    );
  }
}
