#pragma once
#include <freertos/FreeRTOS.h>
#include <freertos/semphr.h>
#include "sync/SyncEngine.h"

namespace rtos {

/** Estado do SyncEngine para o AT#SYNC?: a SyncTask escreve, o ELM327 lê (como SharedTmaxReport). */
class SharedSyncStatus {
public:
    SharedSyncStatus() : _mutex(xSemaphoreCreateMutex()) { configASSERT(_mutex); }

    void set(const datasync::SyncStatus& status) {
        xSemaphoreTake(_mutex, portMAX_DELAY);
        _status = status;
        xSemaphoreGive(_mutex);
    }

    datasync::SyncStatus get() const {
        xSemaphoreTake(_mutex, portMAX_DELAY);
        const datasync::SyncStatus copy = _status;
        xSemaphoreGive(_mutex);
        return copy;
    }

private:
    SemaphoreHandle_t _mutex;
    datasync::SyncStatus _status;
};

}  // namespace rtos
