#pragma once
#include <cstdint>

namespace pairing {

/** Diferença `now - since` com sinal: segura na virada do `millis()` (~49 dias). */
inline int32_t elapsedMs(uint32_t now, uint32_t since) { return static_cast<int32_t>(now - since); }

/** `now` já alcançou `target`? */
inline bool reached(uint32_t now, uint32_t target) { return elapsedMs(now, target) >= 0; }

}  // namespace pairing
