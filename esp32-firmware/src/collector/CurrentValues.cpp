#include "collector/CurrentValues.h"
#include <cstring>

namespace collector {

void CurrentValues::update(const ReadResult& result) {
    if (!result.ok) return;

    Slot* slot = nullptr;
    for (size_t i = 0; i < _count; ++i) {
        if (_slots[i].pid == result.pid) slot = &_slots[i];
    }
    if (!slot) {
        if (_count >= kMaxPids) return;
        slot = &_slots[_count++];
        slot->pid = result.pid;
    }
    memcpy(slot->value.bytes, result.value, sizeof(slot->value.bytes));
    slot->value.len       = result.len;
    slot->value.updatedMs = result.tMs;
}

bool CurrentValues::get(uint8_t pid, Value& out) const {
    for (size_t i = 0; i < _count; ++i) {
        if (_slots[i].pid != pid) continue;
        out = _slots[i].value;
        return true;
    }
    return false;
}

}  // namespace collector
