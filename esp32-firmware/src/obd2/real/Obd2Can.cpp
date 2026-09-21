#include <Arduino.h>
#include "Obd2Can.h"
#include "IsoTpClient.h"
#include <cstring>

// OBD-II CAN IDs (11-bit)
static constexpr uint32_t OBD_REQUEST_ID  = 0x7DF;  // functional broadcast
static constexpr uint32_t OBD_RESPONSE_ID = 0x7E8;  // ECU #1 response
static constexpr uint32_t OBD_RESPONSE_ID_MAX = 0x7EF;  // último ID de resposta física possível

Obd2Can::Obd2Can(ICanBus* can, uint32_t timeoutMs)
    : _can(can), _timeoutMs(timeoutMs) {}

bool Obd2Can::begin() { return _can->begin(); }

int Obd2Can::readPid(uint8_t service, uint8_t pid,
                     uint8_t* buf, size_t maxLen) {
    uint8_t req[2] = { service, pid };
    uint8_t out[7];  // SF payload máximo (7 bytes) cobre SID+PID+dados

    int n = IsoTp::request(_can, OBD_REQUEST_ID, OBD_RESPONSE_ID, OBD_RESPONSE_ID_MAX,
                            req, sizeof(req), out, sizeof(out), _timeoutMs);
    if (n < 0) return -1;  // timeout, negative response ou overflow -> "sem dado"
    if ((size_t)n < 2 || out[0] != (uint8_t)(service | 0x40u) || out[1] != pid) return -1;

    size_t payloadBytes = (size_t)n - 2;
    size_t toCopy        = payloadBytes < maxLen ? payloadBytes : maxLen;
    memcpy(buf, &out[2], toCopy);
    return (int)toCopy;
}

int Obd2Can::readPidAll(uint8_t service, uint8_t pid,
                        Obd2ResponseSet& responses,
                        uint32_t timeoutMs,
                        size_t expectedResponses) {
    responses.count = 0;
    uint8_t req[2] = {service, pid};
    IsoTp::ResponseSet raw;
    int result = IsoTp::requestAll(_can, OBD_REQUEST_ID, OBD_RESPONSE_ID,
                                   OBD_RESPONSE_ID_MAX, req, sizeof(req), raw,
                                   timeoutMs, expectedResponses);
    if (result < 0) return result;
    for (size_t i = 0; i < raw.count && responses.count < OBD2_MAX_ECU_RESPONSES; ++i) {
        const IsoTp::Response& source = raw.items[i];
        if (source.len < 2 || source.data[0] != static_cast<uint8_t>(service | 0x40u) ||
            source.data[1] != pid) continue;
        Obd2Response& target = responses.items[responses.count++];
        target.ecuId = source.ecuId;
        target.len = source.len - 2;
        memcpy(target.data, &source.data[2], target.len);
    }
    return responses.count > 0 ? static_cast<int>(responses.count) : -1;
}

int Obd2Can::readDtc(uint8_t service, uint16_t* dtcCodes, size_t maxCount) {
    uint8_t req[1] = { service };
    uint8_t out[2 + 2 * 32];  // count + até 32 DTCs

    int n = IsoTp::request(_can, OBD_REQUEST_ID, OBD_RESPONSE_ID, OBD_RESPONSE_ID_MAX,
                            req, sizeof(req), out, sizeof(out), _timeoutMs);
    if (n < 0) return -1;  // timeout, negative response ou overflow -> "sem dado"
    if ((size_t)n < 2 || out[0] != (uint8_t)(service | 0x40u)) return -1;

    uint8_t count   = out[1];
    size_t  toCopy  = (size_t)count < maxCount ? count : maxCount;
    for (size_t i = 0; i < toCopy; i++) {
        dtcCodes[i] = (uint16_t)((out[2 + 2 * i] << 8) | out[3 + 2 * i]);
    }
    return (int)toCopy;
}

int Obd2Can::readDtcAll(uint8_t service, Obd2ResponseSet& responses,
                        uint32_t timeoutMs,
                        size_t expectedResponses) {
    responses.count = 0;
    uint8_t req[1] = {service};
    IsoTp::ResponseSet raw;
    int result = IsoTp::requestAll(_can, OBD_REQUEST_ID, OBD_RESPONSE_ID,
                                   OBD_RESPONSE_ID_MAX, req, sizeof(req), raw,
                                   timeoutMs, expectedResponses);
    if (result < 0) return result;
    for (size_t i = 0; i < raw.count && responses.count < OBD2_MAX_ECU_RESPONSES; ++i) {
        const IsoTp::Response& source = raw.items[i];
        if (source.len < 2 || source.data[0] != static_cast<uint8_t>(service | 0x40u)) continue;
        const size_t count = source.data[1];
        if (2 + count * 2 > source.len || 2 + count * 2 > OBD2_MAX_RESPONSE_BYTES) continue;
        Obd2Response& target = responses.items[responses.count++];
        target.ecuId = source.ecuId;
        target.len = 2 + count * 2;
        memcpy(target.data, source.data, target.len);
    }
    return responses.count > 0 ? static_cast<int>(responses.count) : -1;
}

int Obd2Can::readFreezeFramePid(uint8_t pid, uint8_t* buf, size_t maxLen) {
    // Modo 02 real: 3 bytes de requisição (serviço, PID, frame#) — frame#
    // sempre 0, o único suportado pelo simulador/ECU (não há histórico de
    // frames antigos). Resposta: [0x42, PID, frame#, dados...] — 3 bytes de
    // cabeçalho, diferente do Modo 01 (2 bytes: SID+PID).
    uint8_t req[3] = { 0x02, pid, 0x00 };
    uint8_t out[7];  // SF payload máximo (7 bytes) já cobre cabeçalho + dado

    int n = IsoTp::request(_can, OBD_REQUEST_ID, OBD_RESPONSE_ID, OBD_RESPONSE_ID_MAX,
                            req, sizeof(req), out, sizeof(out), _timeoutMs);
    if (n < 0) return -1;  // timeout, negative response (NRC 0x31) ou overflow -> "sem dado"
    if ((size_t)n < 3 || out[0] != 0x42 || out[1] != pid) return -1;

    size_t payloadBytes = (size_t)n - 3;
    size_t toCopy        = payloadBytes < maxLen ? payloadBytes : maxLen;
    memcpy(buf, &out[3], toCopy);
    return (int)toCopy;
}

int Obd2Can::readFreezeFramePidAll(uint8_t pid, Obd2ResponseSet& responses,
                                   uint32_t timeoutMs,
                                   size_t expectedResponses) {
    responses.count = 0;
    uint8_t req[3] = {0x02, pid, 0x00};
    IsoTp::ResponseSet raw;
    int result = IsoTp::requestAll(_can, OBD_REQUEST_ID, OBD_RESPONSE_ID,
                                   OBD_RESPONSE_ID_MAX, req, sizeof(req), raw,
                                   timeoutMs, expectedResponses);
    if (result < 0) return result;
    for (size_t i = 0; i < raw.count && responses.count < OBD2_MAX_ECU_RESPONSES; ++i) {
        const IsoTp::Response& source = raw.items[i];
        if (source.len < 3 || source.data[0] != 0x42 || source.data[1] != pid ||
            source.data[2] != 0x00) continue;
        Obd2Response& target = responses.items[responses.count++];
        target.ecuId = source.ecuId;
        target.len = source.len - 3;
        memcpy(target.data, &source.data[3], target.len);
    }
    return responses.count > 0 ? static_cast<int>(responses.count) : -1;
}
