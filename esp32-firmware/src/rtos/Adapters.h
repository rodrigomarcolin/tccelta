#pragma once
#include <Arduino.h>
#include "collector/Clock.h"
#include "collector/Collector.h"
#include "collector/CurrentValues.h"
#include "collector/TmaxMeter.h"
#include "rtos/CollectorContext.h"

/** Adaptadores das interfaces do núcleo da coleta para o ESP32/FreeRTOS. */
namespace rtos {

class ArduinoClock : public collector::IClock {
public:
    uint32_t nowMs() const override { return millis(); }
};

/** Buffer duplo sobre `freeBlocks` / `fullBlocks`. */
class QueueBlockPool : public collector::IBlockPool {
public:
    explicit QueueBlockPool(CollectorContext& ctx) : _ctx(ctx) {}

    collector::Block* acquire() override {
        collector::Block* block = nullptr;
        return xQueueReceive(_ctx.freeBlocks, &block, 0) == pdTRUE ? block : nullptr;
    }
    void submit(collector::Block* block) override { xQueueSend(_ctx.fullBlocks, &block, portMAX_DELAY); }
    void release(collector::Block* block) override { xQueueSend(_ctx.freeBlocks, &block, portMAX_DELAY); }

    /** Os dois buffers estão livres = tudo o que foi coletado já está na flash. */
    bool allFree() const { return uxQueueMessagesWaiting(_ctx.freeBlocks) == kBlockBuffers; }

private:
    CollectorContext& _ctx;
};

/** Valores correntes compartilhados: a Aquisição escreve, o ELM327 lê. */
class SharedValues : public collector::IValueSink {
public:
    SharedValues() : _mutex(xSemaphoreCreateMutex()) { configASSERT(_mutex); }

    void update(const collector::ReadResult& result) override {
        xSemaphoreTake(_mutex, portMAX_DELAY);
        _values.update(result);
        xSemaphoreGive(_mutex);
    }

    bool get(uint8_t pid, collector::CurrentValues::Value& out) const {
        xSemaphoreTake(_mutex, portMAX_DELAY);
        const bool found = _values.get(pid, out);
        xSemaphoreGive(_mutex);
        return found;
    }

private:
    SemaphoreHandle_t        _mutex;
    collector::CurrentValues _values;
};

/** Resultado da medição de Tmax: a Aquisição escreve, o ELM327 lê. */
class SharedTmaxReport {
public:
    SharedTmaxReport() : _mutex(xSemaphoreCreateMutex()) { configASSERT(_mutex); }

    void set(const collector::TmaxReport& report) {
        xSemaphoreTake(_mutex, portMAX_DELAY);
        _report = report;
        xSemaphoreGive(_mutex);
    }

    collector::TmaxReport get() const {
        xSemaphoreTake(_mutex, portMAX_DELAY);
        const collector::TmaxReport copy = _report;
        xSemaphoreGive(_mutex);
        return copy;
    }

private:
    SemaphoreHandle_t     _mutex;
    collector::TmaxReport _report;
};

/** Cria uma tarefa que roda `self->run()` no núcleo indicado. */
template <typename Task>
void startTask(Task* self, const char* name, uint32_t stackBytes, UBaseType_t priority, BaseType_t core) {
    xTaskCreatePinnedToCore([](void* arg) { static_cast<Task*>(arg)->run(); }, name, stackBytes,
                            self, priority, nullptr, core);
}

/**
 * Envia um pedido à Aquisição e espera ela terminar. Só pode ser usado por
 * uma tarefa (o `Elm327Task`), que processa um comando por vez.
 * @return false se a fila não aceitou o pedido a tempo ou ele foi recusado.
 */
inline bool callAcquisition(CollectorContext& ctx, AcqRequest request,
                            TickType_t enqueueTimeout = pdMS_TO_TICKS(100)) {
    bool accepted = true;
    request.accepted  = &accepted;
    request.requester = xTaskGetCurrentTaskHandle();
    if (xQueueSend(ctx.acqRequests, &request, enqueueTimeout) != pdTRUE) return false;
    ulTaskNotifyTake(pdTRUE, portMAX_DELAY);
    return accepted;
}

/** Envia um pedido sem esperar (GPS, início da medição de Tmax). */
inline bool postAcquisition(CollectorContext& ctx, AcqRequest request) {
    request.accepted  = nullptr;
    request.requester = nullptr;
    return xQueueSend(ctx.acqRequests, &request, 0) == pdTRUE;
}

}  // namespace rtos
