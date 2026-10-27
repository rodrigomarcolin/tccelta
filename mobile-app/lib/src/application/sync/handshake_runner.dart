import 'dart:async';

import 'package:flutter/foundation.dart';
import 'package:tccelta_mobile/src/core/errors/sync_failure.dart';
import 'package:tccelta_mobile/src/domain/repositories/dongle_sync_repository.dart';
import 'package:tccelta_mobile/src/domain/repositories/sync_server_repository.dart';
import 'package:tccelta_mobile/src/domain/sync/dev_id.dart';
import 'package:tccelta_mobile/src/domain/sync/handshake_messages.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';

/// O que disparou um handshake.
enum HandshakeTrigger {
  /// Aviso com o bit 0: nenhum handshake nesta conexão.
  noticeNoSession,

  /// Aviso com o bit 1: nenhum handshake neste boot.
  noticeNoBootAnchor,

  /// Token perto de expirar.
  tokenExpiring,

  /// Servidor respondeu 401.
  unauthorized,

  /// Dongle respondeu 0x05 (sem sessão).
  dongleNoSession,

  /// Dongle recusou o desafio ou o ACK.
  dongleRejected,

  /// O ACK trouxe âncora de tempo falha.
  anchorFailed,

  /// Ação manual de depuração.
  manual,
}

/// Perna BLE do handshake da Fig. 5: repassa o Desafio 0x01 ao dongle, coleta
/// a Resposta 0x02, entrega-a ao servidor e escreve o ACK 0x04 no dongle. O
/// gatilho é decidido por quem chama.
class HandshakeRunner {
  /// Cria o runner sobre o canal do dongle e o servidor.
  HandshakeRunner({
    required this.dongle,
    required this.server,
    this.stepTimeout = kHandshakeStepTimeout,
    this.confirmTimeout = const Duration(seconds: 1),
  });

  /// Canal SYNC com o dongle.
  final DongleSyncRepository dongle;

  /// Servidor de sincronização.
  final SyncServerRepository server;

  /// Espera pela Resposta do dongle (passo 3).
  final Duration stepTimeout;

  /// Espera pela confirmação do ACK (passo 6).
  final Duration confirmTimeout;

  /// Executa o handshake para [devId]. [generation] é a geração do link SYNC
  /// no início; se o link cair ou mudar de geração durante qualquer espera,
  /// lança [HandshakeLinkLost].
  Future<HandshakeOutcome> run(
    DevId devId, {
    required HandshakeTrigger trigger,
    required int generation,
  }) async {
    final watch = Stopwatch()..start();
    var lost = false;
    final buffer = <DongleSyncEvent>[];
    Completer<void>? wake;

    // Passo 0: assina antes de qualquer envio (evita corrida com a resposta).
    final sub = dongle.events.listen((e) {
      if (e is SyncLinkChanged && (!e.up || e.generation != generation)) {
        lost = true;
      }
      buffer.add(e);
      if (wake != null && !wake!.isCompleted) wake!.complete();
    });

    void log(int step, [String extra = '']) {
      if (kDebugMode) {
        debugPrint(
          'SYNC ${watch.elapsedMilliseconds} HS_STEP n=$step '
          'trigger=${trigger.name}$extra',
        );
      }
    }

    void checkLost() {
      if (lost) throw const HandshakeLinkLost();
    }

    /// Espera o primeiro evento que [pick] aceita; `null` no *timeout*.
    Future<T?> waitFor<T>(
      T? Function(DongleSyncEvent e) pick,
      Duration timeout,
    ) async {
      var timedOut = false;
      final timer = Timer(timeout, () {
        timedOut = true;
        if (wake != null && !wake!.isCompleted) wake!.complete();
      });
      try {
        while (true) {
          while (buffer.isNotEmpty) {
            final r = pick(buffer.removeAt(0));
            if (r != null) return r;
          }
          checkLost();
          if (timedOut) return null;
          wake = Completer<void>();
          await wake!.future;
        }
      } finally {
        timer.cancel();
      }
    }

    try {
      // Passo 1: desafio do servidor.
      final challenge = await server.requestChallenge(devId);
      checkLost();
      final info = ChallengeInfo.fromValue(challenge.value);
      log(1, ' cS=${info.cS}');

      // Passo 2: repassa ao dongle.
      await dongle.sendChallenge(challenge);
      log(2, ' cS=${info.cS}');

      // Passo 3: espera a resposta com o mesmo c_S.
      final response = await waitFor<ResponseReceived>((e) {
        if (e is SyncLinkChanged && (!e.up || e.generation != generation)) {
          throw const HandshakeLinkLost();
        }
        if (e is DongleErrorReceived &&
            (e.code == SyncErrorCode.invalidTag ||
                e.code == SyncErrorCode.staleChallenge)) {
          throw HandshakeRejectedByDongle(e.code.code);
        }
        if (e is ResponseReceived && e.info.cS == info.cS) return e;
        return null;
      }, stepTimeout);
      if (response == null) throw const HandshakeTimeout();
      log(3, ' cS=${info.cS}');

      // Passo 4: resposta ao servidor.
      final outcome = await server.submitResponse(devId, response.frame);
      checkLost();
      log(4, ' cS=${info.cS}');

      // Passo 5: ACK ao dongle.
      await dongle.sendAck(outcome.ack);
      log(5, ' cS=${info.cS}');

      // Passo 6: confirmação positiva (aviso sem o bit 0) ou erro.
      await waitFor<bool>((e) {
        if (e is SyncLinkChanged && (!e.up || e.generation != generation)) {
          throw const HandshakeLinkLost();
        }
        if (e is DongleErrorReceived &&
            (e.code == SyncErrorCode.invalidTag ||
                e.code == SyncErrorCode.invalidAckSeq)) {
          throw HandshakeAckRejected(e.code.code);
        }
        if (e is NoticeReceived && !e.notice.flags.noHandshakeConnection) {
          return true;
        }
        return null;
      }, confirmTimeout);
      log(6, ' cS=${info.cS}');

      // Passo 7.
      return outcome;
    } finally {
      await sub.cancel();
    }
  }
}
