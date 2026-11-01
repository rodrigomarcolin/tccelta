#pragma once
#include <algorithm>
#include <cstddef>
#include <cstdint>
#include "collector/BlockFormat.h"
#include "collector/SegmentFormat.h"

// Margem de segurança do log, em % do LittleFS (platformio.ini, [block_log]).
#ifndef BLOCK_LOG_MARGIN_PCT
#define BLOCK_LOG_MARGIN_PCT 10
#endif
static_assert(BLOCK_LOG_MARGIN_PCT >= 0 && BLOCK_LOG_MARGIN_PCT < 100,
              "BLOCK_LOG_MARGIN_PCT deve estar em [0, 100)");

namespace collector {

/** Pior caso de bytes por bloco: registro cheio + cabeçalho de segmento rateado (para cima). */
constexpr size_t maxRecordBytes(size_t tagLen) {
    return recordSize(kBlockSize, tagLen) + (kSegmentHeaderSize + kBlocksPerSegment - 1) / kBlocksPerSegment;
}

/**
 * Limite N de blocos no log: (total − margem%) / registro máximo, preso a
 * [2 segmentos, ceiling]. O piso existe porque o descarte é por segmento
 * inteiro e o segmento aberto nunca é apagado; o teto é o que o índice em RAM comporta.
 */
inline uint32_t maxBlocksFor(size_t totalBytes, unsigned marginPct, size_t tagLen, uint32_t ceiling) {
    const size_t usable = totalBytes - totalBytes * marginPct / 100;
    const uint32_t n = static_cast<uint32_t>(std::min<size_t>(usable / maxRecordBytes(tagLen), UINT32_MAX));
    return std::max<uint32_t>(2u * kBlocksPerSegment, std::min(n, ceiling));
}

}  // namespace collector
