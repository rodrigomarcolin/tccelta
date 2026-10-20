#include "collector/SequentialPidReader.h"
#include <cstring>

namespace collector {

void SequentialPidReader::read(const uint8_t* pids, size_t n, ReadResult* out, uint32_t timeoutMs) {
    for (size_t i = 0; i < n; ++i) {
        ReadResult& result = out[i];
        result = {};
        result.pid = pids[i];

        uint8_t buf[8];
        const int len = _obd.readEnginePid(pids[i], buf, sizeof(buf), timeoutMs);
        result.tMs = _clock.nowMs();
        if (len <= 0) continue;

        result.ok  = true;
        result.len = static_cast<uint8_t>(len < static_cast<int>(kPidValueBytes) ? len : kPidValueBytes);
        memcpy(result.value, buf, result.len);
    }
}

}  // namespace collector
