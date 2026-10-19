#include "tasks/FlushTask.h"
#include "rtos/Adapters.h"

namespace {

constexpr uint32_t   kStackBytes = 6144;
constexpr UBaseType_t kPriority  = 4;
constexpr BaseType_t kCore       = 1;

}  // namespace

void FlushTask::start() { rtos::startTask(this, "Flush", kStackBytes, kPriority, kCore); }

void FlushTask::run() {
    collector::Block* block = nullptr;
    while (true) {
        if (xQueueReceive(_ctx.fullBlocks, &block, portMAX_DELAY) != pdTRUE) continue;

        const bool written = _writer.write(*block);
        xQueueSend(_ctx.freeBlocks, &block, portMAX_DELAY);

        _ctx.counters.failedBlocks = _writer.failedBlocks();
        _ctx.counters.lostBlocks   = _store.lostBlocks();
        if (written) xEventGroupSetBits(_ctx.events, rtos::BLOCKS_PENDING);
    }
}
