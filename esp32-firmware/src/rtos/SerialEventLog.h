#pragma once
#include <atomic>
#include <freertos/FreeRTOS.h>
#include <freertos/ringbuf.h>
#include <freertos/semphr.h>
#include <freertos/task.h>
#include "collector/Events.h"

namespace rtos {

/**
 * Destino dos eventos de depuração (docs/debug-events.md): numera e carimba
 * cada linha ao ser emitida e a põe num ring buffer; uma tarefa de baixa
 * prioridade escreve na serial, para que imprimir não atrase a coleta.
 * Linhas que não cabem no buffer são perdidas (lacuna em `n` + LOG_DROP).
 */
class SerialEventLog : public collector::IEventSink {
public:
    /** Cria o buffer e a tarefa e emite FORMAT. Deve ser o primeiro evento do boot. */
    bool begin();

    void emit(const char* body, size_t len, const uint32_t* tMs) override;

    /** Inclui a tarefa no relatório periódico de pilha (STACK). */
    void watch(const char* name, TaskHandle_t task);

    void run();

private:
    struct Watched {
        const char*  name;
        TaskHandle_t task;
    };
    static constexpr size_t kMaxWatched = 8;

    RingbufHandle_t       _ring  = nullptr;
    SemaphoreHandle_t     _mutex = nullptr;
    uint32_t              _next  = 0;
    std::atomic<uint32_t> _dropped{0};
    Watched               _watched[kMaxWatched] = {};
    size_t                _watchedCount = 0;

    void reportHealth();
};

}  // namespace rtos
