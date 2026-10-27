#pragma once
#include <atomic>
#include <freertos/FreeRTOS.h>
#include <freertos/semphr.h>
#include "collector/BlockFormat.h"
#include "collector/IBlockStore.h"
#include "collector/SegmentFormat.h"

/**
 * Log de blocos v2 sobre LittleFS (spec fase-1/02 §6, §10): arquivos de
 * segmento "/log2/<primeiro seq>.seg" com cabeçalho "TCS2" (versão e tag_len)
 * e até 64 registros `dados | tag | CRC32`. O `seq` é contíguo dentro de um
 * segmento: o BOOT abre sempre um segmento novo e uma gravação que falha
 * sela o segmento. Apagar = remover um segmento inteiro; quando falta
 * espaço, o mais antigo é removido e a perda fica em `overflowUpTo`.
 *
 * Usado pela Flush (append) e pela Sync (read/deleteUpTo) ao mesmo tempo,
 * por isso tem mutex interno. O log da versão 1 (`/log`, sem tag) é apagado
 * no primeiro boot (evento LOG_MIGRATE).
 */
class LittleFsBlockStore : public collector::IBlockStore {
public:
    LittleFsBlockStore();

    /**
     * Monta a partição (formatando se preciso), migra o log v1 e reconstrói o
     * índice. `wipe` = apagar todos os blocos antes (COLLECTOR_WIPE_ON_BOOT).
     */
    bool begin(bool wipe = false);

    bool append(const collector::Block& block) override;
    collector::ReadStatus read(uint32_t seq, collector::Block& out) override;
    uint32_t nextStoredSeq(uint32_t fromSeq) const override;
    size_t deleteUpTo(uint32_t upTo) override;
    void startNewSegment() override;

    bool     empty() const override;
    uint32_t oldestSeq() const override;
    uint32_t newestSeq() const override;
    uint32_t lostBlocks() const override { return _lostBlocks; }
    uint32_t overflowUpTo() const override { return _overflowUpTo; }
    uint16_t usagePermille() const override { return _usagePermille; }
    bool     corrupt(uint32_t seq) override;

private:
    struct Segment {
        uint32_t firstSeq;
        uint32_t lastSeq;
        uint16_t count;
        uint32_t bytes;   // cabeçalho + registros válidos
        uint8_t  tagLen;
        bool     sealed;  // não recebe mais appends (cauda inválida ou falha de escrita)
    };

    static constexpr size_t kMaxSegments = 256;

    SemaphoreHandle_t _mutex;
    Segment  _segments[kMaxSegments] = {};
    size_t   _head  = 0;   // índice circular do segmento mais antigo
    size_t   _count = 0;
    bool     _forceNew = false;
    std::atomic<uint32_t> _lostBlocks{0};
    std::atomic<uint32_t> _overflowUpTo{0};
    std::atomic<uint16_t> _usagePermille{0};
    // Buffer de escrita: cabeçalho do segmento + maior registro (só sob o mutex).
    uint8_t  _record[collector::kSegmentHeaderSize + collector::kBlockSize + collector::kMaxTagSize +
                     collector::kCrcSize];

    Segment&       at(size_t i) { return _segments[(_head + i) % kMaxSegments]; }
    const Segment& at(size_t i) const { return _segments[(_head + i) % kMaxSegments]; }
    Segment&       newest() { return at(_count - 1); }

    void pushSegment(const Segment& segment);
    void dropOldest();      // remove o segmento mais antigo, sem contar perda
    void discardOldest();   // idem, contando como perda (SEG_DROP) e atualizando overflowUpTo
    void ensureSpace(size_t bytes);
    size_t usedBytes() const;
    size_t capacityBytes() const;
    size_t freeBytes() const;
    void refreshUsage();
    void migrateV1();
};
