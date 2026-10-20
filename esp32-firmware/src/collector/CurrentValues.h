#pragma once
#include <cstddef>
#include <cstdint>
#include "collector/CollectorConfig.h"
#include "collector/PidReader.h"

namespace collector {

/**
 * Último valor recebido de cada PID (parte "valor" da CVT). Sem lock: quem
 * compartilha entre tarefas é `SharedValues`.
 */
class CurrentValues {
public:
    struct Value {
        uint8_t  bytes[kPidValueBytes];
        uint8_t  len;
        uint32_t updatedMs;
    };

    void update(const ReadResult& result);
    bool get(uint8_t pid, Value& out) const;
    void clear() { _count = 0; }

private:
    struct Slot {
        uint8_t pid;
        Value   value;
    };

    Slot   _slots[kMaxPids] = {};
    size_t _count = 0;
};

}  // namespace collector
