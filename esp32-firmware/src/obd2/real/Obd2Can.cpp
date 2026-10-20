#include "Obd2Can.h"
#include <cstring>

// Convenções OBD-II/SAE-J1979 que o IsoTpClient (genérico, sem conhecimento
// de diagnóstico) precisa receber como parâmetro.
static constexpr uint8_t OBD2_NEGATIVE_RESPONSE_SID = 0x7F;
static constexpr uint8_t SERVICE_CURRENT_DATA = 0x01;
static constexpr uint8_t SERVICE_FREEZE_FRAME = 0x02;

static uint8_t replySid(uint8_t service) { return static_cast<uint8_t>(service | 0x40u); }

// Copia para `target` o payload de `source` sem os `headerLen` bytes iniciais
// (SID, PID, ...). Devolve false se a resposta for curta demais.
static bool copyPayload(const IsoTp::Response& source, size_t headerLen, Obd2Response& target) {
    if (source.len < headerLen) return false;
    target.ecuId = source.ecuId;
    target.len   = source.len - headerLen;
    memcpy(target.data, &source.data[headerLen], target.len);
    return true;
}

static int firstPayload(const Obd2ResponseSet& responses, uint8_t* buf, size_t maxLen) {
    if (responses.count == 0) return -1;
    const Obd2Response& first = responses.items[0];
    const size_t toCopy = first.len < maxLen ? first.len : maxLen;
    memcpy(buf, first.data, toCopy);
    return static_cast<int>(toCopy);
}

Obd2Can::Obd2Can(ICanBus* can, const ObdAddressing& addressing, uint32_t timeoutMs)
    : _can(can), _addr(addressing), _timeoutMs(timeoutMs),
      _isoTp(can, OBD2_NEGATIVE_RESPONSE_SID, addressing.fcTargetFor, addressing.extended) {}

bool Obd2Can::begin() { return _can->begin(); }

int Obd2Can::requestFunctional(const uint8_t* req, size_t reqLen,
                               const IsoTp::ReplyPrefix& reply,
                               IsoTp::ResponseSet& raw, uint32_t timeoutMs,
                               size_t expectedResponses) {
    return _isoTp.requestAll(_addr.functionalRequestId, _addr.responseIdMin, _addr.responseIdMax,
                             req, reqLen, reply, raw, timeoutMs, expectedResponses);
}

// ── Modo 01/09 ───────────────────────────────────────────────────────────

int Obd2Can::readPid(uint8_t service, uint8_t pid, uint8_t* buf, size_t maxLen) {
    Obd2ResponseSet responses;
    if (readPidAll(service, pid, responses, _timeoutMs, 1) < 0) return -1;
    return firstPayload(responses, buf, maxLen);
}

int Obd2Can::readPidAll(uint8_t service, uint8_t pid, Obd2ResponseSet& responses,
                        uint32_t timeoutMs, size_t expectedResponses) {
    responses.count = 0;
    const uint8_t req[2] = {service, pid};
    IsoTp::ResponseSet raw;
    int result = requestFunctional(req, sizeof(req), IsoTp::ReplyPrefix(replySid(service), pid),
                                   raw, timeoutMs, expectedResponses);
    if (result < 0) return result;
    for (size_t i = 0; i < raw.count && responses.count < OBD2_MAX_ECU_RESPONSES; ++i) {
        if (copyPayload(raw.items[i], 2, responses.items[responses.count])) ++responses.count;
    }
    return responses.count > 0 ? static_cast<int>(responses.count) : -1;
}

int Obd2Can::readEnginePid(uint8_t pid, uint8_t* buf, size_t maxLen, uint32_t timeoutMs) {
    const uint8_t req[2] = {SERVICE_CURRENT_DATA, pid};
    IsoTp::ResponseSet raw;
    int result = _isoTp.requestAll(_addr.engineRequestId, _addr.engineResponseId,
                                   _addr.engineResponseId, req, sizeof(req),
                                   IsoTp::ReplyPrefix(replySid(SERVICE_CURRENT_DATA), pid),
                                   raw, timeoutMs, 1);
    if (result < 0 || raw.count == 0) return -1;

    Obd2ResponseSet responses;
    if (!copyPayload(raw.items[0], 2, responses.items[0])) return -1;
    responses.count = 1;
    return firstPayload(responses, buf, maxLen);
}

// ── Modo 03/07/0A ────────────────────────────────────────────────────────

int Obd2Can::readDtc(uint8_t service, uint16_t* dtcCodes, size_t maxCount) {
    Obd2ResponseSet responses;
    if (readDtcAll(service, responses, _timeoutMs, 1) < 0) return -1;

    const Obd2Response& first = responses.items[0];
    const size_t count  = first.data[1];
    const size_t toCopy = count < maxCount ? count : maxCount;
    for (size_t i = 0; i < toCopy; i++) {
        dtcCodes[i] = static_cast<uint16_t>((first.data[2 + 2 * i] << 8) | first.data[3 + 2 * i]);
    }
    return static_cast<int>(toCopy);
}

int Obd2Can::readDtcAll(uint8_t service, Obd2ResponseSet& responses,
                        uint32_t timeoutMs, size_t expectedResponses) {
    responses.count = 0;
    const uint8_t req[1] = {service};
    IsoTp::ResponseSet raw;
    int result = requestFunctional(req, sizeof(req), replySid(service), raw, timeoutMs,
                                   expectedResponses);
    if (result < 0) return result;
    for (size_t i = 0; i < raw.count && responses.count < OBD2_MAX_ECU_RESPONSES; ++i) {
        const IsoTp::Response& source = raw.items[i];
        if (source.len < 2) continue;
        const size_t count = source.data[1];
        if (2 + count * 2 > source.len || 2 + count * 2 > OBD2_MAX_RESPONSE_BYTES) continue;
        Obd2Response& target = responses.items[responses.count++];
        target.ecuId = source.ecuId;
        target.len = 2 + count * 2;
        memcpy(target.data, source.data, target.len);
    }
    return responses.count > 0 ? static_cast<int>(responses.count) : -1;
}

// ── Modo 02 (freeze frame, só o frame 0) ─────────────────────────────────

int Obd2Can::readFreezeFramePid(uint8_t pid, uint8_t* buf, size_t maxLen) {
    Obd2ResponseSet responses;
    if (readFreezeFramePidAll(pid, responses, _timeoutMs, 1) < 0) return -1;
    return firstPayload(responses, buf, maxLen);
}

int Obd2Can::readFreezeFramePidAll(uint8_t pid, Obd2ResponseSet& responses,
                                   uint32_t timeoutMs, size_t expectedResponses) {
    // Resposta: [0x42, PID, frame#, dados...] — 3 bytes de cabeçalho,
    // diferente do Modo 01 (2 bytes: SID+PID).
    responses.count = 0;
    const uint8_t req[3] = {SERVICE_FREEZE_FRAME, pid, 0x00};
    IsoTp::ResponseSet raw;
    int result = requestFunctional(req, sizeof(req),
                                   IsoTp::ReplyPrefix(replySid(SERVICE_FREEZE_FRAME), pid),
                                   raw, timeoutMs, expectedResponses);
    if (result < 0) return result;
    for (size_t i = 0; i < raw.count && responses.count < OBD2_MAX_ECU_RESPONSES; ++i) {
        const IsoTp::Response& source = raw.items[i];
        if (source.len < 3 || source.data[2] != 0x00) continue;
        if (copyPayload(source, 3, responses.items[responses.count])) ++responses.count;
    }
    return responses.count > 0 ? static_cast<int>(responses.count) : -1;
}
