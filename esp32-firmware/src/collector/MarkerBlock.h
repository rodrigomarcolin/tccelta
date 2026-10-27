#pragma once
#include <cstddef>
#include <cstdint>

#include "collector/BlockFormat.h"

namespace collector {

/**
 * Blocos marcadores BOOT e LOSS (spec fase-1/02 §3): blocos 0x03 comuns cuja
 * área de dados tem um único registro de 11 B (`tamanho = 23`).
 */
constexpr uint8_t kLossRecordType = 0xFD;
constexpr uint8_t kBootRecordType = 0xFE;
constexpr size_t  kMarkerRecordSize = 11;
constexpr size_t  kMarkerBlockLength = kHeaderSize + kMarkerRecordSize;  // 23

enum class MarkerKind : uint8_t { None, Boot, Loss };
struct MarkerInfo {
    MarkerKind kind;
    uint32_t   fromSeq;
    uint32_t   toSeq;
};

/** Monta a área de dados (sem tag). `block.length = 23`; não mexe em tag/crc. */
void buildBootMarker(Block& block, uint32_t seq, uint16_t bootId, uint32_t tMs, uint32_t fromSeq,
                     uint32_t toSeq);
/** seq = fromSeq, boot_id e t_ms = 0. */
void buildLossMarker(Block& block, uint32_t fromSeq, uint32_t toSeq);

/** Lê um bloco e diz se é marcador (tamanho 23 e tipo 0xFD/0xFE). */
MarkerInfo parseMarker(const Block& block);

}  // namespace collector
