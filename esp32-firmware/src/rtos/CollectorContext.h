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
#include "sync/SyncProtocol.h"

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

/** O que a SyncTask recebe (spec fase-1/04 §6): eventos do BLE e da Flush, numa só fila. */
enum class SyncInputKind : uint8_t { Connected, Disconnected, Subscribed, Unsubscribed, Rx, Flushed, InjectAck };

struct SyncInput {
    SyncInputKind kind;
    uint8_t  len;         // Rx: bytes válidos em data
    uint16_t connHandle;  // Connected/Rx
    uint32_t value;       // Flushed: seq; InjectAck: ACK_seq; demais: geração (ctx.connections)
    uint32_t tUs;         // esp_timer (32 bits baixos) no callback: latência de fila
    uint8_t  data[datasync::kSyncRxMax];
};
constexpr UBaseType_t kSyncRxDepth = 8;

constexpr size_t kBlockBuffers = 2;

/** Contadores do AT#LIST, atualizados por várias tarefas. */
struct Counters {
    std::atomic<uint32_t> droppedRecords{0};
    std::atomic<uint32_t> droppedGps{0};
    std::atomic<uint32_t> failedBlocks{0};
    std::atomic<uint32_t> lostBlocks{0};
    std::atomic<uint32_t> syncRxDropped{0};  // fila syncRx cheia (o callback do NimBLE não emite eventos)
};

struct CollectorContext {
    QueueHandle_t      acqRequests;
    QueueHandle_t      freeBlocks;
    QueueHandle_t      fullBlocks;
    QueueHandle_t      syncRx;
    EventGroupHandle_t events;
    std::atomic<uint32_t> connections{0};  // incrementa a cada conexão do app
    std::atomic<bool>     noticePending{false};  // a Flush não conseguiu postar `Flushed`: repetir o aviso
    collector::Block   blocks[kBlockBuffers];
    Counters           counters;

    /** Cria filas e eventos e coloca os buffers de bloco no pool livre. */
    void create();

    int indexOf(const collector::Block* block) const { return static_cast<int>(block - blocks); }

    bool hasBits(EventBits_t bits) const {
        return (xEventGroupGetBits(events) & bits) == bits;
    }
};

}  // namespace rtos
