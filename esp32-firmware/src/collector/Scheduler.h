#pragma once
#include <cstddef>
#include <cstdint>
#include "collector/CollectorConfig.h"
#include "collector/PidReader.h"

namespace collector {

/**
 * Escalonador da CVT: decide qual PID ler e quando (plano §5.3).
 *
 * 1. Há PIDs vencidos → lê o vencido há mais tempo (e outros vencidos, até
 *    `maxPerRead`).
 * 2. Senão, se o próximo vencimento está a mais de Tmax → lê antecipadamente
 *    o de vencimento mais próximo entre os que já cumpriram Pᵢᵐⁱⁿ.
 * 3. Senão, espera.
 *
 * Pertence só à tarefa de Aquisição; não é thread-safe.
 */
class Scheduler {
public:
    struct Decision {
        bool     read;                       // false = esperar `waitMs`
        bool     early;                      // leitura antecipada
        uint8_t  pids[kMaxPidsPerRequest];
        size_t   count;
        uint32_t waitMs;
        uint32_t lateMs;    // leitura vencida: atraso em relação ao vencimento
        uint32_t slackMs;   // leitura antecipada: folga até o próximo vencimento
    };

    /** Uma linha da tabela, para eventos de depuração. */
    struct Row {
        uint8_t  pid;
        uint32_t periodMs;
        uint32_t minIntervalMs;
        uint32_t nextDueMs;
        uint32_t lastReadMs;
    };

    /** Recarrega os PIDs da config; todos vencem imediatamente. */
    void apply(const CollectorConfig& config, uint32_t nowMs);

    Decision next(uint32_t nowMs, size_t maxPerRead) const;

    /** Reagenda os PIDs lidos (com ou sem resposta). */
    void onRead(const Decision& decision, uint32_t nowMs);

    size_t size() const { return _count; }
    Row row(size_t i) const;
    uint32_t nextDue(uint8_t pid) const;

private:
    struct Entry {
        uint8_t  pid;
        uint32_t periodMs;
        uint32_t minIntervalMs;
        uint32_t nextDueMs;
        uint32_t lastReadMs;
    };

    Entry    _entries[kMaxPids] = {};
    size_t   _count  = 0;
    uint32_t _tmaxMs = kDefaultTmaxMs;

    enum class Filter : uint8_t { Overdue, EarlyEligible };

    bool canReadEarly(const Entry& entry, uint32_t nowMs) const;
    void pick(Decision& decision, bool* picked, size_t maxPerRead, Filter filter,
              uint32_t nowMs) const;
    Entry* find(uint8_t pid);
    const Entry* find(uint8_t pid) const;
};

}  // namespace collector
