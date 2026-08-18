import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';

/// Estado observável do conjunto de indicadores exibidos no Painel.
///
/// [indicatorIds] guarda `Obd2Pid.name` (ex.: `"rpm"`) em vez do enum
/// diretamente: uma `List<String>` é trivial de serializar (`["rpm","speed"]`)
/// quando a persistência entrar em escopo. A ordem da lista é a própria ordem
/// de exibição no grid.
class PanelState {
  /// Cria o estado do painel. Começa vazio — o usuário escolhe o que aparece.
  const PanelState({this.indicatorIds = const []});

  /// Identificadores (`Obd2Pid.name`) dos indicadores adicionados, na ordem de
  /// exibição.
  final List<String> indicatorIds;

  /// Os PIDs adicionados, na ordem de exibição.
  List<Obd2Pid> get indicators =>
      indicatorIds.map(Obd2Pid.values.byName).toList(growable: false);

  /// Se [pid] já foi adicionado ao painel.
  bool contains(Obd2Pid pid) => indicatorIds.contains(pid.name);

  /// Cópia com [indicatorIds] sobrescrito.
  PanelState copyWith({List<String>? indicatorIds}) =>
      PanelState(indicatorIds: indicatorIds ?? this.indicatorIds);
}

/// ViewModel do gerenciamento de indicadores do Painel: adiciona, remove e
/// reordena. Toda a lógica vive aqui; a tela e o sheet de sensores só
/// observam e chamam estes métodos.
class PanelViewModel extends Notifier<PanelState> {
  @override
  PanelState build() => const PanelState();

  /// Adiciona [pid] ao final do painel. Sem efeito se já estiver presente.
  void addIndicator(Obd2Pid pid) {
    if (state.contains(pid)) return;
    state = state.copyWith(indicatorIds: [...state.indicatorIds, pid.name]);
  }

  /// Remove [pid] do painel. Sem efeito se não estiver presente.
  void removeIndicator(Obd2Pid pid) {
    if (!state.contains(pid)) return;
    state = state.copyWith(
      indicatorIds: state.indicatorIds.where((id) => id != pid.name).toList(),
    );
  }

  /// Move o indicador de [oldIndex] para o lugar de [newIndex], deslocando os
  /// demais — mesma semântica de "arrastar sobre o alvo" do protótipo (o
  /// `dragEnter` do design original faz o mesmo `splice`/reinserção sobre os
  /// índices correntes, sem o ajuste "pós-remoção" do `ReorderableListView`).
  void reorder(int oldIndex, int newIndex) {
    if (oldIndex == newIndex) return;
    final ids = [...state.indicatorIds];
    final id = ids.removeAt(oldIndex);
    ids.insert(newIndex, id);
    state = state.copyWith(indicatorIds: ids);
  }
}

/// Provider do [PanelViewModel].
///
/// Não é `autoDispose`: a seleção do usuário deve sobreviver a navegações
/// momentâneas para fora do Painel (ex.: abrir "Mais" e voltar) — só é
/// perdida ao reiniciar o app, até a persistência entrar em escopo.
final NotifierProvider<PanelViewModel, PanelState> panelViewModelProvider =
    NotifierProvider<PanelViewModel, PanelState>(PanelViewModel.new);
