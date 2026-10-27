#include "collector/SeqReserve.h"

namespace collector {

SeqRecovery SeqReserve::recover(uint32_t reserveInNvs, uint32_t legacyLastSeq, uint32_t newestInLog) {
    SeqRecovery r{};
    r.reserveRead = reserveInNvs;
    if (reserveInNvs > 0) {
        r.bootSeq = reserveInNvs;
        r.migrated = false;
    } else {
        r.bootSeq = (legacyLastSeq > newestInLog ? legacyLastSeq : newestInLog) + 1;
        r.migrated = true;
    }
    if (r.bootSeq < newestInLog + 1) r.bootSeq = newestInLog + 1;  // defensivo: nunca reusar seq
    r.fromSeq = newestInLog == 0 ? r.bootSeq : newestInLog + 1;
    r.toSeq = r.bootSeq - 1;
    return r;
}

}  // namespace collector
