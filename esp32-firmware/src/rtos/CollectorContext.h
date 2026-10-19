#pragma once
#include <atomic>
#include <freertos/FreeRTOS.h>
#include <freertos/event_groups.h>
#include <freertos/queue.h>
#include <freertos/semphr.h>
#include <freertos/task.h>
#include "collector/BlockFormat.h"
#include "collector/CollectorConfig.h"
#include "obd2/IObd2.h"

/**
 * Recursos de RTOS compartilhados pelas tarefas da coleta (plano §4.5, §4.6)
 * e os tipos que trafegam neles.
 */
namespace rtos {

/** Bits do grupo de eventos `events`. */
enum EventBit : EventBits_t {
    APP_CONNECTED  = BIT0,
    INTERNET       = BIT1,
    SESSION_OK     = BIT2,
    DIAG_MODE      = BIT3,
    BLOCKS_PENDING = BIT4,
};

/** Trabalho executado pela Aquisição, dona do barramento, em nome de outra tarefa. */
using BusJob = void (*)(IObd2& obd, void* context);

enum class AcqOp : uint8_t { RunOnBus, ApplyConfig, SetMode, Stop, MeasureTmax, Gps };

struct AcqRequest {
    AcqOp op;
    BusJob job;                              // RunOnBus
    void* jobContext;                        // RunOnBus (na pilha de quem espera)
    const collector::CollectorConfig* config;  // ApplyConfig (de quem espera)
    bool diag;                               // SetMode
    collector::GpsFix gps;                   // Gps
    uint32_t gpsTimeMs;                      // Gps: marcado ao receber o comando
    bool* accepted;                          // false = recusado (ex.: medindo Tmax)
    TaskHandle_t requester;                  // notificado ao terminar; nullptr = não espera
};

/** Mensagem recebida no canal de dados, ainda não verificada. */
struct DataMessage {
    uint8_t bytes[64];
    size_t  len;
};

constexpr size_t kBlockBuffers = 2;

/** Contadores do AT#LIST, atualizados por várias tarefas. */
struct Counters {
    std::atomic<uint32_t> droppedRecords{0};
    std::atomic<uint32_t> droppedGps{0};
    std::atomic<uint32_t> failedBlocks{0};
    std::atomic<uint32_t> lostBlocks{0};
};

struct CollectorContext {
    QueueHandle_t      acqRequests;
    QueueHandle_t      freeBlocks;
    QueueHandle_t      fullBlocks;
    QueueHandle_t      uploadRx;
    QueueHandle_t      sessionRx;
    EventGroupHandle_t events;
    collector::Block   blocks[kBlockBuffers];
    Counters           counters;

    /** Cria filas e eventos e coloca os buffers de bloco no pool livre. */
    void create();

    bool hasBits(EventBits_t bits) const {
        return (xEventGroupGetBits(events) & bits) == bits;
    }
};

}  // namespace rtos
