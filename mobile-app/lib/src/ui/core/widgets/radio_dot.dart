import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';

/// Indicador de seleção "rádio": um círculo vazio, ou preenchido em ciano
/// quando [selected]. Puramente visual — o toque é tratado pelo widget que o
/// contém (ex.: um `CardButton` usado como linha de seleção).
class RadioDot extends StatelessWidget {
  /// Cria o indicador, preenchido conforme [selected].
  const RadioDot({required this.selected, super.key});

  /// Se este é o item atualmente selecionado.
  final bool selected;

  @override
  Widget build(BuildContext context) {
    return Container(
      width: 18,
      height: 18,
      alignment: Alignment.center,
      decoration: BoxDecoration(
        shape: BoxShape.circle,
        border: Border.all(
          color: selected ? AppColors.cyan500 : AppColors.borderStrong,
          width: 2,
        ),
      ),
      child: selected
          ? Container(
              width: 8,
              height: 8,
              decoration: const BoxDecoration(
                shape: BoxShape.circle,
                color: AppColors.cyan500,
              ),
            )
          : null,
    );
  }
}
