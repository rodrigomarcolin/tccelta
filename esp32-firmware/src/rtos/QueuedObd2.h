#pragma once
#include "obd2/IObd2.h"
#include "rtos/CollectorContext.h"

namespace rtos {

/**
 * IObd2 que executa cada operação na tarefa de Aquisição, a única que usa o
 * barramento. Mantém o código de diagnóstico do `Elm327` como está.
 */
class QueuedObd2 : public IObd2 {
public:
    explicit QueuedObd2(CollectorContext& ctx) : _ctx(ctx) {}

    bool begin() override { return true; }
    void setResponseTimeoutMs(uint32_t timeoutMs) override;

    int readPid(uint8_t service, uint8_t pid, uint8_t* buf, size_t maxLen) override;
    int readPidAll(uint8_t service, uint8_t pid, Obd2ResponseSet& responses,
                   uint32_t timeoutMs = 200, size_t expectedResponses = 0) override;
    int readDtc(uint8_t service, uint16_t* dtcCodes, size_t maxCount) override;
    int readDtcAll(uint8_t service, Obd2ResponseSet& responses,
                   uint32_t timeoutMs = 200, size_t expectedResponses = 0) override;
    int readFreezeFramePid(uint8_t pid, uint8_t* buf, size_t maxLen) override;
    int readFreezeFramePidAll(uint8_t pid, Obd2ResponseSet& responses,
                              uint32_t timeoutMs = 200, size_t expectedResponses = 0) override;

private:
    CollectorContext& _ctx;

    /** Executa `job(obd, context)` na Aquisição; false se não foi aceito. */
    bool runOnBus(BusJob job, void* context);
};

}  // namespace rtos
