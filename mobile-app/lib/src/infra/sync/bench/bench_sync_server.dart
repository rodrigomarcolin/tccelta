import 'dart:async';
import 'dart:math';
import 'dart:typed_data';

import 'package:tccelta_mobile/src/domain/sync/dev_id.dart';
import 'package:tccelta_mobile/src/domain/sync/handshake_messages.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_block.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';
import 'package:tccelta_mobile/src/infra/sync/bench/bench_crypto.dart';
import 'package:tccelta_mobile/src/infra/sync/bench/bench_keys.dart';
import 'package:tccelta_mobile/src/services/sync/sync_server_exceptions.dart';
import 'package:tccelta_mobile/src/services/sync/sync_server_gateway.dart';

/// Causa de uma lacuna registrada pelo servidor de bancada.
enum BenchGapCause {
  /// Marcador de boot (queda de energia).
  boot,

  /// Marcador de perda (CRC inválido).
  loss,

  /// Faixa descartada por flash cheia (TAIL à frente do ACK).
  overflow,
}

/// Faixa de `seq` que o servidor deu por perdida.
class BenchGap {
  /// Cria a lacuna.
  const BenchGap(this.from, this.to, this.cause);

  /// Primeiro `seq`.
  final int from;

  /// Último `seq`.
  final int to;

  /// Causa.
  final BenchGapCause cause;
}

/// Contadores exibidos na tela de depuração.
class BenchStats {
  /// Cria os contadores.
  const BenchStats({
    this.accepted = 0,
    this.rejected = 0,
    this.unverified = 0,
    this.gaps = const [],
    this.lastAck,
  });

  /// Blocos aceitos.
  final int accepted;

  /// Blocos rejeitados (tag, ordem ou formato).
  final int rejected;

  /// Blocos aceitos sem verificar a tag (perfis `none` e `asym`).
  final int unverified;

  /// Lacunas registradas.
  final List<BenchGap> gaps;

  /// Último `ACK_seq` emitido.
  final int? lastAck;
}

class _Pending {
  _Pending(this.nonce, this.issuedAt);

  final Uint8List nonce;
  final DateTime issuedAt;
  bool used = false;
}

class _Token {
  _Token(this.devId, this.cS, this.expiresAt);

  final DevId devId;
  final int cS;
  final DateTime expiresAt;
}

class _DongleState {
  int lastAcceptedCs = 0;
  int ackSeq = 0;
  int head = 0;
  final Map<int, _Pending> pending = {};
}

/// Servidor de bancada em memória (lógica pura, sem rede): emite desafios,
/// confere respostas, aceita lotes e devolve ACKs com as regras do documento
/// §4.3–4.4. Serve de oráculo de contrato para o servidor real.
class BenchSyncServer {
  /// Cria o servidor; [now] e [random] são injetáveis para testes.
  BenchSyncServer({
    BenchKeys? keys,
    DateTime Function()? now,
    Random? random,
  }) : keys = keys ?? BenchKeys.defaults(),
       _now = now ?? DateTime.now,
       _random = random ?? Random.secure();

  /// Validade de um desafio emitido.
  static const Duration challengeValidity = Duration(seconds: 60);

  /// Validade do token.
  static const Duration tokenValidity = Duration(hours: 1);

  /// Diferença máxima entre a emissão do desafio e a resposta para a âncora
  /// de tempo valer.
  static const Duration anchorTolerance = Duration(seconds: 2);

  /// Chaves do servidor.
  final BenchKeys keys;

  final DateTime Function() _now;
  final Random _random;
  final Map<String, _DongleState> _dongles = {};
  final Map<String, _Token> _tokens = {};
  final List<BenchGap> _gaps = [];
  final StreamController<BenchStats> _statsCtrl =
      StreamController<BenchStats>.broadcast();
  int _lastIssuedCs = 0;
  int _accepted = 0;
  int _rejected = 0;
  int _unverified = 0;
  int? _lastAck;

  /// Estatísticas atuais.
  BenchStats get stats => BenchStats(
    accepted: _accepted,
    rejected: _rejected,
    unverified: _unverified,
    gaps: List.unmodifiable(_gaps),
    lastAck: _lastAck,
  );

  /// Emite as estatísticas a cada mudança.
  Stream<BenchStats> get statsStream => _statsCtrl.stream;

  _DongleState _state(DevId id) =>
      _dongles.putIfAbsent(id.hex, _DongleState.new);

  void _notify() {
    if (!_statsCtrl.isClosed) _statsCtrl.add(stats);
  }

  /// Zera o estado de todos os dongles menos `lastIssuedCs` (mantém `c_S`
  /// crescente, já que o dongle persiste `last_cs`).
  void forgetDongles() {
    _dongles.clear();
    _tokens.clear();
    _gaps.clear();
    _accepted = 0;
    _rejected = 0;
    _unverified = 0;
    _lastAck = null;
    _notify();
  }

  /// Avança `lastIssuedCs` (recuperação após erro 0x02 por relógio atrasado).
  void bumpIssuedCs(int cS) {
    if (cS > _lastIssuedCs) _lastIssuedCs = cS;
  }

  /// Emite o Desafio 0x01 para [devId].
  ChallengeEnvelope requestChallenge(DevId devId) {
    final st = _state(devId);
    final now = _now();
    final unixSeconds = now.millisecondsSinceEpoch ~/ 1000;
    final cS = max(_lastIssuedCs + 1, unixSeconds) & 0xFFFFFFFF;
    _lastIssuedCs = cS;
    final nonce = Uint8List.fromList(
      List<int>.generate(16, (_) => _random.nextInt(256)),
    );
    st.pending[cS] = _Pending(nonce, now);

    final payload = Uint8List(29);
    final d = payload.buffer.asByteData()
      ..setUint32(0, cS, Endian.little)
      ..setUint32(20, st.ackSeq, Endian.little)
      ..setUint32(24, st.lastAcceptedCs, Endian.little);
    payload
      ..setRange(4, 20, nonce)
      ..[28] = 0;
    assert(d.lengthInBytes == 29, 'payload do desafio');
    final tag = handshakeTag(
      kAuth: keys.kAuth,
      type: SyncMessageType.challenge.code,
      devId: devId.bytes,
      payload: payload,
    );
    return ChallengeEnvelope(
      challenge: SyncFrame(
        SyncMessageType.challenge,
        Uint8List.fromList([...payload, ...tag]),
      ),
      expiresAt: now.add(challengeValidity).toUtc(),
    );
  }

  /// Confere a Resposta 0x02 e devolve o ACK 0x04 e o token.
  HandshakeEnvelope submitResponse(DevId devId, SyncFrame response) {
    if (response.type != SyncMessageType.response ||
        response.value.length != kResponseLength) {
      throw const ServerMalformed('tlv_invalido');
    }
    final v = response.value;
    final signed = Uint8List.sublistView(v, 0, kResponseLength - 16);
    final expected = handshakeTag(
      kAuth: keys.kAuth,
      type: SyncMessageType.response.code,
      devId: devId.bytes,
      payload: signed,
    );
    if (!constantTimeEquals(expected, v.sublist(kResponseLength - 16))) {
      throw const ServerResponseRejected();
    }
    final info = ResponseInfo.fromValue(v);
    final st = _state(devId);
    final now = _now();
    final pending = st.pending[info.cS];
    final nonce = v.sublist(4, 20);
    if (pending == null ||
        pending.used ||
        now.difference(pending.issuedAt) > challengeValidity ||
        !constantTimeEquals(pending.nonce, nonce)) {
      throw const ServerChallengeRejected();
    }
    if (info.tailSeq > info.headSeq + 1 || info.headSeq < st.head) {
      throw const ServerResponseRejected(inconsistent: true);
    }
    pending.used = true;
    st
      ..lastAcceptedCs = info.cS
      ..head = info.headSeq;
    if (info.tailSeq > st.ackSeq + 1) {
      _gaps.add(
        BenchGap(st.ackSeq + 1, info.tailSeq - 1, BenchGapCause.overflow),
      );
      st.ackSeq = info.tailSeq - 1;
    }
    final anchorFailed = now.difference(pending.issuedAt) > anchorTolerance;
    final token = 'bench.${devId.hex}.${info.cS}';
    final expires = now.add(tokenValidity).toUtc();
    _tokens[token] = _Token(devId, info.cS, expires);
    _lastAck = st.ackSeq;
    _notify();
    return HandshakeEnvelope(
      ack: _ack(devId, st.ackSeq, info.cS, anchorFailed: anchorFailed),
      token: token,
      tokenExpiresAt: expires,
    );
  }

  /// Aceita um lote de blocos e devolve o ACK; interrompe no primeiro bloco
  /// inaceitável.
  SyncFrame postBlocks(String token, List<SyncFrame> blocks) {
    final tk = _tokens[token];
    if (tk == null) throw const ServerUnauthorized();
    if (!tk.expiresAt.isAfter(_now())) {
      throw const ServerUnauthorized('token_expirado');
    }
    if (blocks.length > kMaxBlocksPerPost) {
      throw const ServerRateLimited(Duration(seconds: 1));
    }
    final st = _state(tk.devId);
    final before = st.ackSeq;
    var interrupted = false;
    for (final frame in blocks) {
      if (!_acceptBlock(tk.devId, st, frame)) {
        _rejected++;
        interrupted = true;
        break;
      }
    }
    _lastAck = st.ackSeq;
    _notify();
    final ack = _ack(tk.devId, st.ackSeq, tk.cS, anchorFailed: false);
    if (interrupted && st.ackSeq == before) throw ServerBatchNoProgress(ack);
    return ack;
  }

  bool _acceptBlock(DevId devId, _DongleState st, SyncFrame frame) {
    if (frame.type != SyncMessageType.block ||
        frame.value.length < kBlockHeaderSize) {
      return false;
    }
    final v = frame.value;
    final size = v.buffer
        .asByteData(v.offsetInBytes, v.length)
        .getUint16(10, Endian.little);
    final tagLen = v.length - size;
    if (size < kBlockHeaderSize ||
        (tagLen != 0 && tagLen != 32 && tagLen != 64)) {
      return false;
    }
    final verified = tagLen == blockTagLength;
    if (verified) {
      final expected = blockTag(
        kRec: keys.kRec,
        devId: devId.bytes,
        data: Uint8List.sublistView(v, 0, size),
      );
      if (!constantTimeEquals(expected, v.sublist(size))) return false;
    }
    final SyncBlock b;
    try {
      b = SyncBlock.fromFrame(frame);
    } on FormatException {
      return false;
    }
    final next = st.ackSeq + 1;
    if (b.seq <= st.ackSeq) return true; // duplicado ou coberto por OVERFLOW
    if (b.seq == next) {
      switch (b.marker) {
        case MarkerKind.none:
          st.ackSeq = b.seq;
        case MarkerKind.loss:
          if (b.fromSeq != b.seq || b.toSeq < b.seq) return false;
          _gaps.add(BenchGap(b.fromSeq, b.toSeq, BenchGapCause.loss));
          st.ackSeq = b.toSeq;
        case MarkerKind.boot:
          if (b.toSeq >= b.fromSeq) {
            _gaps.add(BenchGap(b.fromSeq, b.toSeq, BenchGapCause.boot));
          }
          st.ackSeq = b.seq;
      }
    } else if (b.marker == MarkerKind.boot &&
        b.toSeq == b.seq - 1 &&
        b.fromSeq <= next) {
      _gaps.add(BenchGap(next, b.toSeq, BenchGapCause.boot));
      st.ackSeq = b.seq;
    } else {
      return false;
    }
    _accepted++;
    if (!verified) _unverified++;
    return true;
  }

  SyncFrame _ack(
    DevId devId,
    int ackSeq,
    int cConf, {
    required bool anchorFailed,
  }) {
    final payload = Uint8List(9);
    payload.buffer.asByteData()
      ..setUint32(0, ackSeq, Endian.little)
      ..setUint32(4, cConf, Endian.little);
    payload[8] = anchorFailed ? 1 : 0;
    final tag = handshakeTag(
      kAuth: keys.kAuth,
      type: SyncMessageType.ack.code,
      devId: devId.bytes,
      payload: payload,
    );
    return SyncFrame(
      SyncMessageType.ack,
      Uint8List.fromList([...payload, ...tag]),
    );
  }

  /// Libera a stream de estatísticas.
  Future<void> dispose() => _statsCtrl.close();
}
