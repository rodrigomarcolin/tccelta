#include "rtos/CollectorContext.h"

namespace rtos {

void CollectorContext::create() {
    acqRequests = xQueueCreate(4, sizeof(AcqRequest));
    freeBlocks  = xQueueCreate(kBlockBuffers, sizeof(collector::Block*));
    fullBlocks  = xQueueCreate(kBlockBuffers, sizeof(collector::Block*));
    uploadRx    = xQueueCreate(4, sizeof(DataMessage));
    sessionRx   = xQueueCreate(2, sizeof(DataMessage));
    events      = xEventGroupCreate();
    configASSERT(acqRequests && freeBlocks && fullBlocks && uploadRx && sessionRx && events);

    for (collector::Block& block : blocks) {
        collector::Block* pointer = &block;
        xQueueSend(freeBlocks, &pointer, 0);
    }
}

}  // namespace rtos
