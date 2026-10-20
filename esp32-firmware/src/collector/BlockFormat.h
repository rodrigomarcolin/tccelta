#pragma once
#include <cstddef>
#include <cstdint>

/**
 * Formato em blocos (§2.6.4 da Investigação). Inteiros em little-endian.
 *
 *   Cabeçalho (12 B): seq u32 | boot_id u16 | t_ms inicial u32 | tamanho u16
 *   Leitura de PID (5 B): tipo u8 (= PID) | Δt i16 | valor 2 B
 *   Posição GPS (15 B):   tipo u8 (= 0xFF) | Δt i16 | lat i32 | lon i32 | alt i32
 *
 * `tamanho` é o total de bytes do bloco, cabeçalho incluído. Δt é contado a
 * partir de `t_ms inicial` e tem sinal: uma posição GPS marcada ao chegar o
 * comando pode ser anterior ao primeiro registro do bloco. lat/lon em
 * graus × 10⁷, alt em milímetros.
 */
namespace collector {

constexpr size_t   kBlockSize        = 1024;
constexpr size_t   kHeaderSize       = 12;
constexpr size_t   kPidRecordSize    = 5;
constexpr size_t   kGpsRecordSize    = 15;
constexpr uint8_t  kGpsRecordType    = 0xFF;

namespace header {
constexpr size_t kSeq     = 0;
constexpr size_t kBootId  = 4;
constexpr size_t kStartMs = 6;
constexpr size_t kLength  = 10;
}  // namespace header

struct Block {
    uint8_t bytes[kBlockSize];
    size_t  length;  // bytes usados, cabeçalho incluído
};

struct GpsFix {
    int32_t latE7;
    int32_t lonE7;
    int32_t altMm;
};

inline void putU16(uint8_t* p, uint16_t v) {
    p[0] = static_cast<uint8_t>(v);
    p[1] = static_cast<uint8_t>(v >> 8);
}

inline void putU32(uint8_t* p, uint32_t v) {
    for (int i = 0; i < 4; ++i) p[i] = static_cast<uint8_t>(v >> (8 * i));
}

inline uint16_t getU16(const uint8_t* p) {
    return static_cast<uint16_t>(p[0] | (p[1] << 8));
}

inline uint32_t getU32(const uint8_t* p) {
    return static_cast<uint32_t>(p[0]) | (static_cast<uint32_t>(p[1]) << 8) |
           (static_cast<uint32_t>(p[2]) << 16) | (static_cast<uint32_t>(p[3]) << 24);
}

inline uint32_t blockSeq(const Block& block) { return getU32(&block.bytes[header::kSeq]); }

/** Preenche `seq` e `boot_id` — feito pela Flush, logo antes de gravar. */
inline void stampHeader(Block& block, uint32_t seq, uint16_t bootId) {
    putU32(&block.bytes[header::kSeq], seq);
    putU16(&block.bytes[header::kBootId], bootId);
}

}  // namespace collector
