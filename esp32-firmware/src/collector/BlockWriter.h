#pragma once
#include <cstdint>
#include "collector/IBlockStore.h"
#include "collector/SeqCounter.h"

namespace collector {

/**
 * Grava um bloco fechado, nesta ordem (plano §5.5):
 *   1. carimba `seq` e `boot_id`;
 *   2. `append` sincronizado (com uma nova tentativa se falhar);
 *   3. registra o `seq` na NVS;
 *   4. só então consome o `seq`.
 * Se o `append` falhar duas vezes, o bloco é descartado e o `seq` não é
 * consumido.
 */
class BlockWriter {
public:
    BlockWriter(IBlockStore& store, ISeqPersistence& persistence, SeqCounter seq, uint16_t bootId)
        : _store(store), _persistence(persistence), _seq(seq), _bootId(bootId) {}

    bool write(Block& block);

    uint32_t nextSeq() const { return _seq.peek(); }
    uint32_t failedBlocks() const { return _failedBlocks; }

private:
    IBlockStore&     _store;
    ISeqPersistence& _persistence;
    SeqCounter       _seq;
    uint16_t         _bootId;
    uint32_t         _failedBlocks = 0;
};

}  // namespace collector
