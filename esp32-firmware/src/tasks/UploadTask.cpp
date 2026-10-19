#include "tasks/UploadTask.h"
#include "rtos/Adapters.h"

namespace {

constexpr uint32_t   kStackBytes  = 6144;
constexpr UBaseType_t kPriority   = 2;
constexpr BaseType_t kCore        = 0;
constexpr TickType_t kAckPollTicks = pdMS_TO_TICKS(200);

constexpr EventBits_t kReadyBits =
    rtos::APP_CONNECTED | rtos::INTERNET | rtos::SESSION_OK | rtos::BLOCKS_PENDING;

}  // namespace

void UploadTask::start() { rtos::startTask(this, "Upload", kStackBytes, kPriority, kCore); }

void UploadTask::run() {
    while (true) {
        xEventGroupWaitBits(_ctx.events, kReadyBits, pdFALSE, pdTRUE, portMAX_DELAY);

        sendWindow();
        receiveAck();

        if (_cursor.timedOut(millis()) || !_ctx.hasBits(rtos::APP_CONNECTED)) _cursor.rewind();
        updatePendingBit();
    }
}

void UploadTask::sendWindow() {
    if (_store.empty()) return;
    const uint32_t oldest = _store.oldestSeq();
    _cursor.resync(oldest);

    while (_cursor.canSend(_store.newestSeq())) {
        const uint32_t seq = _cursor.takeNext(millis());
        if (!_store.read(seq, _block)) continue;
        const size_t len = _protocol.encodeBatch(oldest, _block, _message, sizeof(_message));
        if (len == 0 || !_channel.send(_message, len)) {
            _cursor.rewind();
            return;
        }
    }
}

void UploadTask::receiveAck() {
    rtos::DataMessage message;
    if (xQueueReceive(_ctx.uploadRx, &message, kAckPollTicks) != pdTRUE) return;

    uint32_t seq;
    if (_protocol.decodeAck(message.bytes, message.len, seq) && _cursor.onAck(seq, millis())) {
        _store.deleteUpTo(seq);
    }
}

void UploadTask::updatePendingBit() {
    if (!_cursor.caughtUp(_store.newestSeq())) return;
    // Desliga primeiro e confere de novo: um bloco gravado nesse meio tempo
    // religa o bit em vez de ser esquecido.
    xEventGroupClearBits(_ctx.events, rtos::BLOCKS_PENDING);
    if (!_cursor.caughtUp(_store.newestSeq())) xEventGroupSetBits(_ctx.events, rtos::BLOCKS_PENDING);
}
