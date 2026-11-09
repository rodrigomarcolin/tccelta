import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/domain/obd2/obd2_pid.dart';
import 'package:tccelta_mobile/src/infra/sync/inject/csv_telemetry_source.dart';
import 'package:tccelta_mobile/src/infra/sync/inject/telemetry_profile.dart';

/// Estado observável da injeção de telemetria de depuração.
class TelemetryInjectionState {
  /// Cria o estado.
  const TelemetryInjectionState({
    this.enabled = false,
    this.profile = TelemetryProfile.economico,
    this.ready = false,
  });

  /// `true` enquanto a injeção está substituindo os PIDs dos blocos.
  final bool enabled;

  /// Perfil de motorista escolhido.
  final TelemetryProfile profile;

  /// `true` quando o CSV do perfil já foi carregado.
  final bool ready;

  /// Cópia com os campos informados trocados.
  TelemetryInjectionState copyWith({
    bool? enabled,
    TelemetryProfile? profile,
    bool? ready,
  }) => TelemetryInjectionState(
    enabled: enabled ?? this.enabled,
    profile: profile ?? this.profile,
    ready: ready ?? this.ready,
  );
}

/// Dirige a injeção: carrega o CSV do perfil escolhido e entrega as amostras
/// ao decorator do canal SYNC. Faz sentido só em build de depuração.
class TelemetryInjectionController extends Notifier<TelemetryInjectionState> {
  CsvTelemetrySource? _source;

  @override
  TelemetryInjectionState build() => const TelemetryInjectionState();

  /// Liga ou desliga a injeção; ao ligar, garante o CSV carregado.
  Future<void> setEnabled({required bool enabled}) async {
    if (enabled) {
      await _load(state.profile);
      state = state.copyWith(enabled: true);
    } else {
      state = state.copyWith(enabled: false);
    }
  }

  /// Troca o perfil; recarrega o CSV se a injeção estiver ligada.
  Future<void> setProfile(TelemetryProfile profile) async {
    if (profile == state.profile) return;
    _source = null;
    state = state.copyWith(profile: profile, ready: false);
    if (state.enabled) await _load(profile);
  }

  Future<void> _load(TelemetryProfile profile) async {
    _source = await CsvTelemetrySource.load(profile);
    state = state.copyWith(ready: true);
  }

  /// Próxima amostra se a injeção está ligada e o CSV já carregou; senão `null`
  /// (o bloco real passa intacto).
  Map<Obd2Pid, double>? takeSample() => state.enabled ? _source?.next() : null;
}

/// Provider do controlador da injeção de telemetria.
final NotifierProvider<TelemetryInjectionController, TelemetryInjectionState>
telemetryInjectionControllerProvider =
    NotifierProvider<TelemetryInjectionController, TelemetryInjectionState>(
      TelemetryInjectionController.new,
    );
