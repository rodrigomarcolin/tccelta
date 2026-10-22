#include "collector/UploadCursor.h"
#include "collector/Clock.h"

namespace collector {

void UploadCursor::resync(uint32_t oldestSeq) {
    if (oldestSeq == 0) return;
    if (_ackSeq + 1 < oldestSeq) _ackSeq = oldestSeq - 1;
    if (_nextSeq < oldestSeq) _nextSeq = oldestSeq;
}

bool UploadCursor::canSend(uint32_t newestSeq) const {
    return _nextSeq <= newestSeq && inFlight() < _window;
}

uint32_t UploadCursor::takeNext(uint32_t nowMs) {
    if (inFlight() == 0) _lastProgressMs = nowMs;
    return _nextSeq++;
}

bool UploadCursor::onAck(uint32_t seq, uint32_t nowMs) {
    if (seq <= _ackSeq) return false;
    // O servidor pode confirmar além do que foi enviado nesta conexão (já
    // tinha os blocos de antes de um reboot).
    _ackSeq = seq;
    if (_nextSeq <= seq) _nextSeq = seq + 1;
    _lastProgressMs = nowMs;
    return true;
}

bool UploadCursor::timedOut(uint32_t nowMs) const {
    return inFlight() > 0 && reached(nowMs, _lastProgressMs + _ackTimeoutMs);
}

}  // namespace collector
