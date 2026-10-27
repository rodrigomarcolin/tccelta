#pragma once
#include <cstdint>

namespace collector {

#ifndef SEQ_RESERVE_STEP
#define SEQ_RESERVE_STEP 64
#endif
constexpr uint32_t kSeqReserveStep = SEQ_RESERVE_STEP;
static_assert(kSeqReserveStep >= 1 && kSeqReserveStep <= 65536, "SEQ_RESERVE_STEP fora da faixa");

/** Onde a Flush grava a reserva de seq (NVS no ESP32). */
class ISeqReservePersistence {
public:
    virtual ~ISeqReservePersistence() = default;
    /** Grava e confirma (commit). false = a NVS falhou. */
    virtual bool saveSeqReserve(uint32_t reserve) = 0;
};

/** Resultado da recuperação no boot (Fig. 4 do documento de segurança). */
struct SeqRecovery {
    uint32_t bootSeq;      // R: seq do bloco BOOT
    uint32_t fromSeq;      // último seq na flash + 1 (R se a flash está vazia)
    uint32_t toSeq;        // R − 1
    uint32_t reserveRead;  // valor lido de seq_rsv (0 = ausente)
    bool     migrated;     // R veio do legado (last_seq/log), não de seq_rsv
};

/**
 * Dono do seq. `peek()` é o próximo seq; só avança em `commit()` (bloco
 * gravado). Antes de gravar `seq`, se `needsReserve(seq)`, a Flush grava
 * `reserveFor(seq)` na NVS e chama `reserved(...)`.
 * Invariante: todo seq gravado na flash é < reserva persistida.
 */
class SeqReserve {
public:
    SeqReserve(uint32_t next = 1, uint32_t reserve = 0, uint32_t step = kSeqReserveStep)
        : _next(next), _reserve(reserve), _step(step) {}

    static SeqRecovery recover(uint32_t reserveInNvs, uint32_t legacyLastSeq, uint32_t newestInLog);
    /** Contador pronto para gravar o BOOT: next = R, reserva em memória = 0 (força gravar R + N). */
    static SeqReserve afterRecovery(const SeqRecovery& recovery, uint32_t step = kSeqReserveStep) {
        return SeqReserve(recovery.bootSeq, 0, step);
    }

    uint32_t peek() const { return _next; }
    void     commit() { ++_next; }
    bool     needsReserve(uint32_t seq) const { return seq >= _reserve; }
    uint32_t reserveFor(uint32_t seq) const { return seq + _step; }
    void     reserved(uint32_t value) { _reserve = value; }
    uint32_t reserve() const { return _reserve; }

private:
    uint32_t _next, _reserve, _step;
};

}  // namespace collector
