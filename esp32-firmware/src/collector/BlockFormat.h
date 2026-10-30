#pragma once
#include <cstddef>
#include <cstdint>

/**
 * Formato em blocos (§2.6.4 da Investigação). Inteiros em little-endian.
 *
 *   Cabeçalho (12 B): seq u32 | boot_id u16 | t_ms inicial u32 | tamanho u16
 *   Leitura de PID (5 B): tipo u8 (0x00–0xFC = PID) | Δt u16 | valor 2 B
 *   Marcador (11 B):      tipo u8 (0xFD = LOSS, 0xFE = BOOT) | Δt u16 | from_seq u32 | to_seq u32
 *   Posição GPS (15 B):   tipo u8 (= 0xFF) | Δt u16 | lat i32 | lon i32 | alt i32
 *
 * `tamanho` é o total de bytes da área de dados, cabeçalho incluído. Δt é
 * contado a partir de `t_ms inicial` e não tem sinal (spec fase-1/02 §2): um
 * registro anterior ao `t_ms inicial` não cabe no bloco aberto e abre outro.
 * lat/lon em graus × 10⁷, alt em milímetros.
 *
 * Na flash o registro é `dados(tamanho) | tag | CRC32(dados ‖ tag)` (spec 02 §5);
 * a tag é calculada por `IMessageAuth` sobre os `tamanho` bytes.
 */
namespace collector {

constexpr size_t   kBlockSize        = 1024;
constexpr size_t   kHeaderSize       = 12;
constexpr size_t   kPidRecordSize    = 5;
constexpr size_t   kGpsRecordSize    = 15;
constexpr uint8_t  kGpsRecordType    = 0xFF;
constexpr uint8_t  kPidRecordTypeMax = 0xFC;  // 0xFD/0xFE são marcadores, 0xFF é GPS
constexpr size_t   kMaxTagSize       = 64;    // maior tag entre os perfis (ECDSA r‖s)
constexpr size_t   kCrcSize          = 4;

namespace header {
constexpr size_t kSeq     = 0;
constexpr size_t kBootId  = 4;
constexpr size_t kStartMs = 6;
constexpr size_t kLength  = 10;
}  // namespace header

struct Block {
    uint8_t  bytes[kBlockSize];   // área de dados
    size_t   length;              // = tamanho (cabeçalho incluído)
    uint8_t  tag[kMaxTagSize];    // só os primeiros tagLength bytes valem
    uint8_t  tagLength;           // 0, 32 ou 64
    uint32_t crc;                 // CRC32(bytes[0..length) ‖ tag[0..tagLength))
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

/** Preenche `seq` e `boot_id` — feito pela Flush, logo antes de assinar e gravar. */
inline void stampHeader(Block& block, uint32_t seq, uint16_t bootId) {
    putU32(&block.bytes[header::kSeq], seq);
    putU16(&block.bytes[header::kBootId], bootId);
}

}  // namespace collector
