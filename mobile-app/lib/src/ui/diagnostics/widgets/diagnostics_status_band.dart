import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/connection/view_model/connecting_view_model.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/diagnostics/view_model/dtc_view_model.dart';

/// Top bar da aba de Diagnóstico: espelha `TelemetryStatusBand`, mas o slot
/// secundário mostra "Modo 03" (fixo) em vez de versão/protocolo do
/// adaptador — a leitura de DTCs não depende dessa informação.
class DiagnosticsStatusBand extends ConsumerWidget {
  /// Cria a faixa de status da aba de Diagnóstico.
  const DiagnosticsStatusBand({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final device = ref.watch(selectedDongleProvider);
    final phase = ref.watch(connectingViewModelProvider.select((s) => s.phase));
    final dtc = ref.watch(dtcViewModelProvider);

    final status = _statusFor(phase, dtc.failure);

    return StatusBand(
      device: device?.displayName ?? 'OBD2Dongle',
      detail: 'Modo 03',
      statusLabel: status.label,
      statusTone: status.tone,
      pulse: status.tone == StatusTone.live,
    );
  }

  /// Mapeia a fase da conexão (+ falha de leitura) para o rótulo/tom do
  /// badge — mesma régua de `TelemetryStatusBand`.
  static ({String label, StatusTone tone}) _statusFor(
    BleConnectionPhase phase,
    Object? failure,
  ) => switch (phase) {
    BleConnectionPhase.ready when failure != null => (
      label: 'SEM DADOS',
      tone: StatusTone.warning,
    ),
    BleConnectionPhase.ready => (label: 'AO VIVO', tone: StatusTone.live),
    BleConnectionPhase.reconnecting => (
      label: 'RECONECTANDO',
      tone: StatusTone.warning,
    ),
    BleConnectionPhase.connecting ||
    BleConnectionPhase.optimizingLink ||
    BleConnectionPhase.discovering ||
    BleConnectionPhase.enablingNotify => (
      label: 'CONECTANDO',
      tone: StatusTone.warning,
    ),
    BleConnectionPhase.failed || BleConnectionPhase.disconnected => (
      label: 'OFFLINE',
      tone: StatusTone.alert,
    ),
    BleConnectionPhase.idle => (label: 'AGUARDANDO', tone: StatusTone.warning),
  };
}
