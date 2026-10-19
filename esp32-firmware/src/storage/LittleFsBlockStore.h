#pragma once
#include <freertos/FreeRTOS.h>
#include <freertos/semphr.h>
#include "collector/IBlockStore.h"

/**
 * Log de blocos sobre LittleFS, em arquivos de segmento de até 64 blocos
 * ("/log/<primeiro seq>.seg"). Apagar = remover um segmento inteiro; quando
 * falta espaço, o segmento mais antigo é removido e a perda é contada.
 *
 * Usado pela Flush (append) e pelo Upload (read/deleteUpTo) ao mesmo tempo,
 * por isso tem mutex interno.
 */
class LittleFsBlockStore : public collector::IBlockStore {
public:
    LittleFsBlockStore();

    /** Monta a partição (formatando se preciso) e reconstrói o índice. */
    bool begin();

    bool append(const collector::Block& block) override;
    bool read(uint32_t seq, collector::Block& out) override;
    void deleteUpTo(uint32_t upTo) override;

    bool     empty() const override;
    uint32_t oldestSeq() const override;
    uint32_t newestSeq() const override;
    uint32_t lostBlocks() const override { return _lostBlocks; }

private:
    struct Segment {
        uint32_t firstSeq;
        uint32_t lastSeq;
        uint16_t count;
        uint32_t bytes;
    };

    static constexpr size_t kMaxSegments = 256;

    SemaphoreHandle_t _mutex;
    Segment  _segments[kMaxSegments] = {};
    size_t   _head  = 0;   // índice circular do segmento mais antigo
    size_t   _count = 0;
    uint32_t _lostBlocks = 0;

    Segment&       at(size_t i) { return _segments[(_head + i) % kMaxSegments]; }
    const Segment& at(size_t i) const { return _segments[(_head + i) % kMaxSegments]; }
    Segment&       newest() { return at(_count - 1); }

    bool loadSegment(const char* path, Segment& out);
    void pushSegment(const Segment& segment);
    void dropOldest();
    void ensureSpace(size_t bytes);
};
