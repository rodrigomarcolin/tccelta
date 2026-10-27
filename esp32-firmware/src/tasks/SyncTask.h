#pragma once
#include "rtos/CollectorContext.h"
#include "rtos/SharedSyncStatus.h"
#include "sync/SyncEngine.h"

/**
 * Casca FreeRTOS do SyncEngine (spec fase-1/05 §10): núcleo 0, prioridade 3.
 * Drena a fila `syncRx` (eventos do BLE e da Flush), envia os blocos do lote
 * ativo e roda o `tick` a cada ~1 s.
 */
class SyncTask {
public:
    SyncTask(rtos::CollectorContext& ctx, datasync::SyncEngine& engine, rtos::SharedSyncStatus& status)
        : _ctx(ctx), _engine(engine), _status(status) {}

    TaskHandle_t start();
    void run();

private:
    rtos::CollectorContext& _ctx;
    datasync::SyncEngine& _engine;
    rtos::SharedSyncStatus& _status;
    uint32_t _reportedDrops = 0;

    void dispatch(const rtos::SyncInput& in);
    void reportQueueDrops();
};
