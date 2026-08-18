import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/telemetry/indicator_display.dart';
import 'package:tccelta_mobile/src/domain/telemetry/panel.dart';

/// Estado observável do conjunto de painéis do usuário e qual está ativo.
///
/// Expõe `indicatorIds`/`displays`/`indicators`/`contains`/`displayFor` como
/// atalhos que delegam para [active] — quem só precisa ler/exibir o painel
/// corrente (o grid do Painel, o ciclo rápido de polling) não muda nada ao
/// ganhar suporte a múltiplos painéis.
class PanelsState {
  /// Cria o estado dos painéis. [panels] nunca deve ficar vazio — sempre há
  /// ao menos um painel.
  const PanelsState({required this.panels, required this.activeId});

  /// Todos os painéis do usuário, na ordem de criação.
  final List<Panel> panels;

  /// `id` do painel exibido atualmente no Painel.
  final String activeId;

  /// O painel ativo.
  Panel get active => panels.firstWhere((p) => p.id == activeId);

  /// Atalho para `active.indicatorIds`.
  List<String> get indicatorIds => active.indicatorIds;

  /// Atalho para `active.displays`.
  Map<String, IndicatorDisplay> get displays => active.displays;

  /// Atalho para `active.indicators`.
  List<Obd2Pid> get indicators => active.indicators;

  /// Atalho para `active.contains`.
  bool contains(Obd2Pid pid) => active.contains(pid);

  /// Atalho para `active.displayFor`.
  IndicatorDisplay displayFor(Obd2Pid pid) => active.displayFor(pid);

  /// Cópia com os campos sobrescritos.
  PanelsState copyWith({List<Panel>? panels, String? activeId}) => PanelsState(
    panels: panels ?? this.panels,
    activeId: activeId ?? this.activeId,
  );

  /// Serializa para um `Map` codificável em JSON — a peça que deixa o
  /// conjunto de painéis pronto para uma futura persistência local (ainda
  /// fora de escopo).
  Map<String, dynamic> toJson() => {
    'panels': panels.map((p) => p.toJson()).toList(),
    'activeId': activeId,
  };
}

/// ViewModel do gerenciamento de painéis: cria, duplica, renomeia e exclui
/// painéis, além de adicionar/remover/reordenar indicadores **do painel
/// ativo**. Toda a lógica vive aqui; a tela e os sheets só observam e chamam
/// estes métodos.
class PanelViewModel extends Notifier<PanelsState> {
  /// Próximo sufixo de `id` a gerar — começa em 2 porque [build] já usa 1
  /// para o painel inicial. Um contador (em vez de `DateTime.now()`) mantém
  /// os `id`s determinísticos para os testes.
  int _nextSeq = 2;

  String _newId() => 'panel_${_nextSeq++}';

  /// Corta [name] em [PanelNameLimits.max] caracteres — o campo de texto já
  /// impede digitar/colar além disso, mas o sufixo `' (cópia)'` de
  /// [duplicatePanel] pode passar do limite em duplicações repetidas.
  String _clampName(String name) => name.length > PanelNameLimits.max
      ? name.substring(0, PanelNameLimits.max)
      : name;

  @override
  PanelsState build() => const PanelsState(
    panels: [Panel(id: 'panel_1', name: 'Painel 1')],
    activeId: 'panel_1',
  );

  /// Aplica [fn] ao painel ativo, preservando os demais.
  void _updateActive(Panel Function(Panel) fn) {
    state = state.copyWith(
      panels: [
        for (final p in state.panels)
          if (p.id == state.activeId) fn(p) else p,
      ],
    );
  }

  /// Adiciona [pid] ao final do painel ativo com a customização [display].
  /// Sem efeito na ordem se [pid] já estiver presente — mas [display] sempre
  /// sobrescreve (mesmo caminho usado para editar, ver [updateDisplay]).
  void addIndicator(Obd2Pid pid, IndicatorDisplay display) {
    _updateActive((p) {
      final ids = p.contains(pid)
          ? p.indicatorIds
          : [...p.indicatorIds, pid.name];
      return p.copyWith(
        indicatorIds: ids,
        displays: {...p.displays, pid.name: display},
      );
    });
  }

  /// Sobrescreve a customização de exibição de [pid] no painel ativo, sem
  /// alterar sua posição. Sem efeito se [pid] não estiver presente.
  void updateDisplay(Obd2Pid pid, IndicatorDisplay display) {
    if (!state.contains(pid)) return;
    _updateActive(
      (p) => p.copyWith(displays: {...p.displays, pid.name: display}),
    );
  }

  /// Remove [pid] do painel ativo. Sem efeito se não estiver presente.
  void removeIndicator(Obd2Pid pid) {
    if (!state.contains(pid)) return;
    _updateActive((p) {
      final displays = {...p.displays}..remove(pid.name);
      return p.copyWith(
        indicatorIds: p.indicatorIds.where((id) => id != pid.name).toList(),
        displays: displays,
      );
    });
  }

  /// Move o indicador de [oldIndex] para o lugar de [newIndex] no painel
  /// ativo, deslocando os demais — mesma semântica de "arrastar sobre o
  /// alvo" do protótipo (o `dragEnter` do design original faz o mesmo
  /// `splice`/reinserção sobre os índices correntes, sem o ajuste
  /// "pós-remoção" do `ReorderableListView`).
  void reorder(int oldIndex, int newIndex) {
    if (oldIndex == newIndex) return;
    _updateActive((p) {
      final ids = [...p.indicatorIds];
      final id = ids.removeAt(oldIndex);
      ids.insert(newIndex, id);
      return p.copyWith(indicatorIds: ids);
    });
  }

  /// Cria um painel vazio ao final da lista (nome `'Painel N'`) e o torna
  /// ativo.
  void createPanel() {
    final panel = Panel(
      id: _newId(),
      name: 'Painel ${state.panels.length + 1}',
    );
    state = state.copyWith(
      panels: [...state.panels, panel],
      activeId: panel.id,
    );
  }

  /// Duplica o painel [id] (indicadores e customizações inclusos) num novo
  /// painel `'{nome} (cópia)'` ao final da lista, e o torna ativo. Sem
  /// efeito se [id] não existir.
  void duplicatePanel(String id) {
    final matches = state.panels.where((p) => p.id == id);
    if (matches.isEmpty) return;
    final source = matches.first;
    final copy = Panel(
      id: _newId(),
      name: _clampName('${source.name} (cópia)'),
      indicatorIds: [...source.indicatorIds],
      displays: {...source.displays},
    );
    state = state.copyWith(panels: [...state.panels, copy], activeId: copy.id);
  }

  /// Renomeia o painel [id] — [name] é cortado em [PanelNameLimits.max]
  /// caracteres. Sem efeito se [id] não existir.
  void renamePanel(String id, String name) {
    final clamped = _clampName(name);
    state = state.copyWith(
      panels: [
        for (final p in state.panels)
          if (p.id == id) p.copyWith(name: clamped) else p,
      ],
    );
  }

  /// Exclui o painel [id]. Sem efeito se ele não existir ou se for o único
  /// painel restante — nunca fica sem nenhum. Se o excluído era o ativo, o
  /// primeiro painel restante vira o novo ativo.
  void deletePanel(String id) {
    if (state.panels.length < 2) return;
    final panels = state.panels.where((p) => p.id != id).toList();
    if (panels.length == state.panels.length) return;
    state = state.copyWith(
      panels: panels,
      activeId: state.activeId == id ? panels.first.id : state.activeId,
    );
  }

  /// Troca o painel ativo para [id]. Sem efeito se não existir.
  void switchPanel(String id) {
    if (!state.panels.any((p) => p.id == id)) return;
    state = state.copyWith(activeId: id);
  }
}

/// Provider do [PanelViewModel].
///
/// Não é `autoDispose`: a seleção do usuário deve sobreviver a navegações
/// momentâneas para fora do Painel (ex.: abrir "Mais" e voltar) — só é
/// perdida ao reiniciar o app, até a persistência entrar em escopo.
final NotifierProvider<PanelViewModel, PanelsState> panelViewModelProvider =
    NotifierProvider<PanelViewModel, PanelsState>(PanelViewModel.new);
