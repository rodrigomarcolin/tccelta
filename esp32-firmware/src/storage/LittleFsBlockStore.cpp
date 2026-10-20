#include "storage/LittleFsBlockStore.h"
#include <Arduino.h>
#include <LittleFS.h>
#include <algorithm>
#include "collector/Events.h"

using collector::Block;
using collector::kBlockSize;
using collector::kHeaderSize;

namespace {

constexpr const char* kDir             = "/log";
constexpr uint16_t    kBlocksPerSegment = 64;
constexpr size_t      kReserveBytes    = 8 * 1024;  // folga para metadados do LittleFS

class Lock {
public:
    explicit Lock(SemaphoreHandle_t mutex) : _mutex(mutex) { xSemaphoreTake(_mutex, portMAX_DELAY); }
    ~Lock() { xSemaphoreGive(_mutex); }

private:
    SemaphoreHandle_t _mutex;
};

String segmentPath(uint32_t firstSeq) {
    char path[32];
    snprintf(path, sizeof(path), "%s/%010lu.seg", kDir, static_cast<unsigned long>(firstSeq));
    return String(path);
}

size_t freeBytes() { return LittleFS.totalBytes() - LittleFS.usedBytes(); }

/** Lê o cabeçalho do próximo bloco; false se acabou ou está inconsistente. */
bool readHeader(File& file, uint8_t* header, size_t& length) {
    if (file.read(header, kHeaderSize) != kHeaderSize) return false;
    length = collector::getU16(&header[collector::header::kLength]);
    return length >= kHeaderSize && length <= kBlockSize &&
           static_cast<size_t>(file.available()) >= length - kHeaderSize;
}

}  // namespace

LittleFsBlockStore::LittleFsBlockStore() : _mutex(xSemaphoreCreateMutex()) { configASSERT(_mutex); }

bool LittleFsBlockStore::begin() {
    if (!LittleFS.begin(true)) return false;
    if (!LittleFS.exists(kDir)) LittleFS.mkdir(kDir);

    Segment found[kMaxSegments];
    size_t foundCount = 0;
    File dir = LittleFS.open(kDir);
    for (File file = dir.openNextFile(); file; file = dir.openNextFile()) {
        const String path = String(kDir) + "/" + file.name();
        file.close();
        Segment segment;
        if (foundCount < kMaxSegments && loadSegment(path.c_str(), segment)) {
            found[foundCount++] = segment;
        } else {
            LittleFS.remove(path);  // vazio ou ilegível
        }
    }

    std::sort(found, found + foundCount,
              [](const Segment& a, const Segment& b) { return a.firstSeq < b.firstSeq; });
    Lock lock(_mutex);
    for (size_t i = 0; i < foundCount; ++i) pushSegment(found[i]);
    return true;
}

bool LittleFsBlockStore::loadSegment(const char* path, Segment& out) {
    File file = LittleFS.open(path, "r");
    if (!file) return false;

    out = {};
    uint8_t header[kHeaderSize];
    size_t length;
    // Um bloco cortado no meio (energia) não é visível no LittleFS, mas por
    // garantia a varredura para no primeiro cabeçalho inconsistente.
    while (readHeader(file, header, length)) {
        const uint32_t seq = collector::getU32(&header[collector::header::kSeq]);
        if (out.count == 0) out.firstSeq = seq;
        out.lastSeq = seq;
        ++out.count;
        out.bytes += length;
        file.seek(length - kHeaderSize, SeekCur);
    }
    return out.count > 0;
}

void LittleFsBlockStore::pushSegment(const Segment& segment) {
    if (_count == kMaxSegments) dropOldest();
    _segments[(_head + _count) % kMaxSegments] = segment;
    ++_count;
}

void LittleFsBlockStore::dropOldest() {
    LittleFS.remove(segmentPath(at(0).firstSeq));
    _head = (_head + 1) % kMaxSegments;
    --_count;
}

void LittleFsBlockStore::ensureSpace(size_t bytes) {
    while (_count > 1 && freeBytes() < bytes + kReserveBytes) {
        const Segment& oldest = at(0);
        collector::Event("SEG_DROP").num("first", oldest.firstSeq).num("last", oldest.lastSeq)
            .num("count", oldest.count);
        _lostBlocks += oldest.count;
        dropOldest();
    }
}

bool LittleFsBlockStore::append(const Block& block) {
    Lock lock(_mutex);
    ensureSpace(block.length);

    const uint32_t seq = collector::blockSeq(block);
    const bool newSegment = _count == 0 || newest().count >= kBlocksPerSegment;
    const uint32_t firstSeq = newSegment ? seq : newest().firstSeq;

    // close() sincroniza: ao retornar, o bloco está na flash.
    File file = LittleFS.open(segmentPath(firstSeq), "a");
    if (!file) return false;
    const size_t written = file.write(block.bytes, block.length);
    file.close();
    if (written != block.length) return false;

    if (newSegment) {
        pushSegment({seq, seq, 1, static_cast<uint32_t>(block.length)});
    } else {
        Segment& segment = newest();
        segment.lastSeq = seq;
        ++segment.count;
        segment.bytes += block.length;
    }
    return true;
}

bool LittleFsBlockStore::read(uint32_t seq, Block& out) {
    Lock lock(_mutex);
    for (size_t i = 0; i < _count; ++i) {
        const Segment& segment = at(i);
        if (seq < segment.firstSeq || seq > segment.lastSeq) continue;

        File file = LittleFS.open(segmentPath(segment.firstSeq), "r");
        if (!file) return false;
        size_t length;
        while (readHeader(file, out.bytes, length)) {
            if (collector::blockSeq(out) == seq) {
                out.length = length;
                return file.read(out.bytes + kHeaderSize, length - kHeaderSize) == length - kHeaderSize;
            }
            file.seek(length - kHeaderSize, SeekCur);
        }
        return false;
    }
    return false;
}

void LittleFsBlockStore::deleteUpTo(uint32_t upTo) {
    Lock lock(_mutex);
    // Só remove segmentos inteiros; o aberto (o mais novo) fica até encher.
    while (_count > 1 && at(0).lastSeq <= upTo) dropOldest();
}

bool LittleFsBlockStore::empty() const {
    Lock lock(_mutex);
    return _count == 0;
}

uint32_t LittleFsBlockStore::oldestSeq() const {
    Lock lock(_mutex);
    return _count == 0 ? 0 : at(0).firstSeq;
}

uint32_t LittleFsBlockStore::newestSeq() const {
    Lock lock(_mutex);
    return _count == 0 ? 0 : at(_count - 1).lastSeq;
}
