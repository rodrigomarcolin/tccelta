#include "collector/Collector.h"

namespace collector {

void Collector::applyConfig(const CollectorConfig& config) {
    closeBlock();
    _tmaxMs = config.tmaxMs;
    _scheduler.apply(config, _clock.nowMs());
}

void Collector::setRunning(bool running) {
    if (!running) closeBlock();
    _running = running;
}

void Collector::closeBlock() {
    if (!_builder.isOpen()) return;
    const bool empty = _builder.empty();
    Block* block = _builder.close();
    if (empty) {
        _pool.release(block);  // bloco vazio não é gravado
    } else {
        _pool.submit(block);
    }
}

void Collector::closeIfDue(uint32_t nowMs) {
    if (_builder.isOpen() && reached(nowMs, _builder.deadlineMs())) closeBlock();
}

uint32_t Collector::poll() {
    uint32_t now = _clock.nowMs();
    closeIfDue(now);
    if (!_running) return _builder.isOpen() ? remaining(now, _builder.deadlineMs()) : UINT32_MAX;

    const Scheduler::Decision decision = _scheduler.next(now, _reader.maxPidsPerRequest());
    if (!decision.read) {
        const uint32_t untilClose = _builder.isOpen() ? remaining(now, _builder.deadlineMs())
                                                      : UINT32_MAX;
        return decision.waitMs < untilClose ? decision.waitMs : untilClose;
    }

    ReadResult results[kMaxPidsPerRequest];
    _reader.read(decision.pids, decision.count, results, _tmaxMs);
    now = _clock.nowMs();
    _scheduler.onRead(decision, now);

    for (size_t i = 0; i < decision.count; ++i) {
        const ReadResult& result = results[i];
        if (!result.ok) continue;
        _values.update(result);
        append(result.tMs, [&](BlockBuilder& b) {
            return b.addPid(result.pid, result.value, result.len, result.tMs);
        });
    }
    closeIfDue(now);
    return 0;
}

void Collector::addGps(const GpsFix& fix, uint32_t tMs) {
    append(tMs, [&](BlockBuilder& b) { return b.addGps(fix, tMs); });
}

template <typename AddFn>
void Collector::append(uint32_t tMs, AddFn add) {
    for (int attempt = 0; attempt < 2; ++attempt) {
        if (!_builder.isOpen()) {
            Block* block = _pool.acquire();
            if (!block) break;
            _builder.open(*block, tMs);
        }
        if (add(_builder)) return;
        closeBlock();  // não coube: fecha e tenta num bloco novo
    }
    ++_droppedRecords;
}

}  // namespace collector
