#pragma once
#include <cstddef>
#include <cstdint>

#include "collector/BlockFormat.h"
#include "collector/IBlockStore.h"

namespace collector {

/**
 * Arquivo de segmento v2 (spec fase-1/02 §5–§6):
 *   cabeçalho (8 B): "TCS2" | versão u8 (=2) | tag_len u8 | reservado u16
 *   registros: dados(tamanho) | tag(tag_len) | CRC32 u32 LE de (dados ‖ tag)
 * Invariantes: seq contíguo dentro do segmento; um único tag_len; até 64 registros.
 */
constexpr size_t   kSegmentHeaderSize = 8;
constexpr uint8_t  kSegmentVersion = 2;
constexpr uint16_t kBlocksPerSegment = 64;

void encodeSegmentHeader(uint8_t tagLen, uint8_t out[kSegmentHeaderSize]);
/** Confere magia, versão e tag_len ∈ {0, 32, 64}. */
bool decodeSegmentHeader(const uint8_t in[kSegmentHeaderSize], uint8_t& tagLen);

constexpr size_t recordSize(size_t length, size_t tagLen) { return length + tagLen + kCrcSize; }
uint32_t computeCrc(const Block& b);  // crc32(dados ‖ tag)
bool     crcMatches(const Block& b);  // computeCrc(b) == b.crc

/** Fonte de bytes sequencial (File no ESP32, buffer em memória no native). */
class IByteSource {
public:
    virtual ~IByteSource() = default;
    virtual size_t read(uint8_t* out, size_t n) = 0;
    virtual bool   skip(size_t n) = 0;
    virtual size_t remaining() const = 0;
};

struct ScanResult {
    uint32_t firstSeq, lastSeq;
    uint16_t count;
    uint32_t bytes;  // cabeçalho do segmento + registros válidos
    uint8_t  tagLen;
    bool     trailingGarbage;  // a varredura parou antes do fim da fonte
};

/**
 * Varre um segmento já posicionado no início do arquivo (lê o cabeçalho).
 * Para no 1º registro com tamanho fora de [12, 1024], seq ≠ firstSeq + k, ou
 * bytes insuficientes para dados + tag + crc. false = cabeçalho inválido.
 */
bool scanSegment(IByteSource& src, uint32_t expectedFirstSeq, ScanResult& out);

/**
 * Lê o registro `seq` (varrendo desde o início da fonte, que inclui o
 * cabeçalho do segmento). O CRC não é conferido.
 */
ReadStatus readRecord(IByteSource& src, uint32_t firstSeq, uint32_t seq, Block& out);

}  // namespace collector
