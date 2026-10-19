#include "rtos/QueuedObd2.h"
#include "rtos/Adapters.h"

namespace rtos {

namespace {

// Cada operação vira um BusJob sem captura; os argumentos e o resultado ficam
// na pilha de quem chamou, que espera a Aquisição terminar.
struct PidArgs {
    uint8_t service, pid;
    uint8_t* buf;
    size_t maxLen;
    Obd2ResponseSet* responses;
    uint32_t timeoutMs;
    size_t expected;
    int result;
};

struct DtcArgs {
    uint8_t service;
    uint16_t* codes;
    size_t maxCount;
    Obd2ResponseSet* responses;
    uint32_t timeoutMs;
    size_t expected;
    int result;
};

}  // namespace

bool QueuedObd2::runOnBus(BusJob job, void* context) {
    AcqRequest request = {};
    request.op = AcqOp::RunOnBus;
    request.job = job;
    request.jobContext = context;
    return callAcquisition(_ctx, request);
}

void QueuedObd2::setResponseTimeoutMs(uint32_t timeoutMs) {
    uint32_t value = timeoutMs;
    runOnBus([](IObd2& obd, void* c) { obd.setResponseTimeoutMs(*static_cast<uint32_t*>(c)); },
             &value);
}

int QueuedObd2::readPid(uint8_t service, uint8_t pid, uint8_t* buf, size_t maxLen) {
    PidArgs a = {service, pid, buf, maxLen, nullptr, 0, 0, -1};
    if (!runOnBus([](IObd2& obd, void* c) {
            auto& a = *static_cast<PidArgs*>(c);
            a.result = obd.readPid(a.service, a.pid, a.buf, a.maxLen);
        }, &a)) return -1;
    return a.result;
}

int QueuedObd2::readPidAll(uint8_t service, uint8_t pid, Obd2ResponseSet& responses,
                           uint32_t timeoutMs, size_t expectedResponses) {
    PidArgs a = {service, pid, nullptr, 0, &responses, timeoutMs, expectedResponses, -1};
    if (!runOnBus([](IObd2& obd, void* c) {
            auto& a = *static_cast<PidArgs*>(c);
            a.result = obd.readPidAll(a.service, a.pid, *a.responses, a.timeoutMs, a.expected);
        }, &a)) return -1;
    return a.result;
}

int QueuedObd2::readDtc(uint8_t service, uint16_t* dtcCodes, size_t maxCount) {
    DtcArgs a = {service, dtcCodes, maxCount, nullptr, 0, 0, -1};
    if (!runOnBus([](IObd2& obd, void* c) {
            auto& a = *static_cast<DtcArgs*>(c);
            a.result = obd.readDtc(a.service, a.codes, a.maxCount);
        }, &a)) return -1;
    return a.result;
}

int QueuedObd2::readDtcAll(uint8_t service, Obd2ResponseSet& responses,
                           uint32_t timeoutMs, size_t expectedResponses) {
    DtcArgs a = {service, nullptr, 0, &responses, timeoutMs, expectedResponses, -1};
    if (!runOnBus([](IObd2& obd, void* c) {
            auto& a = *static_cast<DtcArgs*>(c);
            a.result = obd.readDtcAll(a.service, *a.responses, a.timeoutMs, a.expected);
        }, &a)) return -1;
    return a.result;
}

int QueuedObd2::readFreezeFramePid(uint8_t pid, uint8_t* buf, size_t maxLen) {
    PidArgs a = {0x02, pid, buf, maxLen, nullptr, 0, 0, -1};
    if (!runOnBus([](IObd2& obd, void* c) {
            auto& a = *static_cast<PidArgs*>(c);
            a.result = obd.readFreezeFramePid(a.pid, a.buf, a.maxLen);
        }, &a)) return -1;
    return a.result;
}

int QueuedObd2::readFreezeFramePidAll(uint8_t pid, Obd2ResponseSet& responses,
                                      uint32_t timeoutMs, size_t expectedResponses) {
    PidArgs a = {0x02, pid, nullptr, 0, &responses, timeoutMs, expectedResponses, -1};
    if (!runOnBus([](IObd2& obd, void* c) {
            auto& a = *static_cast<PidArgs*>(c);
            a.result = obd.readFreezeFramePidAll(a.pid, *a.responses, a.timeoutMs, a.expected);
        }, &a)) return -1;
    return a.result;
}

}  // namespace rtos
