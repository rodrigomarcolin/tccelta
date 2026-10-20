#include "collector/Collector.h"
#include <cmath>
#include "collector/Events.h"
#include "collector/Utilization.h"

namespace collector {

namespace {

const char* toString(CloseReason why) {
    switch (why) {
        case CloseReason::Full:     return "full";
        case CloseReason::Deadline: return "deadline";
        case CloseReason::Config:   return "config";
        case CloseReason::Mode:     return "mode";
        case CloseReason::Stop:     return "stop";
    }
    return "-";
}

/** Valor de 2 bytes como gravado no bloco (PIDs de 1 byte completam com 00). */
void recordValue(const ReadResult& result, uint8_t out[2]) {
    out[0] = result.len > 0 ? result.value[0] : 0;
    out[1] = result.len > 1 ? result.value[1] : 0;
}

}  // namespace

void Collector::applyConfig(const CollectorConfig& config, ConfigReason reason) {
    closeBlock(CloseReason::Config);
    _tmaxMs = config.tmaxMs;
    _scheduler.apply(config, _clock.nowMs());
    emitConfig(config, reason);
}

void Collector::emitConfig(const CollectorConfig& config, ConfigReason reason) const {
    if (!kEventsEnabled) return;
    uint8_t pids[kMaxPids];
    for (size_t i = 0; i < config.pidCount; ++i) pids[i] = config.pids[i].pid;
    Event("CONFIG")
        .token("reason", reason == ConfigReason::Boot ? "boot" : "apply")
        .num("proto", static_cast<int>(config.protocol))
        .num("tmax", config.tmaxMs)
        .num("u", static_cast<int64_t>(std::lround(utilization(config) * 1000.0)))
        .hexList("pids", pids, config.pidCount);

    for (size_t i = 0; i < _scheduler.size(); ++i) {
        const Scheduler::Row row = _scheduler.row(i);
        CurrentValues::Value value;
        Event e("CVT");
        e.hex8("pid", row.pid).num("p", row.periodMs).num("pmin", row.minIntervalMs)
         .num("due", row.nextDueMs).num("last", row.lastReadMs);
        if (_values.get(row.pid, value)) e.hex("val", value.bytes, value.len);
        else                             e.absent("val");
    }
}

void Collector::setRunning(bool running, CloseReason why) {
    if (!running) closeBlock(why);
    _running = running;
}

void Collector::closeBlock(CloseReason why) {
    if (!_builder.isOpen()) return;
    const bool empty = _builder.empty();
    const uint32_t ageMs = _clock.nowMs() - _builder.startMs();
    Block* block = _builder.close();
    const int buf = _pool.indexOf(block);
    _openBlock = nullptr;

    if (empty) {
        Event("BLK_RELEASE").num("buf", buf);
        _pool.release(block);  // bloco vazio não é gravado
        return;
    }
    Event("BLK_CLOSE").num("buf", buf).token("reason", toString(why))
        .num("len", static_cast<int64_t>(block->length)).num("recs", _openRecords).num("age", ageMs);
    _pool.submit(block);
}

void Collector::closeIfDue(uint32_t nowMs) {
    if (_builder.isOpen() && reached(nowMs, _builder.deadlineMs())) closeBlock(CloseReason::Deadline);
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

    {
        Event e("DECIDE");
        e.token("rule", decision.early ? "early" : "due").hexList("pids", decision.pids, decision.count);
        if (decision.early) e.absent("late").num("slack", decision.slackMs);
        else                e.num("late", decision.lateMs).absent("slack");
    }

    ReadResult results[kMaxPidsPerRequest];
    _reader.read(decision.pids, decision.count, results, _tmaxMs);
    now = results[decision.count - 1].tMs;  // fim da leitura: vira o `last` dos PIDs lidos
    _scheduler.onRead(decision, now);

    for (size_t i = 0; i < decision.count; ++i) {
        const ReadResult& result = results[i];
        {
            Event e("READ");
            e.at(now).hex8("pid", result.pid).flag("ok", result.ok);
            if (result.ok) e.hex("val", result.value, result.len);
            else           e.absent("val");
            e.num("start", result.startMs).num("dur", now - result.startMs)
             .flag("early", decision.early).num("next_due", _scheduler.nextDue(result.pid));
        }
        if (!result.ok) continue;

        _values.update(result);
        const bool stored = append(result.tMs, [&](BlockBuilder& b) {
            return b.addPid(result.pid, result.value, result.len, result.tMs);
        });
        uint8_t value[2];
        recordValue(result, value);
        if (stored) {
            Event("REC").token("kind", "pid").hex8("pid", result.pid)
                .num("dt", result.tMs - _builder.startMs())
                .hex("val", value, 2).absent("lat").absent("lon").absent("alt")
                .num("len", static_cast<int64_t>(_openBlock->length));
        } else {
            Event("REC_DROP").token("kind", "pid").hex8("pid", result.pid).token("reason", "no_buffer");
        }
    }
    closeIfDue(now);
    return 0;
}

void Collector::addGps(const GpsFix& fix, uint32_t tMs) {
    const bool stored = append(tMs, [&](BlockBuilder& b) { return b.addGps(fix, tMs); });
    if (stored) {
        const int32_t dt = static_cast<int32_t>(tMs - _builder.startMs());
        Event("REC").token("kind", "gps").absent("pid").num("dt", dt)
            .absent("val").num("lat", fix.latE7).num("lon", fix.lonE7).num("alt", fix.altMm)
            .num("len", static_cast<int64_t>(_openBlock->length));
    } else {
        Event("REC_DROP").token("kind", "gps").absent("pid").token("reason", "no_buffer");
    }
}

template <typename AddFn>
bool Collector::append(uint32_t tMs, AddFn add) {
    for (int attempt = 0; attempt < 2; ++attempt) {
        if (!_builder.isOpen()) {
            Block* block = _pool.acquire();
            if (!block) break;
            _builder.open(*block, tMs);
            _openBlock   = block;
            _openRecords = 0;
            Event("BLK_OPEN").num("buf", _pool.indexOf(block)).num("t0", tMs);
        }
        if (add(_builder)) {
            ++_openRecords;
            return true;
        }
        closeBlock(CloseReason::Full);  // não coube: fecha e tenta num bloco novo
    }
    ++_droppedRecords;
    return false;
}

}  // namespace collector
