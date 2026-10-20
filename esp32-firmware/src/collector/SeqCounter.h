#pragma once
#include <cstdint>

namespace collector {

/**
 * Dono do número de sequência dos blocos. Só avança depois que o bloco foi
 * gravado (`commit`), então uma gravação que falha não abre lacuna no meio
 * da sequência.
 */
class SeqCounter {
public:
    explicit SeqCounter(uint32_t next = 1) : _next(next) {}

    /**
     * Próximo `seq` após um boot: um a mais que o maior entre o último
     * gravado segundo a NVS e o último presente no log. Zero = "nenhum".
     */
    static SeqCounter recover(uint32_t lastSeqInNvs, uint32_t newestSeqInLog) {
        const uint32_t last = lastSeqInNvs > newestSeqInLog ? lastSeqInNvs : newestSeqInLog;
        return SeqCounter(last + 1);
    }

    uint32_t peek() const { return _next; }
    void commit() { ++_next; }

private:
    uint32_t _next;
};

}  // namespace collector
