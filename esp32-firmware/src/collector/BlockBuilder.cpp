#include "collector/BlockBuilder.h"
#include "collector/Clock.h"
#include <cstring>

namespace collector {

void BlockBuilder::open(Block& block, uint32_t tStartMs) {
    memset(block.bytes, 0, kHeaderSize);
    block.length = kHeaderSize;
    _block    = &block;
    _tStartMs = tStartMs;
}

bool BlockBuilder::deltaFor(uint32_t tMs, int16_t& delta) const {
    // GPS é marcado ao chegar o comando e pode ser anterior ao início do bloco.
    const int32_t elapsed = static_cast<int32_t>(tMs - _tStartMs);
    if (elapsed < INT16_MIN || elapsed > INT16_MAX) return false;
    delta = static_cast<int16_t>(elapsed);
    return true;
}

bool BlockBuilder::addPid(uint8_t pid, const uint8_t* value, uint8_t len, uint32_t tMs) {
    int16_t delta;
    if (!_block || !fits(kPidRecordSize) || !deltaFor(tMs, delta)) return false;

    uint8_t* p = &_block->bytes[_block->length];
    p[0] = pid;
    putU16(&p[1], static_cast<uint16_t>(delta));
    p[3] = len > 0 ? value[0] : 0;
    p[4] = len > 1 ? value[1] : 0;
    _block->length += kPidRecordSize;
    return true;
}

bool BlockBuilder::addGps(const GpsFix& fix, uint32_t tMs) {
    int16_t delta;
    if (!_block || !fits(kGpsRecordSize) || !deltaFor(tMs, delta)) return false;

    uint8_t* p = &_block->bytes[_block->length];
    p[0] = kGpsRecordType;
    putU16(&p[1], static_cast<uint16_t>(delta));
    putU32(&p[3], static_cast<uint32_t>(fix.latE7));
    putU32(&p[7], static_cast<uint32_t>(fix.lonE7));
    putU32(&p[11], static_cast<uint32_t>(fix.altMm));
    _block->length += kGpsRecordSize;
    return true;
}

Block* BlockBuilder::close() {
    Block* block = _block;
    if (!block) return nullptr;
    putU32(&block->bytes[header::kStartMs], _tStartMs);
    putU16(&block->bytes[header::kLength], static_cast<uint16_t>(block->length));
    _block = nullptr;
    return block;
}

}  // namespace collector
