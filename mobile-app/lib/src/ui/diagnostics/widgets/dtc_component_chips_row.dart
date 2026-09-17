import 'package:flutter/widgets.dart';
import 'package:tccelta_mobile/src/core/theme/theme.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_component.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';

/// Linha horizontal rolável de chips de filtro por componente: "Todos" +
/// um chip por [DtcComponent], cada um com a contagem de ativos (em destaque
/// vermelho quando > 0) ou o total do catálogo (neutro) quando nenhum está
/// ativo ali.
class DtcComponentChipsRow extends StatelessWidget {
  /// Cria a linha de chips de componente.
  const DtcComponentChipsRow({
    required this.selected,
    required this.onSelect,
    required this.activeCount,
    required this.activeCountFor,
    required this.totalCountFor,
    super.key,
  });

  /// Componente selecionado. `null` = "Todos".
  final DtcComponent? selected;

  /// Chamado com o componente tocado (`null` para "Todos").
  final ValueChanged<DtcComponent?> onSelect;

  /// Total de códigos ativos no catálogo inteiro — a contagem do chip
  /// "Todos".
  final int activeCount;

  /// Nº de códigos ativos de um componente.
  final int Function(DtcComponent component) activeCountFor;

  /// Nº de códigos (ativos ou não) de um componente.
  final int Function(DtcComponent component) totalCountFor;

  static const double _height = 40;

  @override
  Widget build(BuildContext context) {
    return SizedBox(
      height: _height,
      child: ListView.separated(
        scrollDirection: Axis.horizontal,
        itemCount: DtcComponent.values.length + 1,
        separatorBuilder: (context, index) =>
            const SizedBox(width: AppSpacing.s2),
        itemBuilder: (context, index) {
          if (index == 0) {
            return AppChip(
              label: 'Todos',
              trailingCount: '$activeCount',
              trailingCountEmphasis: activeCount > 0,
              selected: selected == null,
              onTap: () => onSelect(null),
            );
          }
          final component = DtcComponent.values[index - 1];
          final active = activeCountFor(component);
          return AppChip(
            label: component.label,
            trailingCount: active > 0
                ? '$active'
                : '${totalCountFor(component)}',
            trailingCountEmphasis: active > 0,
            selected: selected == component,
            onTap: () => onSelect(component),
          );
        },
      ),
    );
  }
}
