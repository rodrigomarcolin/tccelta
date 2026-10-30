#include "collector/SegmentFormat.h"

#include <cstring>

#include "crypto/Crc32.h"

namespace collector {

namespace {
constexpr uint8_t kMagic[4] = {'T', 'C', 'S', '2'};

bool validTagLen(uint8_t t) { return t == 0 || t == 32 || t == 64; }

/** Lê um registro inteiro em `out`; false = fim ou incoerência. */
bool readOne(IByteSource& src, uint8_t tagLen, Block& out) {
    if (src.read(out.bytes, kHeaderSize) != kHeaderSize) return false;
    const size_t length = getU16(&out.bytes[header::kLength]);
    if (length < kHeaderSize || length > kBlockSize) return false;
    if (src.remaining() < length - kHeaderSize + tagLen + kCrcSize) return false;
    if (src.read(out.bytes + kHeaderSize, length - kHeaderSize) != length - kHeaderSize) return false;
    out.length = length;
    out.tagLength = tagLen;
    if (tagLen > 0 && src.read(out.tag, tagLen) != tagLen) return false;
    uint8_t crc[kCrcSize];
    if (src.read(crc, kCrcSize) != kCrcSize) return false;
    out.crc = getU32(crc);
    return true;
}
}  // namespace

void encodeSegmentHeader(uint8_t tagLen, uint8_t out[kSegmentHeaderSize]) {
    memcpy(out, kMagic, 4);
    out[4] = kSegmentVersion;
    out[5] = tagLen;
    putU16(&out[6], 0);
}

bool decodeSegmentHeader(const uint8_t in[kSegmentHeaderSize], uint8_t& tagLen) {
    if (memcmp(in, kMagic, 4) != 0 || in[4] != kSegmentVersion || !validTagLen(in[5])) return false;
    tagLen = in[5];
    return true;
}

uint32_t computeCrc(const Block& b) {
    return crypto::crc32(b.tag, b.tagLength, crypto::crc32(b.bytes, b.length));
}

bool crcMatches(const Block& b) { return computeCrc(b) == b.crc; }

bool scanSegment(IByteSource& src, uint32_t expectedFirstSeq, ScanResult& out) {
    out = {};
    uint8_t head[kSegmentHeaderSize];
    if (src.read(head, sizeof(head)) != sizeof(head) || !decodeSegmentHeader(head, out.tagLen)) return false;
    out.bytes = kSegmentHeaderSize;
    out.firstSeq = expectedFirstSeq;

    // Varre só o cabeçalho de cada registro e pula o resto (não lê a tag).
    uint8_t h[kHeaderSize];
    while (src.remaining() > 0) {
        if (src.read(h, kHeaderSize) != kHeaderSize) break;
        const size_t length = getU16(&h[header::kLength]);
        if (length < kHeaderSize || length > kBlockSize) break;
        if (getU32(&h[header::kSeq]) != expectedFirstSeq + out.count) break;
        const size_t rest = length - kHeaderSize + out.tagLen + kCrcSize;
        if (src.remaining() < rest || !src.skip(rest)) break;
        out.lastSeq = expectedFirstSeq + out.count;
        ++out.count;
        out.bytes += static_cast<uint32_t>(recordSize(length, out.tagLen));
    }
    out.trailingGarbage = src.remaining() > 0;
    return true;
}

ReadStatus readRecord(IByteSource& src, uint32_t firstSeq, uint32_t seq, Block& out) {
    if (seq < firstSeq) return ReadStatus::NotFound;
    uint8_t head[kSegmentHeaderSize];
    uint8_t tagLen;
    if (src.read(head, sizeof(head)) != sizeof(head) || !decodeSegmentHeader(head, tagLen)) {
        return ReadStatus::Corrupt;
    }
    const uint32_t index = seq - firstSeq;
    for (uint32_t k = 0; k <= index; ++k) {
        if (k < index) {
            uint8_t h[kHeaderSize];
            if (src.read(h, kHeaderSize) != kHeaderSize) return ReadStatus::Corrupt;
            const size_t length = getU16(&h[header::kLength]);
            if (length < kHeaderSize || length > kBlockSize) return ReadStatus::Corrupt;
            if (!src.skip(length - kHeaderSize + tagLen + kCrcSize)) return ReadStatus::Corrupt;
        } else if (!readOne(src, tagLen, out)) {
            return ReadStatus::Corrupt;
        }
    }
    return blockSeq(out) == seq ? ReadStatus::Ok : ReadStatus::Corrupt;
}

}  // namespace collector
