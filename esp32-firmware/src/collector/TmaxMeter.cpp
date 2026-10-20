#include "collector/TmaxMeter.h"
#include <algorithm>
#include <cmath>

namespace collector {

void TmaxMeter::begin(const CollectorConfig& config) {
    _pidCount = config.pidCount;
    for (size_t i = 0; i < _pidCount; ++i) {
        _pids[i]     = config.pids[i].pid;
        _answered[i] = 0;
        _timeouts[i] = 0;
    }
    _done = 0;
}

void TmaxMeter::record(bool answered, uint32_t latencyMs) {
    if (finished()) return;
    const size_t index = _done % _pidCount;
    if (answered) {
        _latencies[index][_answered[index]++] =
            static_cast<uint16_t>(latencyMs > UINT16_MAX ? UINT16_MAX : latencyMs);
    } else {
        ++_timeouts[index];
    }
    ++_done;
}

uint32_t TmaxMeter::p99(size_t pidIndex) const {
    const size_t n = _answered[pidIndex];
    if (n == 0) return 0;
    uint16_t sorted[kTmaxSamplesPerPid];
    std::copy(_latencies[pidIndex], _latencies[pidIndex] + n, sorted);
    std::sort(sorted, sorted + n);
    // Percentil por posto mais próximo: posto = ⌈0,99 · n⌉.
    const size_t rank = static_cast<size_t>(std::ceil(0.99 * static_cast<double>(n)));
    return sorted[rank - 1];
}

TmaxReport TmaxMeter::report() const {
    TmaxReport report;
    report.state    = finished() ? TmaxReport::State::Done : TmaxReport::State::Running;
    report.done     = _done;
    report.total    = total();
    report.pidCount = _pidCount;

    uint32_t worst = 0;
    for (size_t i = 0; i < _pidCount; ++i) {
        const uint32_t value = p99(i);
        report.pids[i] = {_pids[i], value, _timeouts[i]};
        if (value > worst) worst = value;
    }

    uint32_t suggested = static_cast<uint32_t>(std::ceil(kTmaxMargin * worst));
    suggested = (suggested + kTmaxStepMs - 1) / kTmaxStepMs * kTmaxStepMs;
    report.suggestedTmaxMs = suggested < kMinTmaxMs ? kMinTmaxMs : suggested;
    return report;
}

}  // namespace collector
