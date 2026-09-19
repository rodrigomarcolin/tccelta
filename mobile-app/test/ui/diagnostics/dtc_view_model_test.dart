import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:tccelta_mobile/src/core/errors/dtc_failure.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_active_entry.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_catalog.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_component.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_snapshot.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_status.dart';
import 'package:tccelta_mobile/src/domain/repositories/dtc_repository.dart';
import 'package:tccelta_mobile/src/ui/diagnostics/diagnostics_providers.dart';
import 'package:tccelta_mobile/src/ui/diagnostics/view_model/dtc_view_model.dart';

/// Repository de DTCs fake e determinístico — devolve 2 códigos ativos
/// (P0301, motor, confirmado; C0035, freios, pendente), ambos já presentes
/// no catálogo real ([dtcCatalog]) — o restante do catálogo fica inativo.
class _FakeDtcRepository implements DtcRepository {
  _FakeDtcRepository({this.failure});

  final DtcReadFailure? failure;
  int readCalls = 0;

  static const _default = DtcSnapshot(
    active: [
      DtcActiveEntry(code: 'P0301', status: DtcStatus.confirmed),
      DtcActiveEntry(code: 'C0035', status: DtcStatus.pending),
    ],
    milOn: true,
  );

  @override
  Future<DtcSnapshot> read() async {
    readCalls++;
    final f = failure;
    if (f != null) throw f;
    return _default;
  }
}

void main() {
  late _FakeDtcRepository repo;
  late ProviderContainer container;

  setUp(() {
    repo = _FakeDtcRepository();
    container = ProviderContainer(
      overrides: [dtcRepositoryProvider.overrideWithValue(repo)],
    );
    addTearDown(container.dispose);
  });

  DtcViewModel notifier() => container.read(dtcViewModelProvider.notifier);
  DtcState state() => container.read(dtcViewModelProvider);

  test(
    'estado inicial carrega, depois popula o catálogo inteiro + MIL',
    () async {
      expect(state().isLoading, isTrue);
      expect(state().codes, isEmpty);

      await Future<void>.delayed(const Duration(milliseconds: 10));

      final s = state();
      expect(s.isLoading, isFalse);
      // O catálogo inteiro sempre aparece — só o status de cada um muda.
      expect(s.codes.length, dtcCatalog.length);
      expect(s.milOn, isTrue);
      expect(s.activeCount, 2); // confirmado + pendente
      expect(s.totalCount, dtcCatalog.length);
    },
  );

  test('falha na leitura fica exposta em `failure`', () async {
    final failingRepo = _FakeDtcRepository(
      failure: const DtcReadFailure('sem conexão'),
    );
    final failingContainer = ProviderContainer(
      overrides: [dtcRepositoryProvider.overrideWithValue(failingRepo)],
    );
    addTearDown(failingContainer.dispose);

    failingContainer.read(dtcViewModelProvider); // dispara o build() (lazy).
    await Future<void>.delayed(const Duration(milliseconds: 10));

    final s = failingContainer.read(dtcViewModelProvider);
    expect(s.isLoading, isFalse);
    expect(s.failure, isA<DtcReadFailure>());
  });

  test('reread() chama o repositório de novo', () async {
    state(); // dispara o build() (lazy) e a carga inicial.
    await Future<void>.delayed(const Duration(milliseconds: 10));
    expect(repo.readCalls, 1);

    await notifier().reread();

    expect(repo.readCalls, 2);
  });

  test(
    'setComponentFilter restringe activeCodes/groupedCodes ao componente',
    () async {
      state(); // dispara o build() (lazy) e a carga inicial.
      await Future<void>.delayed(const Duration(milliseconds: 10));

      notifier().setComponentFilter(DtcComponent.engine);

      expect(state().activeCodes.map((c) => c.code).toList(), ['P0301']);
      expect(state().groupedCodes.keys.toList(), [DtcComponent.engine]);
      // Contagens "globais" (badge/card) ignoram o filtro.
      expect(state().activeCount, 2);

      notifier().setComponentFilter(null);
      expect(state().groupedCodes.keys.length, DtcComponent.values.length);
    },
  );

  test('setViewMode alterna o modo de vista', () async {
    expect(state().viewMode, DtcViewMode.active);

    notifier().setViewMode(DtcViewMode.byComponent);

    expect(state().viewMode, DtcViewMode.byComponent);
  });
}
