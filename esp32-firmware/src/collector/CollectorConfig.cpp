#include "collector/CollectorConfig.h"
#include "collector/BlockFormat.h"
#include "collector/Utilization.h"

namespace collector {

bool isValidProtocol(uint8_t number) { return number >= 6 && number <= 9; }

bool usesExtendedIds(CanProtocol protocol) {
    return protocol == CanProtocol::Id29Kbps500 || protocol == CanProtocol::Id29Kbps250;
}

bool uses500Kbps(CanProtocol protocol) {
    return protocol == CanProtocol::Id11Kbps500 || protocol == CanProtocol::Id29Kbps500;
}

const char* describe(CanProtocol protocol) {
    switch (protocol) {
        case CanProtocol::Id11Kbps500: return "ISO 15765-4 (CAN 11/500)";
        case CanProtocol::Id29Kbps500: return "ISO 15765-4 (CAN 29/500)";
        case CanProtocol::Id11Kbps250: return "ISO 15765-4 (CAN 11/250)";
        case CanProtocol::Id29Kbps250: return "ISO 15765-4 (CAN 29/250)";
    }
    return "?";
}

static uint32_t defaultMinInterval(uint32_t periodMs) {
    const uint32_t half = periodMs / 2;
    return half > kMinPeriodMs ? half : kMinPeriodMs;
}

CollectorConfig CollectorConfig::defaults() {
    // Pᵢ = kᵢ · Tmax (Tabela 10). PIDs que mudam devagar não leem antecipado.
    struct Default { uint8_t pid; uint32_t k; bool early; };
    static const Default kTable[] = {
        {0x0C, 5, true},    // rotação
        {0x0D, 5, true},    // velocidade
        {0x49, 5, true},    // pedal
        {0x04, 12, true},   // carga
        {0x10, 12, true},   // MAF
        {0x05, 100, false}, // temperatura do líquido
        {0x2F, 200, false}, // combustível
    };

    CollectorConfig config;
    for (const Default& d : kTable) {
        uint32_t period = d.k * config.tmaxMs;
        if (period < kMinPeriodMs) period = kMinPeriodMs;
        config.pids[config.pidCount++] = {d.pid, period, d.early ? defaultMinInterval(period) : period};
    }
    return config;
}

const PidSetting* CollectorConfig::find(uint8_t pid) const {
    for (size_t i = 0; i < pidCount; ++i) {
        if (pids[i].pid == pid) return &pids[i];
    }
    return nullptr;
}

bool CollectorConfig::setPid(uint8_t pid, uint32_t periodMs) {
    if (pid > kPidRecordTypeMax) return false;
    const PidSetting setting = {pid, periodMs, defaultMinInterval(periodMs)};
    for (size_t i = 0; i < pidCount; ++i) {
        if (pids[i].pid == pid) {
            pids[i] = setting;
            return true;
        }
    }
    if (pidCount >= kMaxPids) return false;
    pids[pidCount++] = setting;
    return true;
}

bool CollectorConfig::removePid(uint8_t pid) {
    for (size_t i = 0; i < pidCount; ++i) {
        if (pids[i].pid != pid) continue;
        for (size_t j = i + 1; j < pidCount; ++j) pids[j - 1] = pids[j];
        --pidCount;
        return true;
    }
    return false;
}

ConfigError validate(const CollectorConfig& config) {
    if (!isValidProtocol(static_cast<uint8_t>(config.protocol))) return ConfigError::InvalidProtocol;
    if (config.tmaxMs < kMinTmaxMs) return ConfigError::TmaxTooSmall;
    if (config.pidCount > kMaxPids) return ConfigError::TooManyPids;
    if (config.flushIntervalMs < kMinFlushIntervalMs || config.flushIntervalMs > kMaxFlushIntervalMs) {
        return ConfigError::FlushOutOfRange;
    }
    for (size_t i = 0; i < config.pidCount; ++i) {
        if (config.pids[i].pid > kPidRecordTypeMax) return ConfigError::InvalidPid;
        if (config.pids[i].periodMs < kMinPeriodMs ||
            config.pids[i].minIntervalMs < kMinPeriodMs) {
            return ConfigError::PeriodTooSmall;
        }
    }
    if (!isFeasible(config)) return ConfigError::Infeasible;
    return ConfigError::None;
}

}  // namespace collector
