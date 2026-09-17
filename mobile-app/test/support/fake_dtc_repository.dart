import 'package:tccelta_mobile/src/domain/obd2/dtc_snapshot.dart';
import 'package:tccelta_mobile/src/domain/repositories/dtc_repository.dart';

/// [DtcRepository] fake e instantâneo (sem `Future.delayed`/`Timer` real) —
/// para telas que só montam a `AppTabBar` (e por isso disparam
/// `dtcViewModelProvider` de qualquer jeito) mas não testam a aba de
/// Diagnóstico em si. Devolve um catálogo vazio por padrão.
class FakeDtcRepository implements DtcRepository {
  FakeDtcRepository({
    this.snapshot = const DtcSnapshot(codes: [], milOn: false),
  });

  final DtcSnapshot snapshot;

  @override
  Future<DtcSnapshot> read() async => snapshot;
}
