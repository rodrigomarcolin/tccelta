import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/core/errors/failure.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_catalog.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_code.dart';
import 'package:tccelta_mobile/src/domain/obd2/dtc_component.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';

/// Vista escolhida na aba de Diagnóstico.
enum DtcViewMode {
  /// Só os códigos ativos agora (confirmados/pendentes).
  active,

  /// Catálogo inteiro, agrupado por componente.
  byComponent,
}

/// Estado observável da aba de Diagnóstico.
class DtcState {
  /// Cria o estado da aba de Diagnóstico.
  const DtcState({
    this.isLoading = true,
    this.codes = const [],
    this.milOn = false,
    this.failure,
    this.componentFilter,
    this.viewMode = DtcViewMode.active,
  });

  /// Se uma leitura (inicial ou "Reler") está em andamento.
  final bool isLoading;

  /// Catálogo de códigos da última leitura (ativos e inativos).
  final List<DtcCode> codes;

  /// Estado da luz de injeção (MIL) na última leitura.
  final bool milOn;

  /// Falha da última leitura, se houver.
  final Failure? failure;

  /// Componente selecionado nos chips de filtro. `null` = "Todos".
  final DtcComponent? componentFilter;

  /// Vista escolhida no alternador (Ativos / Todos por componente).
  final DtcViewMode viewMode;

  /// Cópia com os campos sobrescritos. Use [clearFailure] para zerar a falha.
  DtcState copyWith({
    bool? isLoading,
    List<DtcCode>? codes,
    bool? milOn,
    Failure? failure,
    bool clearFailure = false,
    DtcComponent? componentFilter,
    bool clearComponentFilter = false,
    DtcViewMode? viewMode,
  }) => DtcState(
    isLoading: isLoading ?? this.isLoading,
    codes: codes ?? this.codes,
    milOn: milOn ?? this.milOn,
    failure: clearFailure ? null : (failure ?? this.failure),
    componentFilter: clearComponentFilter
        ? null
        : (componentFilter ?? this.componentFilter),
    viewMode: viewMode ?? this.viewMode,
  );

  List<DtcCode> get _filtered {
    final filter = componentFilter;
    if (filter == null) return codes;
    return codes.where((c) => c.component == filter).toList(growable: false);
  }

  /// Códigos ativos (confirmados/pendentes), respeitando [componentFilter].
  List<DtcCode> get activeCodes =>
      _filtered.where((c) => c.isActive).toList(growable: false);

  /// Total de códigos ativos no catálogo inteiro (ignora [componentFilter] —
  /// é a contagem "de N" do card e do badge da tab bar).
  int get activeCount => codes.where((c) => c.isActive).length;

  /// Total de códigos no catálogo (ignora [componentFilter]).
  int get totalCount => codes.length;

  /// Códigos agrupados por componente, respeitando [componentFilter] — a
  /// fonte da vista "Todos por componente".
  Map<DtcComponent, List<DtcCode>> get groupedCodes => {
    for (final component in DtcComponent.values)
      if (componentFilter == null || componentFilter == component)
        component: codes
            .where((c) => c.component == component)
            .toList(growable: false),
  };

  /// Nº de códigos ativos por componente — alimenta a contagem em destaque
  /// dos chips de filtro.
  int activeCountFor(DtcComponent component) =>
      codes.where((c) => c.component == component && c.isActive).length;

  /// Nº de códigos (ativos ou não) por componente — usado nos chips quando
  /// nenhum está ativo naquele componente.
  int totalCountFor(DtcComponent component) =>
      codes.where((c) => c.component == component).length;
}

/// ViewModel da aba de Diagnóstico: lê o retrato de DTCs uma vez ao montar e
/// de novo sob demanda (botão "Reler"); mantém o filtro de componente e a
/// vista escolhida. Não guarda "qual código está com o sheet aberto" — o
/// `DtcCode` tocado é passado direto pro sheet, sem round-trip pelo estado.
///
/// Não `autoDispose`: o retrato sobrevive a idas-e-vindas de aba (mesmo
/// espírito do `PanelViewModel`) — só o "Reler" força uma nova leitura.
class DtcViewModel extends Notifier<DtcState> {
  late final Obd2Repository _repo;

  @override
  DtcState build() {
    _repo = ref.read(obd2RepositoryProvider);
    unawaited(_load());
    return const DtcState();
  }

  /// Nunca toca em `state` antes do primeiro `await` — `build()` chama isto
  /// via `unawaited` (a carga inicial), e escrever em `state` antes do
  /// primeiro `await` rodaria de forma síncrona, ainda dentro de `build()`
  /// (mesmo cuidado de `TelemetryViewModel._start`).
  Future<void> _load() async {
    try {
      final snapshot = await _repo.readDtc();
      final activeByCode = {for (final a in snapshot.active) a.code: a};
      final codes = [
        for (final definition in dtcCatalog)
          DtcCode.fromDefinition(
            definition,
            active: activeByCode[definition.code],
          ),
      ];
      state = state.copyWith(
        isLoading: false,
        codes: codes,
        milOn: snapshot.milOn,
        clearFailure: true,
      );
    } on Failure catch (f) {
      state = state.copyWith(isLoading: false, failure: f);
    }
  }

  /// Repete a leitura (botão "Reler"). Diferente de [_load] (chamado direto
  /// por `build()`), aqui é seguro marcar `isLoading` antes de ler, pois só é
  /// chamado depois que o provider já foi inicializado.
  Future<void> reread() async {
    state = state.copyWith(isLoading: true, clearFailure: true);
    await _load();
  }

  /// Troca a vista (Ativos / Todos por componente).
  void setViewMode(DtcViewMode mode) => state = state.copyWith(viewMode: mode);

  /// Troca o filtro de componente. `null` = "Todos".
  void setComponentFilter(DtcComponent? component) {
    state = component == null
        ? state.copyWith(clearComponentFilter: true)
        : state.copyWith(componentFilter: component);
  }
}

/// Provider do [DtcViewModel].
final NotifierProvider<DtcViewModel, DtcState> dtcViewModelProvider =
    NotifierProvider<DtcViewModel, DtcState>(DtcViewModel.new);
