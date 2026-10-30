#include "collector/MarkerBlock.h"

#include <cstring>

namespace collector {

namespace {
void build(Block& block, uint32_t seq, uint16_t bootId, uint32_t tMs, uint8_t type, uint32_t fromSeq,
           uint32_t toSeq) {
    uint8_t* b = block.bytes;
    putU32(&b[header::kSeq], seq);
    putU16(&b[header::kBootId], bootId);
    putU32(&b[header::kStartMs], tMs);
    putU16(&b[header::kLength], static_cast<uint16_t>(kMarkerBlockLength));
    b[kHeaderSize] = type;
    putU16(&b[kHeaderSize + 1], 0);  // Δt
    putU32(&b[kHeaderSize + 3], fromSeq);
    putU32(&b[kHeaderSize + 7], toSeq);
    block.length = kMarkerBlockLength;
}
}  // namespace

void buildBootMarker(Block& block, uint32_t seq, uint16_t bootId, uint32_t tMs, uint32_t fromSeq,
                     uint32_t toSeq) {
    build(block, seq, bootId, tMs, kBootRecordType, fromSeq, toSeq);
}

void buildLossMarker(Block& block, uint32_t fromSeq, uint32_t toSeq) {
    build(block, fromSeq, 0, 0, kLossRecordType, fromSeq, toSeq);
}

MarkerInfo parseMarker(const Block& block) {
    MarkerInfo info{MarkerKind::None, 0, 0};
    if (block.length != kMarkerBlockLength) return info;
    const uint8_t type = block.bytes[kHeaderSize];
    if (type != kBootRecordType && type != kLossRecordType) return info;
    info.kind = type == kBootRecordType ? MarkerKind::Boot : MarkerKind::Loss;
    info.fromSeq = getU32(&block.bytes[kHeaderSize + 3]);
    info.toSeq = getU32(&block.bytes[kHeaderSize + 7]);
    return info;
}

}  // namespace collector
