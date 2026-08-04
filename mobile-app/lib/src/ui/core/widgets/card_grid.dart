import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';

/// Grid genérico de cards em [columns] colunas de largura igual.
///
/// Agnóstico de domínio: recebe uma lista de [children] e os distribui em
/// linhas de `Expanded` separadas por [gap] (o idioma `Row`+`Expanded` já usado
/// nas telas), preenchendo os vãos da última linha para manter todas as colunas
/// com a mesma largura. Evita o paredão de `Row`/`Expanded` inline.
class CardGrid extends StatelessWidget {
  /// Cria o grid com os [children], [columns] colunas e o [gap] entre células.
  const CardGrid({
    required this.children,
    this.columns = 2,
    this.gap = AppSpacing.s3,
    super.key,
  });

  /// Cards a distribuir na grade.
  final List<Widget> children;

  /// Número de colunas. @default 2
  final int columns;

  /// Espaço horizontal e vertical entre as células. @default AppSpacing.s3
  final double gap;

  @override
  Widget build(BuildContext context) {
    final rows = <Widget>[];
    for (var i = 0; i < children.length; i += columns) {
      final cells = <Widget>[];
      for (var c = 0; c < columns; c++) {
        if (c > 0) cells.add(SizedBox(width: gap));
        final index = i + c;
        cells.add(
          Expanded(
            child: index < children.length
                ? children[index]
                : const SizedBox.shrink(),
          ),
        );
      }
      if (rows.isNotEmpty) rows.add(SizedBox(height: gap));
      // Alinhamento ao topo (não `stretch`): num scroll view a altura é
      // ilimitada e `stretch` forçaria altura infinita.
      rows.add(
        Row(crossAxisAlignment: CrossAxisAlignment.start, children: cells),
      );
    }
    return Column(
      crossAxisAlignment: CrossAxisAlignment.stretch,
      mainAxisSize: MainAxisSize.min,
      children: rows,
    );
  }
}
