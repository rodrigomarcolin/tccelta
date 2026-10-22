#include "collector/BlockWriter.h"

namespace collector {

namespace {
constexpr int kAppendAttempts = 2;
}  // namespace

bool BlockWriter::write(Block& block) {
    const uint32_t seq = _seq.peek();
    stampHeader(block, seq, _bootId);

    for (int attempt = 0; attempt < kAppendAttempts; ++attempt) {
        _lastAttempts = attempt + 1;
        if (!_store.append(block)) continue;
        _persistence.saveLastSeq(seq);
        _seq.commit();
        return true;
    }
    ++_failedBlocks;
    return false;
}

}  // namespace collector
