#include "storage/LittleFsBlockStore.h"
#include <Arduino.h>
#include <LittleFS.h>
#include <algorithm>
#include <vector>
#include "collector/Events.h"
#include "collector/LogLimit.h"

using collector::Block;
using collector::ReadStatus;

namespace {

constexpr const char* kDir       = "/log2";
constexpr const char* kLegacyDir = "/log";
constexpr size_t      kReserveBytes = 8 * 1024;  // folga para metadados do LittleFS

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

/** "0000000128.seg" → 128. */
bool parseSegmentName(const String& name, uint32_t& firstSeq) {
    if (!name.endsWith(".seg")) return false;
    char* end = nullptr;
    const unsigned long value = strtoul(name.c_str(), &end, 10);
    if (end != name.c_str() + name.length() - 4) return false;
    firstSeq = static_cast<uint32_t>(value);
    return true;
}

/** Adaptador de File para o codec puro (varredura e leitura de registro). */
class FileByteSource : public collector::IByteSource {
public:
    explicit FileByteSource(File& file) : _file(file) {}
    size_t read(uint8_t* out, size_t n) override { return _file.read(out, n); }
    bool skip(size_t n) override { return _file.seek(n, SeekCur); }
    size_t remaining() const override { return _file.size() - _file.position(); }

private:
    File& _file;
};

}  // namespace

LittleFsBlockStore::LittleFsBlockStore() : _mutex(xSemaphoreCreateMutex()) { configASSERT(_mutex); }

void LittleFsBlockStore::migrateV1() {
    if (!LittleFS.exists(kLegacyDir)) return;
    int removed = 0;
    std::vector<String> names;
    File dir = LittleFS.open(kLegacyDir);
    for (File file = dir.openNextFile(); file; file = dir.openNextFile()) {
        names.push_back(String(kLegacyDir) + "/" + file.name());
        file.close();
    }
    dir.close();
    for (const String& path : names) {
        if (LittleFS.remove(path)) ++removed;
    }
    LittleFS.rmdir(kLegacyDir);
    collector::Event("LOG_MIGRATE").num("removed", removed);
}

bool LittleFsBlockStore::begin(bool wipe, size_t tagLen) {
    if (!LittleFS.begin(true)) return false;
    if (wipe && !LittleFS.format()) return false;
    migrateV1();
    if (!LittleFS.exists(kDir)) LittleFS.mkdir(kDir);

    // O nome de cada segmento é o seq do seu primeiro bloco: ordena por ele.
    std::vector<uint32_t> firstSeqs;
    File dir = LittleFS.open(kDir);
    for (File file = dir.openNextFile(); file; file = dir.openNextFile()) {
        const String name = file.name();
        file.close();
        uint32_t firstSeq;
        if (parseSegmentName(name, firstSeq)) firstSeqs.push_back(firstSeq);
        else LittleFS.remove(String(kDir) + "/" + name);  // não é um segmento
    }
    dir.close();
    std::sort(firstSeqs.begin(), firstSeqs.end());

    Lock lock(_mutex);
    for (uint32_t firstSeq : firstSeqs) {
        const String path = segmentPath(firstSeq);
        File file = LittleFS.open(path, "r");
        collector::ScanResult scan;
        bool valid = false;
        if (file) {
            FileByteSource source(file);
            valid = collector::scanSegment(source, firstSeq, scan) && scan.count > 0;
            file.close();
        }
        if (!valid) {
            LittleFS.remove(path);  // cabeçalho inválido ou sem registro válido
            continue;
        }
        Segment segment = {scan.firstSeq, scan.lastSeq, scan.count, scan.bytes, scan.tagLen,
                           scan.trailingGarbage || scan.count >= collector::kBlocksPerSegment};
        if (scan.trailingGarbage) {
            collector::Event("SEG_SEAL").num("first", scan.firstSeq).num("last", scan.lastSeq).token("reason", "scan_tail");
        }
        pushSegment(segment);  // acima do limite, descarta os mais antigos
    }

    // Limite N de blocos; BLOCK_LOG_MAX_BLOCKS (bancada) substitui o cálculo pela margem.
    constexpr uint32_t kCeiling = kMaxSegments * collector::kBlocksPerSegment;
#if defined(BLOCK_LOG_MAX_BLOCKS)
    _maxBlocks = std::max<uint32_t>(2u * collector::kBlocksPerSegment, std::min<uint32_t>(BLOCK_LOG_MAX_BLOCKS, kCeiling));
#else
    _maxBlocks = collector::maxBlocksFor(LittleFS.totalBytes(), BLOCK_LOG_MARGIN_PCT, tagLen, kCeiling);
#endif
    // N pode ter diminuído entre dois firmwares: descarta os mais antigos até caber.
    while (_count > 1 && _blocks > _maxBlocks) discardOldest();
    refreshUsage();
    return true;
}

void LittleFsBlockStore::pushSegment(const Segment& segment) {
    if (_count == kMaxSegments) discardOldest();
    _segments[(_head + _count) % kMaxSegments] = segment;
    ++_count;
    _blocks += segment.count;
}

void LittleFsBlockStore::dropOldest() {
    LittleFS.remove(segmentPath(at(0).firstSeq));
    _blocks -= at(0).count;
    _head = (_head + 1) % kMaxSegments;
    --_count;
}

void LittleFsBlockStore::discardOldest() {
    const Segment oldest = at(0);
    collector::Event("SEG_DROP").num("first", oldest.firstSeq).num("last", oldest.lastSeq).num("count", oldest.count);
    _lostBlocks += oldest.count;
    if (oldest.lastSeq > _overflowUpTo) _overflowUpTo = oldest.lastSeq;
    dropOldest();
}

size_t LittleFsBlockStore::usedBytes() const { return LittleFS.usedBytes(); }
size_t LittleFsBlockStore::freeBytes() const { return LittleFS.totalBytes() - LittleFS.usedBytes(); }

void LittleFsBlockStore::refreshUsage() {
    _usagePermille = _maxBlocks == 0 ? 0 : static_cast<uint16_t>(std::min<uint64_t>(1000, uint64_t{_blocks} * 1000 / _maxBlocks));
}

// O limite de blocos é a regra principal; os bytes livres do LittleFS são a rede de
// segurança (os blocos de 4 KB do LittleFS podem estourar uma margem pequena).
void LittleFsBlockStore::ensureSpace(size_t bytes) {
    while (_count > 1 && (_blocks + 1 > _maxBlocks || freeBytes() < bytes + kReserveBytes)) discardOldest();
}

void LittleFsBlockStore::startNewSegment() {
    Lock lock(_mutex);
    _forceNew = true;
}

bool LittleFsBlockStore::append(const Block& block) {
    Lock lock(_mutex);
    const uint32_t seq = collector::blockSeq(block);
    const uint8_t tagLen = block.tagLength;

    const char* reason = "first";
    bool newSegment = true;
    if (_count > 0) {
        const Segment& last = newest();
        if (_forceNew) reason = "boot";
        else if (last.sealed) reason = "sealed";
        else if (last.count >= collector::kBlocksPerSegment) reason = "full";
        else if (last.tagLen != tagLen) reason = "tag_len";
        else if (last.lastSeq + 1 != seq) reason = "boot";  // defensivo: I1 exige seq contíguo
        else newSegment = false;
    }

    const size_t recordBytes = collector::recordSize(block.length, tagLen);
    ensureSpace(recordBytes + (newSegment ? collector::kSegmentHeaderSize : 0));

    // Monta cabeçalho (se novo) + dados + tag + CRC LE e escreve de uma vez.
    size_t n = 0;
    if (newSegment) {
        collector::encodeSegmentHeader(tagLen, _record);
        n = collector::kSegmentHeaderSize;
    }
    memcpy(_record + n, block.bytes, block.length);
    n += block.length;
    memcpy(_record + n, block.tag, tagLen);
    n += tagLen;
    collector::putU32(_record + n, block.crc);
    n += collector::kCrcSize;

    const uint32_t firstSeq = newSegment ? seq : newest().firstSeq;
    // close() sincroniza: ao retornar, o registro está na flash.
    File file = LittleFS.open(segmentPath(firstSeq), newSegment ? "w" : "a");
    if (!file) {
        if (!newSegment) newest().sealed = true;
        _forceNew = true;
        return false;
    }
    const size_t written = file.write(_record, n);
    file.close();
    if (written != n) {
        if (newSegment) {
            LittleFS.remove(segmentPath(firstSeq));
        } else {
            Segment& s = newest();
            s.sealed = true;
            collector::Event("SEG_SEAL").num("first", s.firstSeq).num("last", s.lastSeq).token("reason", "write_fail");
        }
        _forceNew = true;
        return false;
    }

    _forceNew = false;
    if (newSegment) {
        collector::Event("SEG_NEW").num("first", seq).num("tag_len", tagLen).token("reason", reason);
        pushSegment({seq, seq, 1, static_cast<uint32_t>(n), tagLen, false});
    } else {
        Segment& segment = newest();
        segment.lastSeq = seq;
        ++segment.count;
        ++_blocks;
        segment.bytes += static_cast<uint32_t>(n);
        if (segment.count >= collector::kBlocksPerSegment) segment.sealed = true;
    }
    refreshUsage();
    return true;
}

ReadStatus LittleFsBlockStore::read(uint32_t seq, Block& out) {
    Lock lock(_mutex);
    for (size_t i = 0; i < _count; ++i) {
        const Segment& segment = at(i);
        if (seq < segment.firstSeq || seq > segment.lastSeq) continue;

        File file = LittleFS.open(segmentPath(segment.firstSeq), "r");
        if (!file) return ReadStatus::Corrupt;
        FileByteSource source(file);
        const ReadStatus status = collector::readRecord(source, segment.firstSeq, seq, out);
        file.close();
        return status;
    }
    return ReadStatus::NotFound;
}

uint32_t LittleFsBlockStore::nextStoredSeq(uint32_t fromSeq) const {
    Lock lock(_mutex);
    for (size_t i = 0; i < _count; ++i) {
        const Segment& segment = at(i);
        if (segment.lastSeq >= fromSeq) return std::max(fromSeq, segment.firstSeq);
    }
    return 0;
}

size_t LittleFsBlockStore::deleteUpTo(uint32_t upTo) {
    Lock lock(_mutex);
    size_t removed = 0;
    // Só remove segmentos inteiros; o aberto (o mais novo) fica até encher.
    while (_count > 1 && at(0).lastSeq <= upTo) {
        dropOldest();
        ++removed;
    }
    refreshUsage();
    return removed;
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

bool LittleFsBlockStore::corrupt(uint32_t seq) {
    if (!collector::kEventsEnabled) return false;
    Lock lock(_mutex);
    for (size_t i = 0; i < _count; ++i) {
        const Segment& segment = at(i);
        if (seq < segment.firstSeq || seq > segment.lastSeq) continue;

        File file = LittleFS.open(segmentPath(segment.firstSeq), "r+");
        if (!file) return false;
        // Percorre os cabeçalhos até o registro `seq` e inverte o 1º byte de registro
        // (offset 12): o cabeçalho fica intacto e a varredura do boot continua válida.
        size_t offset = collector::kSegmentHeaderSize;
        for (uint32_t k = segment.firstSeq; k < seq; ++k) {
            uint8_t head[collector::kHeaderSize];
            if (!file.seek(offset) || file.read(head, sizeof(head)) != sizeof(head)) return false;
            offset += collector::recordSize(collector::getU16(&head[collector::header::kLength]), segment.tagLen);
        }
        const size_t target = offset + collector::kHeaderSize;
        uint8_t value;
        if (!file.seek(target) || file.read(&value, 1) != 1) return false;
        value = static_cast<uint8_t>(~value);
        const bool ok = file.seek(target) && file.write(&value, 1) == 1;
        file.close();
        return ok;
    }
    return false;
}
