#include "rtos/SerialEventLog.h"
#include <Arduino.h>
#include <cstring>
#include "rtos/Adapters.h"

namespace rtos {

namespace {

constexpr size_t      kRingBytes      = 32 * 1024;
constexpr uint32_t    kStackBytes     = 4096;
constexpr UBaseType_t kPriority       = 1;
constexpr BaseType_t  kCore           = 0;
constexpr uint32_t    kHealthPeriodMs = 60000;
constexpr int         kFormatVersion  = 1;

}  // namespace

bool SerialEventLog::begin() {
    _ring  = xRingbufferCreate(kRingBytes, RINGBUF_TYPE_NOSPLIT);
    _mutex = xSemaphoreCreateMutex();
    if (!_ring || !_mutex) return false;

    collector::setEventSink(this);
    collector::Event("FORMAT").num("v", kFormatVersion);
    watch("logger", startTask(this, "EventLog", kStackBytes, kPriority, kCore));
    return true;
}

void SerialEventLog::watch(const char* name, TaskHandle_t task) {
    if (_watchedCount < kMaxWatched) _watched[_watchedCount++] = {name, task};
}

void SerialEventLog::emit(const char* body, size_t len, const uint32_t* tMs) {
    // Numeração, carimbo e envio sob o mesmo mutex: a ordem das linhas na
    // serial é a ordem de `n`.
    xSemaphoreTake(_mutex, portMAX_DELAY);
    const uint32_t n = _next++;
    const uint32_t t = tMs ? *tMs : millis();

    char prefix[32];
    const int prefixLen = snprintf(prefix, sizeof(prefix), "EV %lu %lu ",
                                   static_cast<unsigned long>(n), static_cast<unsigned long>(t));
    const size_t total = static_cast<size_t>(prefixLen) + len + 2;

    void* slot = nullptr;
    if (xRingbufferSendAcquire(_ring, &slot, total, 0) == pdTRUE) {
        char* out = static_cast<char*>(slot);
        memcpy(out, prefix, prefixLen);
        memcpy(out + prefixLen, body, len);
        memcpy(out + prefixLen + len, "\r\n", 2);
        xRingbufferSendComplete(_ring, slot);
    } else {
        ++_dropped;  // `n` já foi consumido: a lacuna aparece para o visualizador
    }
    xSemaphoreGive(_mutex);
}

void SerialEventLog::run() {
    uint32_t lastHealthMs = millis();
    while (true) {
        size_t size = 0;
        void* item = xRingbufferReceive(_ring, &size, pdMS_TO_TICKS(1000));
        if (item) {
            Serial.write(static_cast<const uint8_t*>(item), size);
            vRingbufferReturnItem(_ring, item);
        }

        const uint32_t dropped = _dropped.exchange(0);
        if (dropped > 0) collector::Event("LOG_DROP").num("count", dropped);

        if (millis() - lastHealthMs >= kHealthPeriodMs) {
            lastHealthMs = millis();
            reportHealth();
        }
    }
}

void SerialEventLog::reportHealth() {
    for (size_t i = 0; i < _watchedCount; ++i) {
        // No ESP-IDF, o high-water mark já vem em bytes.
        collector::Event("STACK").token("task", _watched[i].name)
            .num("free", uxTaskGetStackHighWaterMark(_watched[i].task));
    }
    collector::Event("HEAP").num("free", esp_get_free_heap_size())
        .num("min", esp_get_minimum_free_heap_size());
}

}  // namespace rtos
