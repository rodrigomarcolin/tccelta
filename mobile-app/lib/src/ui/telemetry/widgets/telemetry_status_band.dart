import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/core/errors/failure.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/connection/view_model/connecting_view_model.dart';
import 'package:tccelta_mobile/src/ui/core/widgets/widgets.dart';
import 'package:tccelta_mobile/src/ui/telemetry/view_model/telemetry_view_model.dart';

/// Top bar do painel ligada ao estado vivo: alimenta o átomo [StatusBand] com o
/// nome real do dongle, a versão + protocolo do adaptador e o status da
/// conexão. Toda a lógica de mapeamento vive aqui; o átomo só renderiza.
class TelemetryStatusBand extends ConsumerWidget {
  /// Cria a faixa de status do painel.
  const TelemetryStatusBand({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final device = ref.watch(selectedDongleProvider);
    final phase = ref.watch(connectingViewModelProvider.select((s) => s.phase));
    final telemetry = ref.watch(telemetryViewModelProvider);

    final status = _statusFor(phase, telemetry.failure);
    final info = telemetry.adapterInfo;
    // Junta versão e protocolo (só os presentes) no slot secundário.
    final detail = [info?.version, info?.protocol]
        .whereType<String>()
        .where((s) => s.isNotEmpty)
        .join(' · ');

    return StatusBand(
      device: device?.displayName ?? 'OBD2Dongle',
      detail: detail.isEmpty ? null : detail,
      statusLabel: status.label,
      statusTone: status.tone,
      pulse: status.tone == StatusTone.live,
    );
  }

  /// Mapeia a fase da conexão (+ falha de leitura) para o rótulo/tom do badge.
  static ({String label, StatusTone tone}) _statusFor(
    BleConnectionPhase phase,
    Failure? failure,
  ) =>
      switch (phase) {
        BleConnectionPhase.ready when failure != null =>
          (label: 'SEM DADOS', tone: StatusTone.warning),
        BleConnectionPhase.ready => (label: 'AO VIVO', tone: StatusTone.live),
        BleConnectionPhase.reconnecting =>
          (label: 'RECONECTANDO', tone: StatusTone.warning),
        BleConnectionPhase.connecting ||
        BleConnectionPhase.optimizingLink ||
        BleConnectionPhase.discovering ||
        BleConnectionPhase.enablingNotify =>
          (label: 'CONECTANDO', tone: StatusTone.warning),
        BleConnectionPhase.failed || BleConnectionPhase.disconnected =>
          (label: 'OFFLINE', tone: StatusTone.alert),
        BleConnectionPhase.idle =>
          (label: 'AGUARDANDO', tone: StatusTone.warning),
      };
}
