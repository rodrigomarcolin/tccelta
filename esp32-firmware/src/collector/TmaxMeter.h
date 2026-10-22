#pragma once
#include <cstddef>
#include <cstdint>
#include "collector/CollectorConfig.h"

namespace collector {

constexpr size_t kTmaxSamplesPerPid = 100;
constexpr double kTmaxMargin        = 1.3;

/** Resultado de uma medição, para o AT#TMAX?. */
struct TmaxReport {
    enum class State : uint8_t { Idle, Running, Done } state = State::Idle;
    size_t done  = 0;
    size_t total = 0;
    struct PidResult {
        uint8_t  pid;
        uint32_t p99Ms;      // 0 se nenhuma resposta
        uint32_t timeouts;
    } pids[kMaxPids] = {};
    size_t   pidCount = 0;
    uint32_t suggestedTmaxMs = 0;
};

/**
 * Medição de Tmax (plano §5.9): 100 requisições por PID, intercaladas entre
 * os PIDs; Tmax sugerido = 1,3 · maior p99, arredondado para cima em
 * múltiplos de 4 ms. Timeouts contam à parte, fora do percentil.
 */
class TmaxMeter {
public:
    void begin(const CollectorConfig& config);

    bool     finished() const { return _done >= total(); }
    size_t   total() const { return _pidCount * kTmaxSamplesPerPid; }
    uint8_t  nextPid() const { return _pids[_done % _pidCount]; }

    /** Registra o resultado da requisição de `nextPid()`. */
    void record(bool answered, uint32_t latencyMs);

    TmaxReport report() const;

private:
    uint8_t  _pids[kMaxPids] = {};
    size_t   _pidCount = 0;
    size_t   _done = 0;
    uint16_t _latencies[kMaxPids][kTmaxSamplesPerPid] = {};
    size_t   _answered[kMaxPids] = {};
    uint32_t _timeouts[kMaxPids] = {};

    uint32_t p99(size_t pidIndex) const;
};

}  // namespace collector
