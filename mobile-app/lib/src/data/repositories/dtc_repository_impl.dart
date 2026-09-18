import 'package:tccelta_mobile/src/domain/obd2/dtc_active_entry.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_snapshot.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_status.dart';
import 'package:tccelta_mobile/src/domain/repositories/dtc_repository.dart';

/// Implementação mockada de [DtcRepository]: devolve só os códigos ativos
/// agora (curadoria pt-BR realista, alinhada ao design), sem tocar em BLE.
///
/// **Placeholder para a leitura real.** Quando o datasource ELM327 existir,
/// ele lerá os Modos 03 (confirmados)/07 (pendentes)/0A (permanentes) para os
/// códigos — a troca é só esta classe (e o
/// `dtcRepositoryProvider` em `diagnostics_providers.dart`), sem tocar em
/// domain/view_model/view. Este método **nunca** deve devolver o catálogo
/// inteiro (isso é `dtcCatalog`, dado de domínio) — só o que está de fato
/// ativo, igual a como os Modos 03/07/0A se comportam de verdade.
class FakeDtcRepositoryImpl implements DtcRepository {
  /// Atraso simulado da leitura, para a UI exercitar o estado de carregamento
  /// (inicial e no "Reler") como uma leitura real faria.
  static const Duration _simulatedLatency = Duration(milliseconds: 600);

  @override
  Future<DtcSnapshot> read() async {
    await Future<void>.delayed(_simulatedLatency);
    const active = _active;
    return DtcSnapshot(
      active: active,
      milOn: active.any((a) => a.status == DtcStatus.confirmed),
    );
  }

  static const List<DtcActiveEntry> _active = [
    DtcActiveEntry(
      code: 'P0301',
      status: DtcStatus.confirmed,
      detectedLabel: 'há 2 dias · 3 ciclos',
    ),
    DtcActiveEntry(
      code: 'P0171',
      status: DtcStatus.pending,
      detectedLabel: 'neste ciclo de condução',
    ),
    DtcActiveEntry(
      code: 'P0420',
      status: DtcStatus.confirmed,
      detectedLabel: 'há 11 dias',
    ),
    DtcActiveEntry(
      code: 'U0121',
      status: DtcStatus.confirmed,
      detectedLabel: 'há 4 h',
    ),
    DtcActiveEntry(
      code: 'B1200',
      status: DtcStatus.pending,
      detectedLabel: 'neste ciclo de condução',
    ),
  ];
}
