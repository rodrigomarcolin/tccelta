import 'package:flutter/widgets.dart';

/// Indicador visual de "isto pode ser arrastado": 4 pontinhos em grade 2×2,
/// esmaecidos — o mesmo idioma do protótipo original do design system.
///
/// Agnóstico de onde é usado: quem quiser sinalizar um item arrastável
/// (ex.: `StatCard.cornerAccessory` no canto superior direito) compõe este
/// átomo em vez de redesenhar os pontinhos.
class DragHandleDots extends StatelessWidget {
  /// Cria o indicador.
  const DragHandleDots({super.key});

  static const double _dotSize = 3;
  static const double _gap = 2;
  static const Color _dotColor = Color(0xFFFFFFFF);

  @override
  Widget build(BuildContext context) {
    const dot = DecoratedBox(
      decoration: BoxDecoration(shape: BoxShape.circle, color: _dotColor),
      child: SizedBox(width: _dotSize, height: _dotSize),
    );
    const row = Row(
      mainAxisSize: MainAxisSize.min,
      children: [
        dot,
        SizedBox(width: _gap),
        dot,
      ],
    );
    return const Opacity(
      opacity: 0.4,
      child: Column(
        mainAxisSize: MainAxisSize.min,
        children: [
          row,
          SizedBox(height: _gap),
          row,
        ],
      ),
    );
  }
}
