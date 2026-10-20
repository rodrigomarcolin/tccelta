#include "tasks/FlushTask.h"
#include "collector/Events.h"
#include "rtos/Adapters.h"

namespace {

constexpr uint32_t   kStackBytes = 6144;
constexpr UBaseType_t kPriority  = 4;
constexpr BaseType_t kCore       = 1;

}  // namespace

TaskHandle_t FlushTask::start() { return rtos::startTask(this, "Flush", kStackBytes, kPriority, kCore); }

void FlushTask::run() {
    collector::Block* block = nullptr;
    while (true) {
        if (xQueueReceive(_ctx.fullBlocks, &block, portMAX_DELAY) != pdTRUE) continue;

        const uint32_t startMs = millis();
        const bool written = _writer.write(*block);
        emitFlush(*block, written, startMs);
        xQueueSend(_ctx.freeBlocks, &block, portMAX_DELAY);

        _ctx.counters.failedBlocks = _writer.failedBlocks();
        _ctx.counters.lostBlocks   = _store.lostBlocks();
        if (written) xEventGroupSetBits(_ctx.events, rtos::BLOCKS_PENDING);
    }
}

void FlushTask::emitFlush(const collector::Block& block, bool written, uint32_t startMs) {
    if (!collector::kEventsEnabled) return;
    static char line[collector::Event::kLargeCapacity];  // só esta tarefa usa
    const uint32_t endMs = millis();
    collector::Event("FLUSH", line, sizeof(line))
        .at(endMs)
        .num("buf", _ctx.indexOf(&block))
        .num("seq", collector::blockSeq(block))
        .num("boot", collector::getU16(&block.bytes[collector::header::kBootId]))
        .num("len", static_cast<int64_t>(block.length))
        .flag("ok", written)
        .num("attempts", _writer.lastAttempts())
        .num("dur", endMs - startMs)
        .hex("hex", block.bytes, block.length);
}
