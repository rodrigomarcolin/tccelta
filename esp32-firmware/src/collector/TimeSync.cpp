#include "collector/TimeSync.h"
#include "collector/Clock.h"

namespace collector {

TimeExchange TimeSync::start(uint16_t bootId, uint32_t nowMs) {
    _exchange = {bootId, nowMs, 0};
    _pending  = true;
    _done     = false;
    return _exchange;
}

bool TimeSync::accept(const TimeExchange& response, uint32_t nowMs) {
    if (!_pending || response.bootId != _exchange.bootId || response.t1Ms != _exchange.t1Ms) {
        return false;
    }
    _exchange.t2Ms = nowMs;
    _pending = false;
    _done    = true;
    return true;
}

bool TimeSync::timedOut(uint32_t nowMs) const {
    return _pending && reached(nowMs, _exchange.t1Ms + kTimeSyncTimeoutMs);
}

}  // namespace collector
