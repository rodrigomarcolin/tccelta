import 'package:tccelta_mobile/src/core/errors/failure.dart';
import 'package:tccelta_mobile/src/core/errors/panel_failure.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/domain/repositories/panel_repository.dart';
import 'package:tccelta_mobile/src/domain/telemetry/panel.dart';
import 'package:tccelta_mobile/src/domain/telemetry/panel_collection.dart';

/// Orquestra o carregamento/gravação dos painéis do usuário sobre um
/// [PanelRepository], reconciliando dado persistido possivelmente
/// obsoleto/corrompido com o catálogo de PIDs atual — a regra de negócio que
/// justifica esta camada (ao contrário de um passthrough): nunca deixar o
/// app quebrar ou ficar sem nenhum painel por causa de um dado salvo por uma
/// versão antiga.
class PanelPersistenceUseCase {
  /// Cria o use case sobre um [PanelRepository].
  const PanelPersistenceUseCase(this._repo);

  final PanelRepository _repo;

  /// Painel inicial usado na primeira execução e como fallback seguro
  /// sempre que o dado salvo não puder ser usado.
  static const PanelsState _seed = PanelsState(
    panels: [Panel(id: 'panel_1', name: 'Painel 1')],
    activeId: 'panel_1',
  );

  /// Carrega os painéis salvos. Primeira execução, dado corrompido
  /// ([PanelStorageFailure] via [Failure]) ou lista de painéis vazia sempre
  /// caem no mesmo painel inicial de hoje — nunca propaga erro para a UI.
  Future<PanelsState> load() async {
    try {
      final loaded = await _repo.load();
      if (loaded == null || loaded.panels.isEmpty) return _seed;
      return _sanitize(loaded);
    } on Failure {
      return _seed;
    }
  }

  /// Persiste [state]. Regra de negócio real fica em [load]; aqui é
  /// intencionalmente fino.
  Future<void> save(PanelsState state) => _repo.save(state);

  /// Descarta `indicatorIds`/`displays` que referenciam um [Obd2Pid] que não
  /// existe mais (ex.: PID removido/renomeado numa atualização do app) —
  /// sem isso, `Panel.indicators` (`Obd2Pid.values.byName`) lançaria ao ler
  /// um painel salvo por uma versão antiga. Também garante que `activeId`
  /// aponta para um painel que de fato existe.
  PanelsState _sanitize(PanelsState loaded) {
    final validPidNames = Obd2Pid.values.map((p) => p.name).toSet();
    final panels = loaded.panels.map((panel) {
      final ids = panel.indicatorIds
          .where(validPidNames.contains)
          .toList(growable: false);
      final displays = Map.fromEntries(
        panel.displays.entries.where((e) => validPidNames.contains(e.key)),
      );
      return panel.copyWith(indicatorIds: ids, displays: displays);
    }).toList(growable: false);

    final activeId = panels.any((p) => p.id == loaded.activeId)
        ? loaded.activeId
        : panels.first.id;

    return PanelsState(panels: panels, activeId: activeId);
  }
}
