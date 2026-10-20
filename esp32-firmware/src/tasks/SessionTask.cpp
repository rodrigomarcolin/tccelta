#include "tasks/SessionTask.h"
#include "rtos/Adapters.h"

namespace {

constexpr uint32_t   kStackBytes   = 4096;
constexpr UBaseType_t kPriority    = 3;
constexpr BaseType_t kCore         = 0;
constexpr TickType_t kPollTicks    = pdMS_TO_TICKS(500);

}  // namespace

void SessionTask::start() { rtos::startTask(this, "Session", kStackBytes, kPriority, kCore); }

void SessionTask::run() {
    while (true) {
        xEventGroupWaitBits(_ctx.events, rtos::APP_CONNECTED | rtos::INTERNET, pdFALSE, pdTRUE,
                            portMAX_DELAY);

        // TODO(segurança): handshake com o servidor antes de liberar a sessão.
        xEventGroupSetBits(_ctx.events, rtos::SESSION_OK);

        _sync = collector::TimeSync();
        synchronizeTime();
        waitForDisconnect();
    }
}

void SessionTask::synchronizeTime() {
    while (!_sync.done() && _ctx.hasBits(rtos::APP_CONNECTED | rtos::INTERNET)) {
        if (!_sync.pending() || _sync.timedOut(millis())) {
            const size_t len = _protocol.encodeTimeRequest(_sync.start(_bootId, millis()),
                                                           _message, sizeof(_message));
            _channel.send(_message, len);
        }

        rtos::DataMessage message;
        if (xQueueReceive(_ctx.sessionRx, &message, kPollTicks) != pdTRUE) continue;

        collector::TimeExchange response;
        if (_protocol.decodeTimeResponse(message.bytes, message.len, response) &&
            _sync.accept(response, millis())) {
            const size_t len = _protocol.encodeTimeReport(_sync.report(), _message, sizeof(_message));
            _channel.send(_message, len);
        }
    }
}

void SessionTask::waitForDisconnect() {
    while (_ctx.hasBits(rtos::APP_CONNECTED)) vTaskDelay(kPollTicks);
}
