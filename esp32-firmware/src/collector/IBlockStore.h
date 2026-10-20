#pragma once
#include <cstdint>
#include "collector/BlockFormat.h"

namespace collector {

/**
 * Log persistente de blocos, ordenado por `seq`. Quando não há espaço, o
 * próprio log descarta os blocos mais antigos (a perda é visível como
 * `oldestSeq` maior para o servidor).
 */
class IBlockStore {
public:
    virtual ~IBlockStore() = default;

    /** Grava o bloco; só retorna true depois que ele está sincronizado na flash. */
    virtual bool append(const Block& block) = 0;

    /** Lê o bloco `seq`; false se ele não existe (já apagado ou ainda não gravado). */
    virtual bool read(uint32_t seq, Block& out) = 0;

    /** Apaga os blocos com `seq ≤ upTo` que puder (pode reter alguns). */
    virtual void deleteUpTo(uint32_t upTo) = 0;

    virtual bool     empty() const = 0;
    virtual uint32_t oldestSeq() const = 0;   // válido só se !empty()
    virtual uint32_t newestSeq() const = 0;   // 0 se vazio
    virtual uint32_t lostBlocks() const = 0;  // descartados por falta de espaço
};

/** Onde a Flush registra o último `seq` gravado (NVS no ESP32). */
class ISeqPersistence {
public:
    virtual ~ISeqPersistence() = default;
    virtual void saveLastSeq(uint32_t seq) = 0;
};

}  // namespace collector
