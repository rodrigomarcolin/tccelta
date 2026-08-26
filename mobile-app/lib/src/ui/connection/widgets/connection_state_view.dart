import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/ui/connection/widgets/connection_background.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// Composição centrada reutilizada pelas telas "de estado" do fluxo de conexão
/// (permissões, BT desligado, conectado, conexão perdida).
///
/// Estrutura: um [IconBadge] do estado + título + descrição centralizados, com
/// uma área de ações no rodapé. Os dois slots opcionais cobrem as variações de
/// layout do design:
/// - [centerExtra] entra na região central, sob a descrição (ex.: infos do
///   "Conectado", pílula de valor obsoleto do "Conexão perdida").
/// - [bottomExtra] entra logo acima dos botões (ex.: linhas de permissão,
///   callout de aviso).
class ConnectionStateView extends StatelessWidget {
  /// Cria a tela de estado.
  const ConnectionStateView({
    required this.icon,
    required this.title,
    required this.primaryAction,
    this.tone = StatusTone.ok,
    this.badgeShape = IconBadgeShape.roundedSquare,
    this.pulse = false,
    this.description,
    this.centerExtra,
    this.bottomExtra,
    this.secondaryAction,
    super.key,
  });

  /// Ícone do badge de estado.
  final AppIconData icon;

  /// Título do estado.
  final String title;

  /// Ação primária (normalmente um `AppButton` full-width).
  final Widget primaryAction;

  /// Tom semântico do estado. @default [StatusTone.ok]
  final StatusTone tone;

  /// Formato do badge. @default [IconBadgeShape.roundedSquare]
  final IconBadgeShape badgeShape;

  /// Halo "respirando" no badge (sucesso ao vivo). @default false
  final bool pulse;

  /// Parágrafo de descrição. @default null
  final String? description;

  /// Conteúdo extra na região central, sob a descrição. @default null
  final Widget? centerExtra;

  /// Conteúdo extra logo acima dos botões. @default null
  final Widget? bottomExtra;

  /// Ação secundária (normalmente `AppButton` variante link). @default null
  final Widget? secondaryAction;

  // A descrição recebe um leve tingimento por tom; o vermelho usa o token de
  // texto sobre vermelho translúcido. Demais tons ficam no neutro secundário.
  Color get _descriptionColor =>
      tone == StatusTone.alert ? AppColors.red200 : AppColors.textSecondary;

  @override
  Widget build(BuildContext context) {
    return ConnectionBackground(
      tone: tone,
      child: Padding(
        padding: const EdgeInsets.all(AppSpacing.s7),
        child: Column(
          children: [
            // O herói centraliza quando há espaço e rola quando a tela é curta;
            // a barra de ações fica fixa abaixo.
            Expanded(
              child: LayoutBuilder(
                builder: (context, constraints) {
                  return SingleChildScrollView(
                    child: ConstrainedBox(
                      constraints: BoxConstraints(
                        minHeight: constraints.maxHeight,
                      ),
                      child: Center(child: _hero()),
                    ),
                  );
                },
              ),
            ),
            if (bottomExtra != null) ...[
              bottomExtra!,
              const SizedBox(height: AppSpacing.s5),
            ],
            primaryAction,
            if (secondaryAction != null) ...[
              const SizedBox(height: AppSpacing.s2),
              secondaryAction!,
            ],
          ],
        ),
      ),
    );
  }

  Widget _hero() {
    return Column(
      mainAxisSize: MainAxisSize.min,
      children: [
        IconBadge(icon, tone: tone, shape: badgeShape, pulse: pulse),
        const SizedBox(height: AppSpacing.s9),
        Text(title, style: AppTypography.heading, textAlign: TextAlign.center),
        if (description != null) ...[
          const SizedBox(height: AppSpacing.s5),
          ConstrainedBox(
            constraints: const BoxConstraints(maxWidth: 300),
            child: Text(
              description!,
              style: AppTypography.body.copyWith(color: _descriptionColor),
              textAlign: TextAlign.center,
            ),
          ),
        ],
        if (centerExtra != null) ...[
          const SizedBox(height: AppSpacing.s7),
          centerExtra!,
        ],
      ],
    );
  }
}
