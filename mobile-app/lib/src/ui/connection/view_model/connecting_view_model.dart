import 'dart:async';

import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:tccelta_mobile/src/application/connection/prepare_secure_connection_use_case.dart';
import 'package:tccelta_mobile/src/core/errors/failure.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_connection.dart';
import 'package:tccelta_mobile/src/domain/ble/ble_link_error.dart';
import 'package:tccelta_mobile/src/domain/ble/dongle_pairing_hint.dart';
import 'package:tccelta_mobile/src/domain/ble/last_dongle.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/last_dongle_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/obd2_repository.dart';
import 'package:tccelta_mobile/src/ui/connection/connection_providers.dart';
import 'package:tccelta_mobile/src/ui/telemetry/telemetry_providers.dart';

/// Sub-fase da preparação OBD-II, que roda DEPOIS de o BLE ficar `ready`: init
/// do ELM327 e leitura das capacidades (PIDs suportados).
enum ConnectingPrep {
  /// Ainda não começou (BLE ainda não está `ready`).
  idle,

  /// Init do ELM327 em andamento (reset + echo off).
  preparing,

  /// Lendo as capacidades do veículo (bitmask de PIDs suportados).
  reading,

  /// Preparação concluída — pode seguir para "Conectado" (com os dados já
  /// preenchidos). Terminal também em caso de falha best-effort.
  done,
}

/// Etapa da segurança do enlace (pareamento LESC Just Works conduzido pelo SO).
enum LinkSecurityStep {
  /// Nada em andamento.
  idle,

  /// Decidindo o plano (restaurar, parear agora ou esperar o botão).
  planning,

  /// Esperando o usuário segurar o botão do dongle (janela fechada).
  waitingButton,

  /// Conectando e pareando (o SO pode mostrar o pedido de pareamento).
  pairing,

  /// Conectando com um bond existente (sem diálogo).
  restoring,

  /// Sessão cifrada estabelecida.
  secured,

  /// O pareamento foi recusado com a janela ainda aberta.
  rejected,

  /// A janela de pareamento do dongle fechou antes do pareamento.
  windowExpired,

  /// Removendo o bond antigo do SO (Android) para parear de novo.
  removingBond,

  /// iOS guarda um pareamento que o dongle não reconhece: esquecer nos Ajustes.
  forgetOnIos,

  /// Falha sem tratamento específico.
  failed,
}

/// Estado do handshake: a fase BLE + a segurança do enlace + a sub-fase de
/// preparação OBD-II. Imutável, estilo `ScanState`.
class ConnectingState {
  /// Cria o estado.
  const ConnectingState({
    this.phase = BleConnectionPhase.idle,
    this.prep = ConnectingPrep.idle,
    this.security = LinkSecurityStep.idle,
    this.linkError,
    this.dongleSeen = false,
    this.watching = false,
    this.canManageBonds = true,
  });

  /// Fase do transporte BLE.
  final BleConnectionPhase phase;

  /// Sub-fase da preparação OBD-II (só avança após `phase == ready`).
  final ConnectingPrep prep;

  /// Etapa da segurança do enlace.
  final LinkSecurityStep security;

  /// Última falha classificada do enlace.
  final BleLinkError? linkError;

  /// Em `waitingButton`: o anúncio do dongle está chegando.
  final bool dongleSeen;

  /// Em `waitingButton`: a busca pelo anúncio está ativa (falso após o prazo).
  final bool watching;

  /// O SO deixa o app gerenciar bonds (Android).
  final bool canManageBonds;

  /// Cópia com campos sobrescritos. [clearError] zera o [linkError].
  ConnectingState copyWith({
    BleConnectionPhase? phase,
    ConnectingPrep? prep,
    LinkSecurityStep? security,
    BleLinkError? linkError,
    bool clearError = false,
    bool? dongleSeen,
    bool? watching,
    bool? canManageBonds,
  }) => ConnectingState(
    phase: phase ?? this.phase,
    prep: prep ?? this.prep,
    security: security ?? this.security,
    linkError: clearError ? null : (linkError ?? this.linkError),
    dongleSeen: dongleSeen ?? this.dongleSeen,
    watching: watching ?? this.watching,
    canManageBonds: canManageBonds ?? this.canManageBonds,
  );
}

/// ViewModel do handshake de conexão: decide o plano de conexão segura,
/// acompanha o pareamento conduzido pelo SO, reflete as fases BLE e, ao ficar
/// `ready`, encadeia a preparação OBD-II (init do ELM327 + capacidades). Só
/// sinaliza "pronto para seguir" quando `prep == done`.
class ConnectingViewModel extends Notifier<ConnectingState> {
  /// Quanto tempo esperar a janela de pareamento abrir antes de oferecer
  /// "Tentar de novo".
  static const Duration hintTimeout = Duration(seconds: 120);

  /// Quanto tempo olhar o anúncio para distinguir recusa de janela fechada.
  static const Duration refusalProbe = Duration(seconds: 4);

  late final DongleRepository _repo;
  late final Obd2Repository _obd;
  StreamSubscription<BleConnectionPhase>? _sub;
  StreamSubscription<DonglePairingHint?>? _hintSub;
  // Lidos sob demanda: dependem de persistência que só o fluxo de conexão usa.
  LastDongleRepository get _lastDongle =>
      ref.read(lastDongleRepositoryProvider);
  PrepareSecureConnectionUseCase get _plan =>
      ref.read(prepareSecureConnectionUseCaseProvider);

  String? _deviceId;
  DonglePairingHint? _lastHint;

  @override
  ConnectingState build() {
    _repo = ref.read(dongleRepositoryProvider);
    _obd = ref.read(obd2RepositoryProvider);
    ref.onDispose(() {
      unawaited(_sub?.cancel());
      unawaited(_hintSub?.cancel());
    });
    return const ConnectingState();
  }

  /// `true` quando há uma conexão segura em andamento (a tela de conexão não
  /// deve reiniciá-la ao ser montada).
  bool get inProgress => switch (state.security) {
    LinkSecurityStep.planning ||
    LinkSecurityStep.pairing ||
    LinkSecurityStep.restoring ||
    LinkSecurityStep.removingBond => true,
    _ => false,
  };

  /// Inicia a conexão segura com [deviceId] e passa a refletir as fases.
  ///
  /// Reinicia o estado do zero (fase + preparação): este ViewModel é mantido
  /// vivo globalmente (o `ConnectionGuard` o observa), então numa RECONEXÃO
  /// após "Conexão perdida" o `prep` ainda estaria em `done` da sessão
  /// anterior. [hint] é a dica do anúncio visto na busca.
  Future<void> connect(String deviceId, {DonglePairingHint? hint}) async {
    _deviceId = deviceId;
    _lastHint = hint ?? _lastHint;
    state = ConnectingState(
      security: LinkSecurityStep.planning,
      canManageBonds: _repo.canManageBonds,
    );
    await _stopHintWatch();
    final SecureConnectionPlan plan;
    try {
      plan = await _plan(deviceId: deviceId, hint: _lastHint);
    } on Object {
      state = state.copyWith(
        phase: BleConnectionPhase.failed,
        security: LinkSecurityStep.failed,
      );
      return;
    }
    switch (plan) {
      case SecureConnectionPlan.restore:
        state = state.copyWith(security: LinkSecurityStep.restoring);
        await _startLink(deviceId);
      case SecureConnectionPlan.pairNow:
        state = state.copyWith(security: LinkSecurityStep.pairing);
        await _startLink(deviceId);
      case SecureConnectionPlan.waitForButton:
        _waitForButton(deviceId);
    }
  }

  Future<void> _startLink(String deviceId) async {
    state = state.copyWith(phase: BleConnectionPhase.connecting);
    await _sub?.cancel();
    _sub = _repo.connectionPhase.listen(
      _onPhase,
      onError: (_) => _onFailed(BleLinkError.other),
    );
    try {
      await _repo.connect(deviceId);
    } on Failure {
      _onFailed(BleLinkError.other);
    }
  }

  void _onPhase(BleConnectionPhase p) {
    final wasReady = state.phase == BleConnectionPhase.ready;
    if (p == BleConnectionPhase.failed) {
      _onFailed(_repo.lastLinkError ?? BleLinkError.other);
      return;
    }
    state = state.copyWith(phase: p);
    if (p == BleConnectionPhase.ready) {
      state = state.copyWith(
        security: LinkSecurityStep.secured,
        clearError: true,
      );
      if (state.prep == ConnectingPrep.idle) unawaited(_prepare());
      if (!wasReady) _rememberDevice();
    }
  }

  /// Decide o que fazer após uma falha do enlace (tabela da spec fase-2/04
  /// §5.6). Atualiza fase e etapa de uma vez para a UI não ver estados
  /// intermediários.
  void _onFailed(BleLinkError error) {
    final id = _deviceId;
    final canManage = state.canManageBonds;
    final lost = error.isPairingLost;
    LinkSecurityStep next;
    switch (state.security) {
      case LinkSecurityStep.pairing:
        final refused =
            error == BleLinkError.pairingRejected ||
            error == BleLinkError.pairingTimeout ||
            error == BleLinkError.notAuthorized;
        if (refused && id != null) {
          // Recusa ou janela fechada? Só um anúncio novo distingue.
          state = state.copyWith(
            phase: BleConnectionPhase.failed,
            linkError: error,
          );
          unawaited(_resolveRefusal(id));
          return;
        }
        next = LinkSecurityStep.failed;
      case LinkSecurityStep.restoring:
        if (!lost) {
          next = LinkSecurityStep.failed;
        } else if (canManage) {
          next = LinkSecurityStep.removingBond;
        } else if (error == BleLinkError.bondLostIos) {
          next = LinkSecurityStep.forgetOnIos;
        } else {
          next = LinkSecurityStep.waitingButton;
        }
      case LinkSecurityStep.idle:
      case LinkSecurityStep.planning:
      case LinkSecurityStep.waitingButton:
      case LinkSecurityStep.secured:
      case LinkSecurityStep.rejected:
      case LinkSecurityStep.windowExpired:
      case LinkSecurityStep.removingBond:
      case LinkSecurityStep.forgetOnIos:
      case LinkSecurityStep.failed:
        // `secured` (queda depois de pronto) ou qualquer outra: a tela de
        // "Conexão perdida" oferece "Parear novamente" quando o motivo é de
        // pareamento (ver `repair`).
        next = LinkSecurityStep.failed;
    }
    state = state.copyWith(
      phase: BleConnectionPhase.failed,
      security: next,
      linkError: error,
    );
    if (id == null) return;
    switch (next) {
      case LinkSecurityStep.removingBond:
        unawaited(_removeBondAndReplan(id));
      case LinkSecurityStep.forgetOnIos:
        unawaited(_lastDongle.clear().catchError((_) {}));
      case LinkSecurityStep.waitingButton:
        // iOS: o dongle não reconheceu este aparelho; o registro caducou.
        unawaited(_lastDongle.clear().catchError((_) {}));
        _waitForButton(id);
      case LinkSecurityStep.idle:
      case LinkSecurityStep.planning:
      case LinkSecurityStep.pairing:
      case LinkSecurityStep.restoring:
      case LinkSecurityStep.secured:
      case LinkSecurityStep.rejected:
      case LinkSecurityStep.windowExpired:
      case LinkSecurityStep.failed:
        break;
    }
  }

  /// Depois de uma recusa durante o pareamento, olha o anúncio mais recente:
  /// janela ainda aberta = o pareamento foi recusado; fechada (ou dongle não
  /// visto) = o tempo para parear acabou.
  Future<void> _resolveRefusal(String id) async {
    var open = false;
    try {
      final hint = await _repo
          .watchPairingHint(id, timeout: refusalProbe)
          .where((h) => h != null)
          .first
          .timeout(refusalProbe);
      _lastHint = hint;
      open = hint?.windowOpen ?? false;
    } on Object {
      open = false;
    }
    state = state.copyWith(
      security: open
          ? LinkSecurityStep.rejected
          : LinkSecurityStep.windowExpired,
    );
  }

  /// Android: derruba a conexão antiga (remover o bond com o ACL ativo
  /// derrubaria o enlace e o adapter tentaria reconectar sozinho), remove o
  /// bond e replaneja (agora sem bond).
  Future<void> _removeBondAndReplan(String id) async {
    try {
      await _repo.disconnect();
      await _repo.removeBond(id);
    } on Object {
      state = state.copyWith(security: LinkSecurityStep.failed);
      return;
    }
    await connect(id);
  }

  void _waitForButton(String deviceId) {
    state = state.copyWith(
      phase: BleConnectionPhase.idle,
      security: LinkSecurityStep.waitingButton,
      dongleSeen: false,
      watching: true,
    );
    unawaited(_hintSub?.cancel());
    _hintSub = _repo
        .watchPairingHint(deviceId, timeout: hintTimeout)
        .listen(
          (hint) {
            if (hint == null) return;
            _lastHint = hint;
            if (hint.windowOpen) {
              unawaited(_stopHintWatch());
              state = state.copyWith(
                security: LinkSecurityStep.pairing,
                watching: false,
              );
              unawaited(_startLink(deviceId));
            } else {
              state = state.copyWith(dongleSeen: true);
            }
          },
          onError: (_) => state = state.copyWith(watching: false),
          onDone: () {
            if (state.security == LinkSecurityStep.waitingButton) {
              state = state.copyWith(dongleSeen: false, watching: false);
            }
          },
        );
  }

  Future<void> _stopHintWatch() async {
    final sub = _hintSub;
    _hintSub = null;
    await sub?.cancel();
  }

  /// Preparação OBD-II sobre a conexão já `ready`: init do ELM327 e leitura das
  /// capacidades. Best-effort — se qualquer passo falhar, ainda conclui
  /// (`done`) e as telas seguintes degradam graciosamente (protocolo `—`, 0
  /// sensores); o link BLE está saudável, então não é "Conexão perdida".
  Future<void> _prepare() async {
    state = state.copyWith(prep: ConnectingPrep.preparing);
    try {
      await _obd.initialize();
      state = state.copyWith(prep: ConnectingPrep.reading);
      await _obd.discoverSupported();
    } on Object {
      // Best-effort: segue mesmo assim.
    }
    state = state.copyWith(prep: ConnectingPrep.done);
  }

  /// Grava o dongle selecionado como o último conectado com sucesso (só depois
  /// de uma sessão autorizada), para a próxima abertura do app reconectar
  /// sozinha. Best-effort.
  void _rememberDevice() {
    final device = ref.read(selectedDongleProvider);
    if (device == null) return;
    unawaited(
      _lastDongle
          .save(LastDongle(id: device.id, name: device.name))
          .catchError((_) {}),
    );
  }

  /// "Tentar de novo" das telas de pareamento.
  Future<void> retry() async {
    final id = _deviceId;
    if (id == null) return;
    switch (state.security) {
      case LinkSecurityStep.rejected:
        await connect(id);
      case LinkSecurityStep.windowExpired:
      case LinkSecurityStep.waitingButton:
        _waitForButton(id);
      case LinkSecurityStep.idle:
      case LinkSecurityStep.planning:
      case LinkSecurityStep.pairing:
      case LinkSecurityStep.restoring:
      case LinkSecurityStep.secured:
      case LinkSecurityStep.removingBond:
      case LinkSecurityStep.forgetOnIos:
      case LinkSecurityStep.failed:
        break;
    }
  }

  /// iOS: "Já pareei este iPhone" — tenta restaurar com o bond do SO.
  Future<void> assumePaired() async {
    final id = _deviceId;
    if (id == null || state.canManageBonds) return;
    if (state.security != LinkSecurityStep.waitingButton) return;
    await _stopHintWatch();
    state = state.copyWith(
      security: LinkSecurityStep.restoring,
      watching: false,
    );
    await _startLink(id);
  }

  /// iOS: "Já esqueci" — volta a esperar o botão do dongle.
  void confirmForgotten() {
    final id = _deviceId;
    if (id == null || state.security != LinkSecurityStep.forgetOnIos) return;
    _waitForButton(id);
  }

  /// "Parear novamente" da tela de conexão perdida: prepara o terreno para
  /// refazer o pareamento. Android remove o bond antigo (a tela segue para a
  /// conexão, que já replaneja sem bond); iOS só pode orientar o usuário.
  Future<void> repair() async {
    final id = _deviceId;
    if (id == null) return;
    await _stopHintWatch();
    if (state.canManageBonds) {
      try {
        await _repo.disconnect();
        await _repo.removeBond(id);
      } on Object {
        state = state.copyWith(security: LinkSecurityStep.failed);
        return;
      }
      state = state.copyWith(security: LinkSecurityStep.idle);
    } else {
      await _lastDongle.clear().catchError((_) {});
      state = state.copyWith(security: LinkSecurityStep.forgetOnIos);
    }
  }

  /// Cancela/encerra a conexão em andamento.
  Future<void> cancel() async {
    await _stopHintWatch();
    state = state.copyWith(
      security: LinkSecurityStep.idle,
      watching: false,
    );
    await _repo.disconnect();
  }
}

/// Provider do [ConnectingViewModel].
final NotifierProvider<ConnectingViewModel, ConnectingState>
connectingViewModelProvider =
    NotifierProvider<ConnectingViewModel, ConnectingState>(
      ConnectingViewModel.new,
    );
