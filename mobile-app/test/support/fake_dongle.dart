import 'dart:async';
import 'dart:typed_data';

import 'package:tccelta_mobile/src/domain/repositories/dongle_sync_repository.dart';
import 'package:tccelta_mobile/src/domain/sync/dev_id.dart';
import 'package:tccelta_mobile/src/domain/sync/fragment_assembler.dart';
import 'package:tccelta_mobile/src/domain/sync/handshake_messages.dart';
import 'package:tccelta_mobile/src/domain/sync/new_block_notice.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_block.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_frame.dart';
import 'package:tccelta_mobile/src/domain/sync/sync_protocol.dart';
import 'package:tccelta_mobile/src/infra/sync/bench/bench_crypto.dart';
import 'package:tccelta_mobile/src/infra/sync/bench/bench_keys.dart';

import 'sync_test_blocks.dart';

/// [DongleSyncRepository] que emula o dongle (spec 05 em versão mínima):
/// confere as tags do desafio e do ACK com as chaves de bancada, mantém
/// `last_cs`, sessão, cursor e `ACK` local, serve lotes e erros.
class FakeDongle implements DongleSyncRepository {
  /// Cria o dongle falso.
  FakeDongle({BenchKeys? keys})
    : keys = keys ?? BenchKeys.defaults(),
      devId = benchDevId;

  /// Chaves de bancada.
  final BenchKeys keys;

  /// Identidade.
  final DevId devId;

  final StreamController<DongleSyncEvent> _ctrl =
      StreamController<DongleSyncEvent>.broadcast();

  // Estado do dongle.
  bool linkUp = false;
  int generation = 0;
  int tail = 1;
  int head = 0;
  int bootId = 3;
  int tMs = 123456;
  int lastCs = 0;
  int ackLocal = 0;
  int cursor = 1;
  bool sessionOpen = false;
  int? _answeredCs;
  final Map<int, SyncBlock> blocks = {};

  /// `seq` cujo envio será engolido uma vez (perda no transporte).
  final Set<int> swallowOnce = {};

  // Comportamentos.
  /// Responde sozinho aos desafios.
  bool autoRespond = true;

  /// Serve blocos sozinho a cada pedido de lote.
  bool autoServe = true;

  /// Emite o aviso sem o bit 0 depois de um ACK de abertura válido.
  bool confirmAck = true;

  /// Se não nulo, devolve este erro a todo desafio.
  SyncErrorCode? challengeError;

  /// Se não nulo, devolve este erro a todo ACK.
  SyncErrorCode? ackError;

  // Registro do que o app enviou.
  final List<SyncFrame> challenges = [];
  final List<SyncFrame> acks = [];
  final List<int> batches = [];
  final List<SyncErrorCode> errors = [];

  @override
  Stream<DongleSyncEvent> get events => _ctrl.stream;

  @override
  bool get isLinkUp => linkUp;

  @override
  int get linkGeneration => generation;

  @override
  int get mtu => 247;

  void _emit(DongleSyncEvent e) {
    if (!_ctrl.isClosed) _ctrl.add(e);
  }

  /// Sobe o link: nova geração, sessão fechada, cursor em `ACK + 1`.
  void connect() {
    generation++;
    linkUp = true;
    sessionOpen = false;
    _answeredCs = null;
    cursor = ackLocal + 1;
    _emit(SyncLinkChanged(up: true, generation: generation));
  }

  /// Derruba o link.
  void disconnect() {
    linkUp = false;
    sessionOpen = false;
    _emit(SyncLinkChanged(up: false, generation: generation));
  }

  /// Emite o aviso; sem [flags], bit 0 enquanto não há sessão.
  void notice({int? flags}) {
    _emit(
      NoticeReceived(
        NewBlockNotice(
          headSeq: head,
          flags: NoticeFlags(flags ?? (sessionOpen ? 0 : 0x01)),
          devId: devId,
        ),
      ),
    );
  }

  /// Grava blocos de dados `[from, to]` na "flash" e avança o HEAD.
  void addData(int from, int to) {
    for (var s = from; s <= to; s++) {
      blocks[s] = dataBlock(s);
    }
    if (to > head) head = to;
  }

  /// Grava um bloco qualquer na "flash".
  void addBlock(SyncBlock b) {
    blocks[b.seq] = b;
    if (b.seq > head) head = b.seq;
  }

  /// Emite o erro [code] como se viesse do dongle.
  void emitError(SyncErrorCode code) => _emit(DongleErrorReceived(code));

  /// Emite [block] direto, sem passar pelo cursor.
  void emitBlock(SyncBlock block) => _emit(BlockReceived(block));

  /// Emite uma falha de transporte.
  void emitIncomplete() =>
      _emit(const TransportIncomplete(AssemblyErrorKind.timeout));

  void _serve(int k) {
    for (var i = 0; i < k; i++) {
      final candidates = blocks.keys.where((s) => s >= cursor).toList()..sort();
      if (candidates.isEmpty) return;
      final b = blocks[candidates.first]!;
      final contiguous =
          b.seq == cursor ||
          (b.marker == MarkerKind.boot && b.fromSeq <= cursor);
      if (!contiguous) return;
      cursor = b.lastCoveredSeq + 1;
      if (swallowOnce.remove(b.seq)) continue;
      _emit(BlockReceived(b));
    }
  }

  @override
  Future<void> sendChallenge(SyncFrame challenge) async {
    challenges.add(challenge);
    final err = challengeError;
    if (err != null) {
      _emit(DongleErrorReceived(err));
      return;
    }
    final v = challenge.value;
    final signed = Uint8List.sublistView(v, 0, v.length - 16);
    final expected = handshakeTag(
      kAuth: keys.kAuth,
      type: 0x01,
      devId: devId.bytes,
      payload: signed,
    );
    if (!constantTimeEquals(expected, v.sublist(v.length - 16))) {
      _emit(const DongleErrorReceived(SyncErrorCode.invalidTag));
      return;
    }
    final info = ChallengeInfo.fromValue(v);
    if (info.cS <= lastCs) {
      _emit(const DongleErrorReceived(SyncErrorCode.staleChallenge));
      return;
    }
    lastCs = info.cS;
    _answeredCs = info.cS;
    if (!autoRespond) return;
    emitResponse(info.cS, v.sublist(4, 20));
  }

  /// Emite uma Resposta 0x02 para [cS] com o [nonce] dado (por padrão, o do
  /// último desafio recebido), com tag válida.
  void emitResponse(int cS, [List<int>? nonce]) {
    final n = nonce ?? challenges.last.value.sublist(4, 20);
    final payload = Uint8List(34);
    payload.buffer.asByteData()
      ..setUint32(0, cS, Endian.little)
      ..setUint32(20, tail, Endian.little)
      ..setUint32(24, head, Endian.little)
      ..setUint16(28, bootId, Endian.little)
      ..setUint32(30, tMs, Endian.little);
    payload.setRange(4, 20, n);
    final tag = handshakeTag(
      kAuth: keys.kAuth,
      type: 0x02,
      devId: devId.bytes,
      payload: payload,
    );
    final frame = SyncFrame(
      SyncMessageType.response,
      Uint8List.fromList([...payload, ...tag]),
    );
    _emit(ResponseReceived(frame, ResponseInfo.fromValue(frame.value)));
  }

  @override
  Future<void> sendAck(SyncFrame ack) async {
    acks.add(ack);
    final err = ackError;
    if (err != null) {
      _emit(DongleErrorReceived(err));
      return;
    }
    final v = ack.value;
    final expected = handshakeTag(
      kAuth: keys.kAuth,
      type: 0x04,
      devId: devId.bytes,
      payload: Uint8List.sublistView(v, 0, 9),
    );
    if (!constantTimeEquals(expected, v.sublist(9))) {
      _emit(const DongleErrorReceived(SyncErrorCode.invalidTag));
      return;
    }
    final info = AckInfo.fromValue(v);
    final opening = !sessionOpen;
    if (opening && info.cConf != _answeredCs) {
      _emit(const DongleErrorReceived(SyncErrorCode.invalidAckSeq));
      return;
    }
    if (info.ackSeq > head) {
      _emit(const DongleErrorReceived(SyncErrorCode.invalidAckSeq));
      return;
    }
    sessionOpen = true;
    if (info.ackSeq > ackLocal) ackLocal = info.ackSeq;
    blocks.removeWhere((s, _) => s <= ackLocal);
    if (cursor < ackLocal + 1) cursor = ackLocal + 1;
    if (opening && confirmAck) notice(flags: 0);
  }

  @override
  Future<void> requestBatch(int k) async {
    batches.add(k);
    if (!sessionOpen) {
      _emit(const DongleErrorReceived(SyncErrorCode.noSession));
      return;
    }
    if (autoServe) _serve(k);
  }

  @override
  Future<void> sendError(SyncErrorCode code) async {
    errors.add(code);
    cursor = ackLocal + 1;
  }
}
