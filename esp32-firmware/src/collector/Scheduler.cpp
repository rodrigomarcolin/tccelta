#include "collector/Scheduler.h"
#include "collector/Clock.h"

namespace collector {

namespace {

constexpr uint32_t kIdleWaitMs = 1000;  // sem PIDs configurados

}  // namespace

void Scheduler::apply(const CollectorConfig& config, uint32_t nowMs) {
    _tmaxMs = config.tmaxMs;
    _count  = config.pidCount;
    for (size_t i = 0; i < _count; ++i) {
        const PidSetting& setting = config.pids[i];
        _entries[i] = {setting.pid, setting.periodMs, setting.minIntervalMs,
                       nowMs, nowMs - setting.minIntervalMs};
    }
}

bool Scheduler::canReadEarly(const Entry& entry, uint32_t nowMs) const {
    return reached(nowMs, entry.lastReadMs + entry.minIntervalMs);
}

Scheduler::Decision Scheduler::next(uint32_t nowMs, size_t maxPerRead) const {
    Decision decision = {};
    if (_count == 0) {
        decision.waitMs = kIdleWaitMs;
        return decision;
    }
    if (maxPerRead > kMaxPidsPerRequest) maxPerRead = kMaxPidsPerRequest;

    bool picked[kMaxPids] = {};

    // Regra 1: vencidos, do mais atrasado para o menos.
    pick(decision, picked, maxPerRead, Filter::Overdue, nowMs);
    if (decision.count > 0) {
        decision.read = true;
        decision.lateMs = nowMs - find(decision.pids[0])->nextDueMs;  // o mais atrasado
        return decision;
    }

    uint32_t earliestDue = _entries[0].nextDueMs;
    for (size_t i = 1; i < _count; ++i) {
        if (!reached(_entries[i].nextDueMs, earliestDue)) earliestDue = _entries[i].nextDueMs;
    }

    // Regra 2: antecipa só com folga maior que Tmax até o próximo vencimento,
    // para nunca atrasar uma leitura agendada.
    const uint32_t untilDue = remaining(nowMs, earliestDue);
    if (untilDue > _tmaxMs) {
        pick(decision, picked, maxPerRead, Filter::EarlyEligible, nowMs);
        if (decision.count > 0) {
            decision.read    = true;
            decision.early   = true;
            decision.slackMs = untilDue;
            return decision;
        }
    }

    // Regra 3: espera o próximo vencimento ou a próxima antecipação possível.
    uint32_t wait = untilDue;
    for (size_t i = 0; i < _count; ++i) {
        const uint32_t eligibleIn = remaining(nowMs, _entries[i].lastReadMs + _entries[i].minIntervalMs);
        if (eligibleIn < wait && untilDue - eligibleIn > _tmaxMs) wait = eligibleIn;
    }
    decision.waitMs = wait;
    return decision;
}

void Scheduler::pick(Decision& decision, bool* picked, size_t maxPerRead, Filter filter,
                     uint32_t nowMs) const {
    // Acrescenta, em ordem de vencimento, até `maxPerRead` entradas que passam no filtro.
    while (decision.count < maxPerRead) {
        int best = -1;
        for (size_t i = 0; i < _count; ++i) {
            const Entry& e = _entries[i];
            const bool eligible = filter == Filter::Overdue ? reached(nowMs, e.nextDueMs)
                                                            : canReadEarly(e, nowMs);
            if (picked[i] || !eligible) continue;
            if (best < 0 || !reached(e.nextDueMs, _entries[best].nextDueMs)) best = static_cast<int>(i);
        }
        if (best < 0) return;
        picked[best] = true;
        decision.pids[decision.count++] = _entries[best].pid;
    }
}

void Scheduler::onRead(const Decision& decision, uint32_t nowMs) {
    for (size_t i = 0; i < decision.count; ++i) {
        Entry* entry = find(decision.pids[i]);
        if (!entry) continue;
        entry->lastReadMs = nowMs;
        if (decision.early) {
            entry->nextDueMs = nowMs + entry->periodMs;
        } else {
            // Mantém a fase: PIDs de mesmo período continuam lidos em sequência.
            entry->nextDueMs += entry->periodMs;
            if (reached(nowMs, entry->nextDueMs)) entry->nextDueMs = nowMs;
        }
    }
}

Scheduler::Entry* Scheduler::find(uint8_t pid) {
    for (size_t i = 0; i < _count; ++i) {
        if (_entries[i].pid == pid) return &_entries[i];
    }
    return nullptr;
}

const Scheduler::Entry* Scheduler::find(uint8_t pid) const {
    return const_cast<Scheduler*>(this)->find(pid);
}

Scheduler::Row Scheduler::row(size_t i) const {
    const Entry& e = _entries[i];
    return {e.pid, e.periodMs, e.minIntervalMs, e.nextDueMs, e.lastReadMs};
}

uint32_t Scheduler::nextDue(uint8_t pid) const {
    const Entry* entry = find(pid);
    return entry ? entry->nextDueMs : 0;
}

}  // namespace collector
