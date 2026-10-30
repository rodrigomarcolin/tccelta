#include "sync/SyncEngine.h"

#include <cstring>

#include "collector/Events.h"
#include "collector/MarkerBlock.h"
#include "collector/SegmentFormat.h"
#include "security/DeviceKeys.h"

namespace datasync {

using collector::Block;
using collector::Event;
using collector::MarkerInfo;
using collector::MarkerKind;
using collector::ReadStatus;

namespace {

const char* kindName(MarkerKind k) {
    switch (k) {
        case MarkerKind::Boot: return "boot";
        case MarkerKind::Loss: return "loss";
        default: return "data";
    }
}

const char* noticeReasonName(uint8_t r) {
    static const char* const kNames[] = {"subscribe", "flush", "ack", "resync", "overflow", "flash", "retry"};
    return kNames[r];
}

const char* batchEndName(uint8_t r) {
    static const char* const kNames[] = {"done", "link", "overflow", "app_error", "superseded", "empty"};
    return kNames[r];
}

}  // namespace

SyncEngine::SyncEngine(const SyncDeps& deps) : _d(deps), _blk(), _loss(), _tx() {}

void SyncEngine::begin() {
    _lastAcceptedCs = _d.persistence.loadLastAcceptedCs();
    _overflowSeen = _d.store.overflowUpTo();
    _ackLocal = effectiveTail() - 1;
    _cursor = _ackLocal + 1;
    _noHsBoot = true;
    _flashHigh = _d.store.usagePermille() > 800;
}

SyncStatus SyncEngine::status() const {
    SyncStatus s;
    s.connected = _connected;
    s.subscribed = _subscribed;
    s.sessionOpen = _sessionOpen;
    s.batchActive = _batchActive;
    s.ackLocal = _ackLocal;
    s.cursor = _cursor;
    s.batchEnd = _batchEnd;
    s.head = _d.store.newestSeq();
    s.tail = _d.store.oldestSeq();
    s.lastAcceptedCs = _lastAcceptedCs;
    s.respondedCs = _respondedCs;
    s.flags = noticeFlags();
    s.mtu = _d.link.mtu();
    return s;
}

uint8_t SyncEngine::noticeFlags() const {
    return static_cast<uint8_t>((_noHsConn ? kNoticeNoHandshakeConn : 0) |
                                (_noHsBoot ? kNoticeNoHandshakeBoot : 0) | (_flashHigh ? kNoticeFlashHigh : 0));
}

// ── Conexão ──────────────────────────────────────────────────────────────

void SyncEngine::onConnected(uint32_t generation) {
    _connected = true;
    _subscribed = false;
    _generation = generation;
    _sessionOpen = false;
    _hasResponded = false;
    _noHsConn = true;
    _batchActive = false;
    _cursor = _ackLocal + 1;
    Event("SYNC_SESSION").flag("open", false).token("reason", "connect");
    Event("SYNC_CURSOR").num("from", _cursor).num("to", _cursor).token("reason", "connect");
}

void SyncEngine::onDisconnected() {
    const bool wasOpen = _sessionOpen;
    _connected = _subscribed = _sessionOpen = _batchActive = false;
    if (wasOpen) Event("SYNC_SESSION").flag("open", false).token("reason", "disconnect");
}

void SyncEngine::onSubscribed(bool on) {
    _subscribed = on;
    Event("SYNC_SUB").flag("on", on);
    if (on) sendNotice(NoticeReason::Subscribe);
}

void SyncEngine::setSession(bool open, const char* reason) {
    if (_sessionOpen == open) return;
    _sessionOpen = open;
    Event("SYNC_SESSION").flag("open", open).token("reason", reason);
}

// ── Envio de mensagens ───────────────────────────────────────────────────

bool SyncEngine::sendTlv(uint8_t type, size_t valueLen, SendStats& stats) {
    putTlvHeader(type, static_cast<uint16_t>(valueLen), _tx);
    stats = {};
    const bool ok = _d.link.send(_tx, kTlvHeaderSize + valueLen, stats);
    Event("SYNC_TX")
        .hex8("type", type)
        .num("len", static_cast<int64_t>(kTlvHeaderSize + valueLen))
        .num("frags", stats.fragments)
        .num("mtu", stats.mtu)
        .us("dur_us", stats.durUs)
        .num("retries", stats.retries)
        .flag("ok", ok);
    return ok;
}

void SyncEngine::sendError(ErrorCode code) {
    _tx[kTlvHeaderSize] = static_cast<uint8_t>(code);
    SendStats stats;
    sendTlv(static_cast<uint8_t>(MsgType::Error), kErrorLen, stats);
    const uint8_t c = static_cast<uint8_t>(code);
    Event("SYNC_ERR").token("dir", "tx").hex8("code", c);
}

void SyncEngine::sendNotice(NoticeReason reason) {
    if (!_connected || !_subscribed) return;
    const uint32_t head = _d.store.newestSeq();
    const uint8_t flags = noticeFlags();
    encodeNotice(head, flags, _d.devId, &_tx[kTlvHeaderSize]);
    SendStats stats;
    sendTlv(static_cast<uint8_t>(MsgType::Notice), kNoticeLen, stats);
    _lastNoticeMs = _d.clock.nowMs();
    Event("SYNC_NOTICE").num("head", head).hex8("flags", flags).token("reason", noticeReasonName(static_cast<uint8_t>(reason)));
}

// ── Recepção ─────────────────────────────────────────────────────────────

void SyncEngine::onMessage(const uint8_t* tlv, size_t len, uint32_t queuedUs) {
    const uint64_t startUs = _d.clock.nowUs();
    TlvView msg;
    if (!parseTlv(tlv, len, msg)) {
        Event("SYNC_RX").absent("type").num("len", static_cast<int64_t>(len)).us("q_us", queuedUs).flag("ok", false)
            .token("reason", len < kTlvHeaderSize ? "short" : "bad_len");
        return;
    }
    const bool known = msg.type == 0x01 || msg.type == 0x04 || msg.type == 0x05 || msg.type == 0x06;
    Event("SYNC_RX").hex8("type", msg.type).num("len", static_cast<int64_t>(len)).us("q_us", queuedUs)
        .flag("ok", known).token("reason", known ? "ok" : "unexpected");
    if (!known) return;

    switch (static_cast<MsgType>(msg.type)) {
        case MsgType::Challenge: handleChallenge(msg.value, msg.len, queuedUs, startUs); break;
        case MsgType::Ack: handleAck(msg.value, msg.len); break;
        case MsgType::BatchRequest: handleBatchRequest(msg.value, msg.len); break;
        case MsgType::Error: handleAppError(msg.value, msg.len); break;
        default: break;
    }
}

void SyncEngine::handleChallenge(const uint8_t* v, size_t len, uint32_t queuedUs, uint64_t startUs) {
    Challenge c;
    if (!decodeChallenge(v, len, c)) {
        Event("HS_CHALLENGE").absent("c_s").num("last_cs", _lastAcceptedCs).absent("ack").absent("c_conf")
            .absent("flags").absent("diag_min").num("len", static_cast<int64_t>(len)).us("verify_us", 0)
            .token("result", "bad_len");
        sendError(ErrorCode::InvalidTag);
        return;
    }

    uint64_t t0 = _d.clock.nowUs();
    const bool tagOk = _d.handshakeAuth.verify(security::kTypeChallenge, v, c.payloadLen, c.tag);
    const uint32_t verifyUs = sinceUs(t0);
    auto emitChallenge = [&](const char* result) {
        Event e("HS_CHALLENGE");
        e.num("c_s", c.cs).num("last_cs", _lastAcceptedCs).num("ack", c.ackSeq).num("c_conf", c.cConf)
            .hex8("flags", c.flags);
        if (c.flags & kChallengeFlagDiag) e.num("diag_min", c.diagMin);
        else e.absent("diag_min");
        e.num("len", static_cast<int64_t>(len)).us("verify_us", verifyUs).token("result", result);
    };
    if (!tagOk) {
        emitChallenge("bad_tag");
        sendError(ErrorCode::InvalidTag);
        return;
    }
    if (c.cs <= _lastAcceptedCs) {
        emitChallenge("old_cs");
        sendError(ErrorCode::StaleChallenge);
        return;
    }

    // Gravar antes de responder: uma resposta só existe para um c_S já protegido contra replay.
    t0 = _d.clock.nowUs();
    const bool saved = _d.persistence.saveLastAcceptedCs(c.cs);
    const uint32_t nvsUs = sinceUs(t0);
    if (!saved) {
        emitChallenge("nvs_fail");
        return;
    }
    emitChallenge("ok");
    _lastAcceptedCs = c.cs;

    const uint32_t head = _d.store.newestSeq();
    bool freed = false;
    if (c.ackSeq > _ackLocal && c.ackSeq <= head) {
        advanceAck(c.ackSeq);
        freed = true;
    }
    if ((c.flags & kChallengeFlagDiag) && _d.diagGate) _d.diagGate->grant(c.diagMin);

    Response r;
    r.cs = c.cs;
    memcpy(r.nonce, c.nonce, sizeof(r.nonce));
    r.headSeq = _d.store.newestSeq();
    r.tailSeq = effectiveTail();
    r.bootId = _d.bootId;
    r.tMs = _d.clock.nowMs();  // lido imediatamente antes da tag: é a âncora de tempo
    uint8_t* value = &_tx[kTlvHeaderSize];
    encodeResponsePayload(r, value);
    t0 = _d.clock.nowUs();
    _d.handshakeAuth.tag(security::kTypeResponse, value, kResponsePayloadLen, value + kResponsePayloadLen);
    const uint32_t tagUs = sinceUs(t0);

    SendStats stats;
    sendTlv(static_cast<uint8_t>(MsgType::Response), kResponseLen, stats);
    _respondedCs = c.cs;
    _hasResponded = true;
    Event e("HS_RESPONSE");
    e.num("c_s", c.cs).num("tail", r.tailSeq).num("head", r.headSeq).num("boot", r.bootId).num("t_ms", r.tMs);
    if (freed) e.num("deleted_upto", c.ackSeq);
    else e.absent("deleted_upto");
    e.us("nvs_us", nvsUs).us("tag_us", tagUs).us("total_us", sinceUs(startUs) + queuedUs).us("q_us", queuedUs);
}

void SyncEngine::handleAck(const uint8_t* v, size_t len) {
    Ack a;
    if (!decodeAck(v, len, a)) {
        Event("HS_ACK").absent("ack").absent("c_conf").absent("flags").us("verify_us", 0).token("result", "bad_len")
            .flag("advanced", false).flag("opened", false).flag("anchor_ok", false);
        sendError(ErrorCode::InvalidTag);
        return;
    }
    const uint64_t t0 = _d.clock.nowUs();
    const bool tagOk = _d.handshakeAuth.verify(security::kTypeAck, v, kAckPayloadLen, a.tag);
    const uint32_t verifyUs = sinceUs(t0);
    auto emitAck = [&](const char* result, bool advanced, bool opened) {
        Event("HS_ACK").num("ack", a.ackSeq).num("c_conf", a.cConf).hex8("flags", a.flags).us("verify_us", verifyUs)
            .token("result", result).flag("advanced", advanced).flag("opened", opened)
            .flag("anchor_ok", (a.flags & kAckFlagAnchorFailed) == 0);
    };
    if (!tagOk) {
        emitAck("bad_tag", false, false);
        sendError(ErrorCode::InvalidTag);
        return;
    }
    if (a.ackSeq < _ackLocal || a.ackSeq > _d.store.newestSeq()) {
        emitAck("bad_seq", false, false);
        sendError(ErrorCode::InvalidAckSeq);
        return;
    }

    const bool advanced = a.ackSeq > _ackLocal;
    if (advanced) advanceAck(a.ackSeq);

    bool opened = false;
    if (_hasResponded && a.cConf == _respondedCs) {
        opened = true;
        _noHsConn = false;
        // Com flags_a bit 0 a âncora falhou: a sessão abre, mas o bit 1 fica ligado para repetir o handshake.
        if ((a.flags & kAckFlagAnchorFailed) == 0) _noHsBoot = false;
    }
    emitAck("ok", advanced, opened);
    if (opened) {
        setSession(true, "ack");
        sendNotice(NoticeReason::Ack);
    }
}

void SyncEngine::handleBatchRequest(const uint8_t* v, size_t len) {
    uint16_t k = 0;
    if (!decodeBatchRequest(v, len, k)) return;
    const uint32_t head = _d.store.newestSeq();
    if (!_sessionOpen) {
        Event("SYNC_REQ").num("k", k).num("cursor", _cursor).num("head", head).absent("end").token("result", "no_session");
        sendError(ErrorCode::NoSession);
        return;
    }
    if (k == 0 || _cursor > head) {
        Event("SYNC_REQ").num("k", k).num("cursor", _cursor).num("head", head).absent("end")
            .token("result", k == 0 ? "ignored" : "empty");
        return;
    }
    if (_batchActive) endBatch(BatchEnd::Superseded);
    const uint64_t end = static_cast<uint64_t>(_cursor) + k - 1;
    _batchEnd = end < head ? static_cast<uint32_t>(end) : head;
    _batchActive = true;
    _batchSent = _batchBytes = 0;
    _batchStartUs = _d.clock.nowUs();
    Event("SYNC_REQ").num("k", k).num("cursor", _cursor).num("head", head).num("end", _batchEnd).token("result", "ok");
}

void SyncEngine::handleAppError(const uint8_t* v, size_t len) {
    if (len != kErrorLen) return;
    Event("SYNC_ERR").token("dir", "rx").hex8("code", v[0]);
    if (v[0] != static_cast<uint8_t>(ErrorCode::Incomplete) &&
        v[0] != static_cast<uint8_t>(ErrorCode::ResendFromBoundary)) {
        return;
    }
    const uint32_t from = _cursor;
    if (_batchActive) endBatch(BatchEnd::AppError);
    _cursor = _ackLocal + 1;
    Event("SYNC_CURSOR").num("from", from).num("to", _cursor).token("reason", "app_error");
    // Informativo: atualiza HEAD/flags; a fronteira do lado do app é o 1º bloco com seq = ACK + 1.
    sendNotice(NoticeReason::Resync);
}

// ── Cursor e flash ───────────────────────────────────────────────────────

uint32_t SyncEngine::effectiveTail() {
    if (_d.store.empty()) return _d.store.newestSeq() + 1;
    const uint32_t oldest = _d.store.oldestSeq();
    // Um BOOT representa a faixa [from_seq, seq]: os seq da lacuna de boot nunca existiram.
    if (_d.store.read(oldest, _blk) == ReadStatus::Ok && collector::crcMatches(_blk)) {
        const MarkerInfo m = collector::parseMarker(_blk);
        if (m.kind == MarkerKind::Boot && m.fromSeq <= oldest) return m.fromSeq;
    }
    return oldest;
}

void SyncEngine::advanceAck(uint32_t ack, size_t* freedSegs) {
    _ackLocal = ack;
    const uint64_t t0 = _d.clock.nowUs();
    const size_t segs = _d.store.deleteUpTo(ack);
    if (freedSegs) *freedSegs = segs;
    Event("SYNC_FREE").num("upto", ack).num("segs", static_cast<int64_t>(segs)).num("oldest", _d.store.oldestSeq())
        .us("dur_us", sinceUs(t0));
    if (_cursor < ack + 1) {
        const uint32_t from = _cursor;
        _cursor = ack + 1;
        Event("SYNC_CURSOR").num("from", from).num("to", _cursor).token("reason", "ack");
    }
    if (_batchActive && _batchEnd < _cursor) endBatch(BatchEnd::Done);
}

void SyncEngine::checkOverflow() {
    const uint32_t up = _d.store.overflowUpTo();
    if (up <= _overflowSeen) return;
    _overflowSeen = up;
    if (up <= _ackLocal) return;  // só descartou o que o servidor já confirmou
    // Descartou bloco não confirmado: o servidor precisa saber (novo TAIL no próximo handshake).
    Event("SYNC_OVERFLOW").num("upto", up).num("ack", _ackLocal);
    if (_batchActive) endBatch(BatchEnd::Overflow);
    _noHsConn = true;
    setSession(false, "overflow");
    sendNotice(NoticeReason::Overflow);
}

void SyncEngine::endBatch(BatchEnd reason) {
    if (!_batchActive) return;
    _batchActive = false;
    Event("SYNC_BATCH_END").token("reason", batchEndName(static_cast<uint8_t>(reason))).num("sent", _batchSent)
        .num("bytes", _batchBytes).us("dur_us", sinceUs(_batchStartUs));
}

void SyncEngine::onFlushed(uint32_t) {
    checkOverflow();
    sendNotice(NoticeReason::Flush);
}

void SyncEngine::tick() {
    checkOverflow();
    const uint16_t usage = _d.store.usagePermille();
    const bool wasHigh = _flashHigh;
    if (usage > 800) _flashHigh = true;
    else if (usage < 780) _flashHigh = false;
    if (_flashHigh != wasHigh) sendNotice(NoticeReason::Flash);

    if (_connected && _subscribed && _noHsConn &&
        _d.clock.nowMs() - _lastNoticeMs >= kNoticeRetryMs) {
        sendNotice(NoticeReason::Retry);
    }
}

// ── Lote ─────────────────────────────────────────────────────────────────

uint32_t SyncEngine::findLossEnd(uint32_t s, uint32_t limit) {
    uint32_t to = s;
    const uint32_t head = _d.store.newestSeq();
    // `_loss` serve de rascunho aqui; o marcador é montado depois.
    while (to + 1 <= head && to + 1 - s < limit) {
        const ReadStatus st = _d.store.read(to + 1, _loss);
        const bool bad = st == ReadStatus::Corrupt || (st == ReadStatus::Ok && !collector::crcMatches(_loss));
        if (!bad) break;  // NotFound ou íntegro encerra a faixa
        ++to;
    }
    return to;
}

bool SyncEngine::sendLoss(uint32_t s, uint32_t to, const char* reason) {
    collector::buildLossMarker(_loss, s, to);
    const uint64_t t0 = _d.clock.nowUs();
    _loss.tagLength = static_cast<uint8_t>(_d.lossAuth.tagSize());
    const bool signed_ = _d.lossAuth.sign(_loss.bytes, _loss.length, _loss.tag);
    const uint32_t signUs = sinceUs(t0);
    if (!signed_) {
        // Não envia um LOSS sem tag válida: o cursor não avança e o marcador é remontado na próxima tentativa.
        Event("SIGN_FAIL").token("kind", "loss").num("seq", s).num("attempt", 1).num("rc", _d.lossAuth.lastError());
        return false;
    }
    Event("MARKER").token("kind", "loss").num("seq", s).num("from", s).num("to", to).token("reason", reason);

    uint8_t* value = &_tx[kTlvHeaderSize];
    memcpy(value, _loss.bytes, _loss.length);
    memcpy(value + _loss.length, _loss.tag, _loss.tagLength);
    const size_t valueLen = _loss.length + _loss.tagLength;
    SendStats stats;
    const bool ok = sendTlv(static_cast<uint8_t>(MsgType::Block), valueLen, stats);
    if (ok) {
        _cursor = to + 1;
        ++_batchSent;
        _batchBytes += static_cast<uint32_t>(kTlvHeaderSize + valueLen);
    }
    Event("SYNC_BLOCK").num("seq", s).token("kind", "loss").num("len", static_cast<int64_t>(_loss.length))
        .num("tag_len", _loss.tagLength).absent("read_us").absent("crc_us").absent("crc_ok").us("sign_us", signUs)
        .us("tx_us", stats.durUs).num("frags", stats.fragments).num("mtu", stats.mtu).num("cursor", _cursor);
    return ok;
}

bool SyncEngine::sendNextBlock() {
    if (!_batchActive) return false;
    if (!(_connected && _subscribed && _sessionOpen) || _cursor > _batchEnd) {
        endBatch(_cursor > _batchEnd ? BatchEnd::Done : BatchEnd::Link);
        return false;
    }
    checkOverflow();
    if (!_sessionOpen || !_batchActive) return false;

    const uint32_t s = _cursor;
    uint64_t t0 = _d.clock.nowUs();
    const ReadStatus st = _d.store.read(s, _blk);
    const uint32_t readUs = sinceUs(t0);

    if (st == ReadStatus::Ok) {
        t0 = _d.clock.nowUs();
        const bool crcOk = collector::crcMatches(_blk);
        const uint32_t crcUs = sinceUs(t0);
        if (!crcOk) {
            const uint32_t to = findLossEnd(s, kMaxLossRun);
            if (!sendLoss(s, to, "crc")) endBatch(BatchEnd::Link);
            return true;
        }
        const MarkerInfo marker = collector::parseMarker(_blk);
        uint8_t* value = &_tx[kTlvHeaderSize];
        memcpy(value, _blk.bytes, _blk.length);
        memcpy(value + _blk.length, _blk.tag, _blk.tagLength);
        const size_t valueLen = _blk.length + _blk.tagLength;
        SendStats stats;
        const bool ok = sendTlv(static_cast<uint8_t>(MsgType::Block), valueLen, stats);
        if (ok) {
            _cursor = s + 1;
            ++_batchSent;
            _batchBytes += static_cast<uint32_t>(kTlvHeaderSize + valueLen);
        }
        Event("SYNC_BLOCK").num("seq", s).token("kind", kindName(marker.kind))
            .num("len", static_cast<int64_t>(_blk.length)).num("tag_len", _blk.tagLength).us("read_us", readUs)
            .us("crc_us", crcUs).flag("crc_ok", true).absent("sign_us").us("tx_us", stats.durUs)
            .num("frags", stats.fragments).num("mtu", stats.mtu).num("cursor", _cursor);
        if (!ok) endBatch(BatchEnd::Link);
        return true;
    }

    if (st == ReadStatus::Corrupt) {
        const uint32_t to = findLossEnd(s, kMaxLossRun);
        if (!sendLoss(s, to, "crc")) endBatch(BatchEnd::Link);
        return true;
    }

    // NotFound: lacuna (BOOT) ou perda.
    const uint32_t t = _d.store.nextStoredSeq(s);
    if (t == 0) {
        endBatch(BatchEnd::Empty);
        return false;
    }
    if (_d.store.read(t, _loss) == ReadStatus::Ok && collector::crcMatches(_loss)) {
        const MarkerInfo m = collector::parseMarker(_loss);
        if (m.kind == MarkerKind::Boot && m.fromSeq <= s) {
            // O BOOT declara a lacuna: salta para ele e o envia no mesmo lote.
            Event("SYNC_CURSOR").num("from", s).num("to", t).token("reason", "boot_gap");
            _cursor = t;
            if (_batchEnd < t) _batchEnd = t;
            return true;
        }
    }
    uint32_t to = t - 1;
    if (to - s + 1 > kMaxLossRun) to = s + kMaxLossRun - 1;
    if (!sendLoss(s, to, "missing")) endBatch(BatchEnd::Link);
    return true;
}

// ── Bancada ──────────────────────────────────────────────────────────────

void SyncEngine::injectAck(uint32_t ackSeq) {
    uint8_t v[kAckLen];
    const uint32_t cConf = _hasResponded ? _respondedCs : _lastAcceptedCs;
    encodeAckPayload(ackSeq, cConf, 0, v);
    _d.handshakeAuth.tag(security::kTypeAck, v, kAckPayloadLen, v + kAckPayloadLen);
    handleAck(v, kAckLen);
}

}  // namespace datasync
