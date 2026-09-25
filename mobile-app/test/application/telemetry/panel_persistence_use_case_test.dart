import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/application/telemetry/panel_persistence_use_case.dart';
import 'package:tccelta_mobile/src/domain/telemetry/panel.dart';
import 'package:tccelta_mobile/src/domain/telemetry/panel_collection.dart';

import '../../support/fake_panel_repository.dart';

void main() {
  group('load', () {
    test('primeira execução (nada salvo) cai no painel padrão', () async {
      final useCase = PanelPersistenceUseCase(FakePanelRepository());

      final state = await useCase.load();

      expect(state.panels, hasLength(1));
      expect(state.active.name, 'Painel 1');
    });

    test('lista de painéis vazia persistida cai no painel padrão', () async {
      final repo = FakePanelRepository(
        initial: const PanelsState(panels: [], activeId: ''),
      );
      final useCase = PanelPersistenceUseCase(repo);

      final state = await useCase.load();

      expect(state.panels, hasLength(1));
    });

    test(
      'estado salvo válido é devolvido sem alterações relevantes',
      () async {
        const saved = PanelsState(
          panels: [
            Panel(id: 'p1', name: 'Diário'),
            Panel(id: 'p2', name: 'Pista'),
          ],
          activeId: 'p2',
        );
        final useCase = PanelPersistenceUseCase(
          FakePanelRepository(initial: saved),
        );

        final state = await useCase.load();

        expect(state.panels.map((p) => p.name), ['Diário', 'Pista']);
        expect(state.activeId, 'p2');
      },
    );

    test(
      'indicatorIds referenciando um Obd2Pid inexistente são descartados',
      () async {
        const saved = PanelsState(
          panels: [
            Panel(
              id: 'p1',
              name: 'Diário',
              indicatorIds: ['rpm', 'pid-removido-numa-versao-antiga'],
            ),
          ],
          activeId: 'p1',
        );
        final useCase = PanelPersistenceUseCase(
          FakePanelRepository(initial: saved),
        );

        final state = await useCase.load();

        expect(state.active.indicatorIds, ['rpm']);
      },
    );

    test('activeId órfão é corrigido para o primeiro painel', () async {
      const saved = PanelsState(
        panels: [Panel(id: 'p1', name: 'Diário')],
        activeId: 'painel-que-nao-existe-mais',
      );
      final useCase = PanelPersistenceUseCase(
        FakePanelRepository(initial: saved),
      );

      final state = await useCase.load();

      expect(state.activeId, 'p1');
    });
  });

  group('save', () {
    test('repassa o estado ao repository', () async {
      final repo = FakePanelRepository();
      final useCase = PanelPersistenceUseCase(repo);
      const state = PanelsState(
        panels: [Panel(id: 'p1', name: 'Diário')],
        activeId: 'p1',
      );

      await useCase.save(state);

      expect(repo.lastSaved, state);
    });
  });
}
