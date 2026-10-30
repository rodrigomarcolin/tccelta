#pragma once
#include <cstdint>

namespace collector {

/** Tempo desde o boot. */
class IClock {
public:
    virtual ~IClock() = default;
    virtual uint32_t nowMs() const = 0;
    /** Microssegundos desde o boot (durações `_us` dos eventos, spec fase-1/06 §1). */
    virtual uint64_t nowUs() const { return static_cast<uint64_t>(nowMs()) * 1000u; }
};

/** `a` ocorre em ou depois de `b`, tolerando a volta do contador de 32 bits. */
inline bool reached(uint32_t a, uint32_t b) { return static_cast<int32_t>(a - b) >= 0; }

/** Quanto falta de `now` até `target` (0 se já passou). */
inline uint32_t remaining(uint32_t now, uint32_t target) {
    return reached(now, target) ? 0 : target - now;
}

}  // namespace collector
