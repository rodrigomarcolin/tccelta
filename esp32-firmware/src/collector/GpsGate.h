#pragma once
#include <cstdint>

namespace collector {

constexpr uint32_t kGpsMinIntervalMs = 1000;
// Tolerância para a variação de chegada pelo BLE: um app que manda a cada 1 s
// não pode ter metade das posições descartadas por chegarem com 990 ms.
constexpr uint32_t kGpsJitterMs = 100;

/**
 * Decide se uma posição GPS vinda do app é gravada: não no modo diagnóstico
 * e no máximo uma por segundo (pelo instante de chegada do comando).
 */
class GpsGate {
public:
    enum class Verdict : uint8_t { Accept, Diag, Rate };

    /** Ao aceitar, passa a contar o intervalo a partir de `tRxMs`. */
    Verdict check(bool diagMode, uint32_t tRxMs);

private:
    bool     _hasLast = false;
    uint32_t _lastMs  = 0;
};

const char* toString(GpsGate::Verdict verdict);

}  // namespace collector
