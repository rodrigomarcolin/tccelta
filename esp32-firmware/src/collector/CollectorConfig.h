#pragma once
#include <cstddef>
#include <cstdint>

/**
 * Parâmetros configuráveis do Dongle (Tabela 6 da Investigação) e as
 * constantes de projeto que os limitam (Tabela 7 + pisos do plano).
 */
namespace collector {

constexpr size_t   kMaxPids         = 16;     // capacidade da CVT
constexpr uint32_t kDefaultTmaxMs   = 50;
constexpr uint32_t kMinTmaxMs       = 20;     // piso do AT ST
constexpr uint32_t kTmaxStepMs      = 4;      // AT ST hh → hh × 4 ms
constexpr uint32_t kMinPeriodMs     = 100;    // piso de Pᵢ e Pᵢᵐⁱⁿ
constexpr double   kMaxUtilization  = 0.8;    // Umax

// Prazo de fechamento de um bloco (T_flush). O teto vem do Δt dos registros
// (u16, 65,5 s), com folga para uma leitura em andamento.
constexpr uint32_t kDefaultFlushIntervalMs = 10000;
constexpr uint32_t kMinFlushIntervalMs     = 1000;
constexpr uint32_t kMaxFlushIntervalMs     = 30000;

/** Protocolos aceitos pelo AT SP (ISO 15765-4, Tabela 9). */
enum class CanProtocol : uint8_t {
    Id11Kbps500 = 6,
    Id29Kbps500 = 7,
    Id11Kbps250 = 8,
    Id29Kbps250 = 9,
};

bool isValidProtocol(uint8_t number);
bool usesExtendedIds(CanProtocol protocol);
bool uses500Kbps(CanProtocol protocol);
const char* describe(CanProtocol protocol);   // texto do AT DP

struct PidSetting {
    uint8_t  pid;
    uint32_t periodMs;       // Pᵢ: intervalo máximo desejado entre leituras
    uint32_t minIntervalMs;  // Pᵢᵐⁱⁿ: intervalo mínimo para leitura antecipada
};

struct CollectorConfig {
    CanProtocol protocol = CanProtocol::Id11Kbps500;
    uint32_t    tmaxMs   = kDefaultTmaxMs;
    uint32_t    flushIntervalMs = kDefaultFlushIntervalMs;
    PidSetting  pids[kMaxPids] = {};
    size_t      pidCount = 0;

    /** Os 7 PIDs e períodos da Tabela 10, com Pᵢᵐⁱⁿ da Tabela 13. */
    static CollectorConfig defaults();

    const PidSetting* find(uint8_t pid) const;

    /** Adiciona ou altera um PID. `minIntervalMs` padrão: max(Pᵢ/2, 100 ms). */
    bool setPid(uint8_t pid, uint32_t periodMs);  // false se cheio ou PID reservado (> 0xFC)
    bool removePid(uint8_t pid);
};

enum class ConfigError : uint8_t {
    None,
    InvalidProtocol,
    TmaxTooSmall,
    TooManyPids,
    PeriodTooSmall,
    Infeasible,       // U > Umax
    FlushOutOfRange,  // prazo de fechamento fora de [1 s, 30 s]
    Busy,             // Aquisição recusou (ex.: medindo Tmax)
    InvalidPid,       // PID ≥ 0xFD colide com marcador/GPS (spec fase-1/02 §2)
};

/** Confere pisos e viabilidade (U ≤ Umax com Tᵢ = Tmax). */
ConfigError validate(const CollectorConfig& config);

}  // namespace collector
