#pragma once
#include <cstdint>
#include "collector/BlockFormat.h"

namespace collector {

/**
 * Monta um bloco: abre no primeiro registro, acrescenta leituras de PID e
 * posições GPS, e fecha preenchendo `t_ms inicial` e `tamanho`. Não conhece
 * `seq` nem `boot_id`, que são da Flush.
 */
class BlockBuilder {
public:
    /** Abre `block` com o primeiro registro em `tStartMs`; fecha `closeAfterMs` depois. */
    void open(Block& block, uint32_t tStartMs, uint32_t closeAfterMs);
    bool isOpen() const { return _block != nullptr; }
    bool empty() const { return !_block || _block->length == kHeaderSize; }

    /** Instante do primeiro registro (t_ms inicial do cabeçalho). */
    uint32_t startMs() const { return _tStartMs; }

    /** Instante em que o bloco aberto deve fechar (tStart + T_flush). */
    uint32_t deadlineMs() const { return _tStartMs + _closeAfterMs; }

    /** false = não cabe (espaço, registro anterior ao início ou Δt > 65 535 ms); o chamador fecha e abre outro. */
    bool addPid(uint8_t pid, const uint8_t* value, uint8_t len, uint32_t tMs);
    bool addGps(const GpsFix& fix, uint32_t tMs);

    /** Fecha o bloco aberto e o devolve (nullptr se não havia). */
    Block* close();

private:
    Block*   _block    = nullptr;
    uint32_t _tStartMs = 0;
    uint32_t _closeAfterMs = 0;

    /** Δt (sem sinal) a partir do início; false se anterior ao início ou acima de 16 bits. */
    bool deltaFor(uint32_t tMs, uint16_t& delta) const;
    bool fits(size_t recordSize) const { return _block->length + recordSize <= kBlockSize; }
};

}  // namespace collector
