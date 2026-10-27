#pragma once
#include <cstddef>
#include <cstdint>
#include "collector/BlockFormat.h"

namespace collector {

enum class ReadStatus : uint8_t {
    Ok,        // registro lido inteiro (dados, tag, crc); o CRC NÃO foi conferido
    NotFound,  // seq fora de todo segmento do índice
    Corrupt,   // o índice diz que existe, mas a varredura não chega nele / E/S falhou
};

/**
 * Log persistente de blocos, ordenado por `seq` (spec fase-1/02 §9). Quando
 * não há espaço, o próprio log descarta os blocos mais antigos (a perda fica
 * visível em `overflowUpTo` e no `oldestSeq`). O CRC é conferido por quem lê.
 */
class IBlockStore {
public:
    virtual ~IBlockStore() = default;

    /** Grava dados | tag | crc; true só depois que o registro está sincronizado na flash. */
    virtual bool append(const Block& block) = 0;

    virtual ReadStatus read(uint32_t seq, Block& out) = 0;

    /** Menor `seq` guardado >= `fromSeq`; 0 = nenhum. */
    virtual uint32_t nextStoredSeq(uint32_t fromSeq) const = 0;

    /** Apaga os segmentos inteiros com `lastSeq <= upTo`; devolve quantos removeu. */
    virtual size_t deleteUpTo(uint32_t upTo) = 0;

    /** O próximo `append` abre um arquivo novo (o BOOT abre sempre um segmento novo). */
    virtual void startNewSegment() = 0;

    virtual bool     empty() const = 0;
    virtual uint32_t oldestSeq() const = 0;       // TAIL na flash (0 se vazio)
    virtual uint32_t newestSeq() const = 0;       // HEAD (0 se vazio)
    virtual uint32_t lostBlocks() const = 0;      // descartados por falta de espaço
    virtual uint32_t overflowUpTo() const = 0;    // maior seq já descartado por falta de espaço (0 = nunca)
    virtual uint16_t usagePermille() const = 0;   // ocupação do log (0 a 1000), em cache

    /** Bancada (AT#CORRUPT), só COLLECTOR_EVENTS: inverte um byte do registro `seq`. */
    virtual bool corrupt(uint32_t /*seq*/) { return false; }
};

}  // namespace collector
