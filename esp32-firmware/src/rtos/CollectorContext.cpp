#include "rtos/CollectorContext.h"

namespace rtos {

void CollectorContext::create() {
    acqRequests = xQueueCreate(4, sizeof(AcqRequest));
    freeBlocks  = xQueueCreate(kBlockBuffers, sizeof(collector::Block*));
    fullBlocks  = xQueueCreate(kBlockBuffers, sizeof(collector::Block*));
    syncRx      = xQueueCreate(kSyncRxDepth, sizeof(SyncInput));
    events      = xEventGroupCreate();
    configASSERT(acqRequests && freeBlocks && fullBlocks && syncRx && events);

    for (collector::Block& block : blocks) {
        collector::Block* pointer = &block;
        xQueueSend(freeBlocks, &pointer, 0);
    }
}

}  // namespace rtos
