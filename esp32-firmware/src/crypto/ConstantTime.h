#pragma once
#include <cstddef>
#include <cstdint>

namespace crypto {

/** Comparação em tempo constante (não para no primeiro byte diferente). */
inline bool ctEqual(const uint8_t* a, const uint8_t* b, size_t len) {
    const volatile uint8_t* va = a;
    const volatile uint8_t* vb = b;
    uint8_t diff = 0;
    for (size_t i = 0; i < len; i++) diff |= static_cast<uint8_t>(va[i] ^ vb[i]);
    return diff == 0;
}

/** Zera memória sem que o compilador remova a escrita. */
inline void secureZero(void* p, size_t len) {
    volatile uint8_t* v = static_cast<volatile uint8_t*>(p);
    for (size_t i = 0; i < len; i++) v[i] = 0;
}

}  // namespace crypto
