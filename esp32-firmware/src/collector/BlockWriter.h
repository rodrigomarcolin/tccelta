#pragma once
#include <cstdint>
#include "collector/Clock.h"
#include "collector/IBlockStore.h"
#include "collector/IMessageAuth.h"
#include "collector/SeqReserve.h"

namespace collector {

/** Medidas de uma gravação (spec fase-1/02 §8); as durações alimentam o evento FLUSH. */
struct WriteReport {
    uint32_t seq = 0;
    bool     ok = false;
    bool     signOk = true;     // false: a assinatura falhou nas duas tentativas (SIGN_FAIL)
    bool     boot = false;      // o bloco gravado foi o BOOT
    int      attempts = 0;
    bool     reserved = false;  // gravou nova reserva de seq na NVS antes
    uint32_t reserveUs = 0;
    uint32_t signUs = 0;
    uint32_t crcUs = 0;
    uint32_t writeUs = 0;
};

/**
 * Grava um bloco fechado, nesta ordem (spec fase-1/02 §8, 03 §2):
 *   1. se o BOOT está pendente, grava-o antes de qualquer dado;
 *   2. reserva de `seq` na NVS, se `seq >= reserva`, antes da flash;
 *   3. carimba `seq` e `boot_id`, assina (`IMessageAuth`, 2 tentativas) e calcula o CRC;
 *   4. `append` sincronizado (2 tentativas, a 2ª em segmento novo);
 *   5. só então consome o `seq`.
 * Se a assinatura ou o `append` falharem duas vezes, o bloco é descartado, o `seq` não é
 * consumido e `failedBlocks` cresce. Nunca vai à flash um bloco sem tag válida.
 */
class BlockWriter {
public:
    BlockWriter(IBlockStore& store, ISeqReservePersistence& persistence, IMessageAuth& auth,
                const IClock& clock, SeqReserve reserve, uint16_t bootId)
        : _store(store), _persistence(persistence), _auth(auth), _clock(clock), _reserve(reserve),
          _bootId(bootId) {}

    /** Bloco de dados fechado pela Aquisição. Se o BOOT ainda está pendente, tenta-o antes. */
    bool write(Block& block, WriteReport& report);

    /**
     * Monta e grava o BOOT deste boot com seq = R. Chamado uma vez no boot; se
     * falhar, fica pendente e é refeito no próximo `write()`.
     */
    bool writeBootMarker(uint32_t fromSeq, uint32_t toSeq, WriteReport& report);

    bool     bootPending() const { return _bootPending; }
    uint32_t nextSeq() const { return _reserve.peek(); }
    uint32_t reserve() const { return _reserve.reserve(); }
    uint32_t failedBlocks() const { return _failedBlocks; }

    /** O BOOT montado (valido depois de `writeBootMarker`), para o evento FLUSH. */
    const Block& bootBlock() const { return _boot; }

private:
    static constexpr int kAppendAttempts = 2;
    static constexpr int kSignAttempts = 2;

    IBlockStore&            _store;
    ISeqReservePersistence& _persistence;
    IMessageAuth&           _auth;
    const IClock&           _clock;
    SeqReserve              _reserve;
    uint16_t                _bootId;
    uint32_t                _failedBlocks = 0;
    bool                    _bootPending = false;
    uint32_t                _bootFrom = 0, _bootTo = 0;
    Block                   _boot = {};  // ~1,1 KB: o BlockWriter é estático no firmware, nunca na pilha

    bool commit(Block& block, WriteReport& report);
};

}  // namespace collector
