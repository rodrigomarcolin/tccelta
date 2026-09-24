import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/application/telemetry/panel_persistence_use_case.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/telemetry/indicator_display.dart';
import 'package:tccelta_mobile/src/domain/telemetry/panel.dart';
import 'package:tccelta_mobile/src/domain/telemetry/panel_collection.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';

/// Reconhece o sufixo numérico de um id `panel_<n>` (o formato gerado ao
/// criar um painel).
final RegExp _panelIdSeq = RegExp(r'^panel_(\d+)$');

/// ViewModel do gerenciamento de painéis: cria, duplica, renomeia e exclui
/// painéis, além de adicionar/remover/reordenar indicadores **do painel
/// ativo**. Toda a lógica vive aqui; a tela e os sheets só observam e chamam
/// estes métodos.
///
/// `AsyncNotifier`: `build()` carrega os painéis salvos via
/// [PanelPersistenceUseCase] (mesmo idioma de `PskNotifier`/
/// `SecurityModeNotifier` em `settings_providers.dart`); cada mutação grava
/// o novo estado como `AsyncData` e dispara a persistência em
/// fire-and-forget — falha ao salvar não deve travar a edição do painel.
class PanelViewModel extends AsyncNotifier<PanelsState> {
  late final PanelPersistenceUseCase _persistence;

  /// Próximo sufixo de `id` a gerar — recalculado em [build] a partir dos
  /// ids restaurados, para não colidir com painéis já persistidos.
  int _nextSeq = 2;

  String _newId() => 'panel_${_nextSeq++}';

  /// Corta [name] em [PanelNameLimits.max] caracteres — o campo de texto já
  /// impede digitar/colar além disso, mas o sufixo `' (cópia)'` de
  /// [duplicatePanel] pode passar do limite em duplicações repetidas.
  String _clampName(String name) => name.length > PanelNameLimits.max
      ? name.substring(0, PanelNameLimits.max)
      : name;

  @override
  Future<PanelsState> build() async {
    _persistence = ref.read(panelPersistenceUseCaseProvider);
    final loaded = await _persistence.load();
    _nextSeq = _seqAfter(loaded.panels);
    return loaded;
  }

  /// Maior sufixo `panel_<n>` visto em [panels], mais um — ponto de partida
  /// seguro para novos ids depois de restaurar painéis salvos.
  static int _seqAfter(List<Panel> panels) {
    var maxSeq = 1;
    for (final p in panels) {
      final match = _panelIdSeq.firstMatch(p.id);
      if (match == null) continue;
      final n = int.tryParse(match.group(1)!);
      if (n != null && n > maxSeq) maxSeq = n;
    }
    return maxSeq + 1;
  }

  /// Aplica [transform] ao estado atual (no-op se ainda não houver estado
  /// carregado), grava o resultado como `AsyncData` e persiste em
  /// fire-and-forget.
  void _apply(PanelsState Function(PanelsState current) transform) {
    final current = state.value;
    if (current == null) return;
    final next = transform(current);
    state = AsyncData(next);
    unawaited(_persistence.save(next));
  }

  /// [current] com [fn] aplicado ao painel [id], preservando os demais. Sem
  /// efeito se [id] não existir.
  PanelsState _updatePanelIn(
    PanelsState current,
    String id,
    Panel Function(Panel) fn,
  ) => current.copyWith(
    panels: [
      for (final p in current.panels)
        if (p.id == id) fn(p) else p,
    ],
  );

  /// Adiciona [pid] ao final do painel ativo com a customização [display].
  /// Ver [addIndicatorTo].
  void addIndicator(Obd2Pid pid, IndicatorDisplay display) {
    final activeId = state.value?.activeId;
    if (activeId == null) return;
    addIndicatorTo(activeId, pid, display);
  }

  /// Adiciona [pid] ao final do painel [panelId] com a customização
  /// [display]. Sem efeito na ordem se [pid] já estiver presente nesse
  /// painel — mas [display] sempre sobrescreve (mesmo caminho usado para
  /// editar, ver [updateDisplayIn]). Sem efeito se [panelId] não existir.
  void addIndicatorTo(String panelId, Obd2Pid pid, IndicatorDisplay display) {
    _apply(
      (current) => _updatePanelIn(current, panelId, (p) {
        final ids = p.contains(pid)
            ? p.indicatorIds
            : [...p.indicatorIds, pid.name];
        return p.copyWith(
          indicatorIds: ids,
          displays: {...p.displays, pid.name: display},
        );
      }),
    );
  }

  /// Sobrescreve a customização de exibição de [pid] no painel ativo. Ver
  /// [updateDisplayIn].
  void updateDisplay(Obd2Pid pid, IndicatorDisplay display) {
    final activeId = state.value?.activeId;
    if (activeId == null) return;
    updateDisplayIn(activeId, pid, display);
  }

  /// Sobrescreve a customização de exibição de [pid] no painel [panelId],
  /// sem alterar sua posição. Sem efeito se [pid] não estiver presente nesse
  /// painel, ou se [panelId] não existir.
  void updateDisplayIn(String panelId, Obd2Pid pid, IndicatorDisplay display) {
    final current = state.value;
    if (current == null) return;
    final matches = current.panels.where((p) => p.id == panelId);
    if (matches.isEmpty || !matches.first.contains(pid)) return;
    _apply(
      (c) => _updatePanelIn(
        c,
        panelId,
        (p) => p.copyWith(displays: {...p.displays, pid.name: display}),
      ),
    );
  }

  /// Remove [pid] do painel ativo. Ver [removeIndicatorFrom].
  void removeIndicator(Obd2Pid pid) {
    final activeId = state.value?.activeId;
    if (activeId == null) return;
    removeIndicatorFrom(activeId, pid);
  }

  /// Remove [pid] do painel [panelId]. Sem efeito se não estiver presente
  /// nesse painel, ou se [panelId] não existir.
  void removeIndicatorFrom(String panelId, Obd2Pid pid) {
    final current = state.value;
    if (current == null) return;
    final matches = current.panels.where((p) => p.id == panelId);
    if (matches.isEmpty || !matches.first.contains(pid)) return;
    _apply(
      (c) => _updatePanelIn(c, panelId, (p) {
        final displays = {...p.displays}..remove(pid.name);
        return p.copyWith(
          indicatorIds: p.indicatorIds.where((id) => id != pid.name).toList(),
          displays: displays,
        );
      }),
    );
  }

  /// Move o indicador de [oldIndex] para o lugar de [newIndex] no painel
  /// ativo, deslocando os demais — mesma semântica de "arrastar sobre o
  /// alvo" do protótipo (o `dragEnter` do design original faz o mesmo
  /// `splice`/reinserção sobre os índices correntes, sem o ajuste
  /// "pós-remoção" do `ReorderableListView`).
  void reorder(int oldIndex, int newIndex) {
    if (oldIndex == newIndex) return;
    _apply(
      (current) => _updatePanelIn(current, current.activeId, (p) {
        final ids = [...p.indicatorIds];
        final id = ids.removeAt(oldIndex);
        ids.insert(newIndex, id);
        return p.copyWith(indicatorIds: ids);
      }),
    );
  }

  /// Cria um painel vazio ao final da lista (nome `'Painel N'`) e o torna
  /// ativo.
  void createPanel() {
    _apply((current) {
      final panel = Panel(
        id: _newId(),
        name: 'Painel ${current.panels.length + 1}',
      );
      return current.copyWith(
        panels: [...current.panels, panel],
        activeId: panel.id,
      );
    });
  }

  /// Duplica o painel [id] (indicadores e customizações inclusos) num novo
  /// painel `'{nome} (cópia)'` ao final da lista, e o torna ativo. Sem
  /// efeito se [id] não existir.
  void duplicatePanel(String id) {
    final current = state.value;
    if (current == null) return;
    final matches = current.panels.where((p) => p.id == id);
    if (matches.isEmpty) return;
    final source = matches.first;
    final copy = Panel(
      id: _newId(),
      name: _clampName('${source.name} (cópia)'),
      indicatorIds: [...source.indicatorIds],
      displays: {...source.displays},
    );
    _apply(
      (c) => c.copyWith(panels: [...c.panels, copy], activeId: copy.id),
    );
  }

  /// Renomeia o painel [id] — [name] é cortado em [PanelNameLimits.max]
  /// caracteres. Sem efeito se [id] não existir.
  void renamePanel(String id, String name) {
    final clamped = _clampName(name);
    _apply(
      (current) => current.copyWith(
        panels: [
          for (final p in current.panels)
            if (p.id == id) p.copyWith(name: clamped) else p,
        ],
      ),
    );
  }

  /// Exclui o painel [id]. Sem efeito se ele não existir ou se for o único
  /// painel restante — nunca fica sem nenhum painel. Se o excluído era o
  /// ativo, o primeiro painel restante vira o novo ativo.
  void deletePanel(String id) {
    final current = state.value;
    if (current == null || current.panels.length < 2) return;
    final panels = current.panels.where((p) => p.id != id).toList();
    if (panels.length == current.panels.length) return;
    _apply(
      (c) => c.copyWith(
        panels: panels,
        activeId: c.activeId == id ? panels.first.id : c.activeId,
      ),
    );
  }

  /// Troca o painel ativo para [id]. Sem efeito se não existir.
  void switchPanel(String id) {
    final current = state.value;
    if (current == null || !current.panels.any((p) => p.id == id)) return;
    _apply((c) => c.copyWith(activeId: id));
  }
}

/// Provider do [PanelViewModel].
///
/// Não é `autoDispose`: a seleção do usuário deve sobreviver a navegações
/// momentâneas para fora do Painel (ex.: abrir "Mais" e voltar), e o estado
/// persistido é o mesmo entre reentradas — só é recarregado ao reiniciar o
/// app.
final AsyncNotifierProvider<PanelViewModel, PanelsState>
panelViewModelProvider =
    AsyncNotifierProvider<PanelViewModel, PanelsState>(PanelViewModel.new);
