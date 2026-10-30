#include "tasks/FlushTask.h"
#include <esp_timer.h>
#include "collector/Events.h"
#include "rtos/Adapters.h"
#include "security/SecurityProfile.h"

namespace {

// ECDSA (perfil asym) usa mais pilha que o HMAC (spec fase-1/01 §6).
constexpr uint32_t   kStackBytes = security::kProfile == security::Profile::Asym ? 12288 : 8192;
constexpr UBaseType_t kPriority  = 4;
constexpr BaseType_t kCore       = 1;

}  // namespace

TaskHandle_t FlushTask::start() { return rtos::startTask(this, "Flush", kStackBytes, kPriority, kCore); }

void FlushTask::run() {
    collector::Block* block = nullptr;
    while (true) {
        if (xQueueReceive(_ctx.fullBlocks, &block, portMAX_DELAY) != pdTRUE) continue;

        const uint32_t startMs = millis();
        collector::WriteReport report;
        const bool written = _writer.write(*block, report);
        emitFlush(_ctx.indexOf(block), *block, report, startMs);
        xQueueSend(_ctx.freeBlocks, &block, portMAX_DELAY);

        _ctx.counters.failedBlocks = _writer.failedBlocks();
        _ctx.counters.lostBlocks   = _store.lostBlocks();
        if (written) postFlushed(report.seq);
    }
}

void FlushTask::postFlushed(uint32_t seq) {
    rtos::SyncInput input = {};
    input.kind  = rtos::SyncInputKind::Flushed;
    input.value = seq;
    // Fila cheia: a SyncTask repete o aviso quando drenar (noticePending).
    if (xQueueSend(_ctx.syncRx, &input, 0) != pdTRUE) _ctx.noticePending = true;
}

void FlushTask::emitFlush(int buf, const collector::Block& block, const collector::WriteReport& report,
                          uint32_t startMs) {
    if (!collector::kEventsEnabled) return;
    static char line[collector::Event::kLargeCapacity];  // só esta tarefa (ou o boot, antes das tarefas) usa
    const uint32_t endMs = millis();
    const bool full = collector::kEventsHexEnabled;
    collector::Event e("FLUSH", line, sizeof(line));
    e.at(endMs);
    if (report.boot) e.absent("buf");
    else             e.num("buf", buf);
    e.token("kind", report.boot ? "boot" : "data")
        .num("seq", report.seq)
        .num("boot", collector::getU16(&block.bytes[collector::header::kBootId]))
        .num("len", static_cast<int64_t>(block.length))
        .num("tag_len", block.tagLength)
        .flag("ok", report.ok)
        .num("attempts", report.attempts)
        .flag("reserved", report.reserved)
        .us("reserve_us", report.reserveUs)
        .us("sign_us", report.signUs)
        .us("crc_us", report.crcUs)
        .us("write_us", report.writeUs)
        .num("dur", endMs - startMs);
    // CRC como valor u32 em 8 dígitos hexa, mais significativo primeiro.
    const uint8_t crc[4] = {static_cast<uint8_t>(block.crc >> 24), static_cast<uint8_t>(block.crc >> 16),
                            static_cast<uint8_t>(block.crc >> 8), static_cast<uint8_t>(block.crc)};
    e.hex("crc", crc, 4);
    if (full) e.hex("hex", block.bytes, block.length);
    else      e.absent("hex");
    if (full && block.tagLength > 0) e.hex("tag", block.tag, block.tagLength);
    else                             e.absent("tag");
}
