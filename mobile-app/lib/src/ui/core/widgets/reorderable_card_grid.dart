import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';

/// Grid de [columns] colunas cujos cards podem ser reposicionados por
/// arrastar-e-soltar (segurar e arrastar — [LongPressDraggable], para não
/// competir com o gesto de rolagem vertical da tela que o envolve).
///
/// Generalização de `CardGrid` para conteúdo reordenável: cada célula vira um
/// `DragTarget` que, ao ser sobrevoada por outra sendo arrastada, já chama
/// [onReorder] — os demais cards se deslocam antes de soltar. O card sendo
/// arrastado fica com opacidade reduzida em seu lugar original (via
/// `childWhenDragging`), mas continua sendo reconstruído por [itemBuilder] a
/// cada chamada — os valores ao vivo não param de atualizar durante o
/// arrasto. Um único `Wrap` (não uma grade de `Row`s por linha) mantém todas
/// as células num mesmo pai, para que [keyOf] preserve a identidade de cada
/// item através das reordenações. Cada célula tem no mínimo [minCellHeight]
/// (pode crescer além disso — nunca fica menor).
class ReorderableCardGrid<T extends Object> extends StatelessWidget {
  /// Cria o grid reordenável.
  const ReorderableCardGrid({
    required this.items,
    required this.itemBuilder,
    required this.onReorder,
    required this.keyOf,
    this.columns = 2,
    this.gap = AppSpacing.s3,
    this.minCellHeight = 86,
    super.key,
  });

  /// Itens a exibir, na ordem de exibição.
  final List<T> items;

  /// Constrói o conteúdo do card de `item` (índice `index` na lista atual).
  final Widget Function(BuildContext context, T item, int index) itemBuilder;

  /// Chamado ao vivo (antes de soltar) quando um item passa de `oldIndex`
  /// para `newIndex`.
  final void Function(int oldIndex, int newIndex) onReorder;

  /// Chave estável por item (ex.: um identificador), preservando o estado do
  /// `Draggable` de cada célula entre reordenações.
  final Key Function(T item) keyOf;

  /// Número de colunas. @default 2
  final int columns;

  /// Espaço horizontal e vertical entre as células. @default [AppSpacing.s3]
  final double gap;

  /// Altura mínima de cada célula — o card sempre ocupa ao menos uma posição
  /// inteira do grid, mesmo com conteúdo curto. @default 86
  final double minCellHeight;

  @override
  Widget build(BuildContext context) {
    return LayoutBuilder(
      builder: (context, constraints) {
        final cellWidth =
            (constraints.maxWidth - gap * (columns - 1)) / columns;
        return Wrap(
          spacing: gap,
          runSpacing: gap,
          children: [
            for (var i = 0; i < items.length; i++)
              ConstrainedBox(
                key: keyOf(items[i]),
                constraints: BoxConstraints(
                  minWidth: cellWidth,
                  maxWidth: cellWidth,
                  minHeight: minCellHeight,
                ),
                child: _Cell<T>(
                  item: items[i],
                  index: i,
                  items: items,
                  cellWidth: cellWidth,
                  onReorder: onReorder,
                  child: itemBuilder(context, items[i], i),
                ),
              ),
          ],
        );
      },
    );
  }
}

class _Cell<T extends Object> extends StatefulWidget {
  const _Cell({
    required this.item,
    required this.index,
    required this.items,
    required this.cellWidth,
    required this.onReorder,
    required this.child,
    super.key,
  });

  final T item;
  final int index;
  final List<T> items;
  final double cellWidth;
  final void Function(int oldIndex, int newIndex) onReorder;
  final Widget child;

  @override
  State<_Cell<T>> createState() => _CellState<T>();
}

class _CellState<T extends Object> extends State<_Cell<T>> {
  /// Chave só do card renderizado (não do `feedback`) — usada para medir seu
  /// tamanho real e evitar que o `feedback` (fora da árvore normal, dentro do
  /// `Overlay`) receba constraints diferentes das do grid e estoure.
  final GlobalKey _measureKey = GlobalKey();

  Size? _measuredSize;

  void _measure(Duration _) {
    final box = _measureKey.currentContext?.findRenderObject();
    if (box is RenderBox && box.hasSize) _measuredSize = box.size;
  }

  @override
  Widget build(BuildContext context) {
    WidgetsBinding.instance.addPostFrameCallback(_measure);

    return DragTarget<T>(
      onWillAcceptWithDetails: (details) {
        final from = widget.items.indexOf(details.data);
        if (from != -1 && from != widget.index) {
          widget.onReorder(from, widget.index);
        }
        return true;
      },
      builder: (context, candidateData, rejectedData) {
        return LongPressDraggable<T>(
          data: widget.item,
          dragAnchorStrategy: pointerDragAnchorStrategy,
          feedback: SizedBox(
            width: _measuredSize?.width ?? widget.cellWidth,
            height: _measuredSize?.height,
            child: Opacity(
              opacity: 0.92,
              child: DecoratedBox(
                decoration: const BoxDecoration(
                  borderRadius: AppRadii.brLg,
                  boxShadow: [
                    BoxShadow(
                      color: Color(0x66000000),
                      blurRadius: 18,
                      offset: Offset(0, 8),
                    ),
                  ],
                ),
                child: widget.child,
              ),
            ),
          ),
          childWhenDragging: Opacity(opacity: 0.35, child: widget.child),
          child: KeyedSubtree(key: _measureKey, child: widget.child),
        );
      },
    );
  }
}
