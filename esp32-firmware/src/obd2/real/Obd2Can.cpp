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

int Obd2Can::readDtc(uint8_t service, uint16_t* dtcCodes, size_t maxCount) {
    uint8_t req[1] = { service };
    uint8_t out[2 + 2 * 32];  // count + até 32 DTCs

    int n = IsoTp::request(_can, OBD_REQUEST_ID, OBD_RESPONSE_ID, OBD_RESPONSE_ID_MAX,
                            req, sizeof(req), out, sizeof(out));
    if (n < 0) return -1;  // timeout, negative response ou overflow -> "sem dado"
    if ((size_t)n < 2 || out[0] != (uint8_t)(service | 0x40u)) return -1;

    uint8_t count   = out[1];
    size_t  toCopy  = (size_t)count < maxCount ? count : maxCount;
    for (size_t i = 0; i < toCopy; i++) {
        dtcCodes[i] = (uint16_t)((out[2 + 2 * i] << 8) | out[3 + 2 * i]);
    }
    return (int)toCopy;
}

int Obd2Can::readFreezeFramePid(uint8_t pid, uint8_t* buf, size_t maxLen) {
    // Modo 02 real: 3 bytes de requisição (serviço, PID, frame#) — frame#
    // sempre 0, o único suportado pelo simulador/ECU (não há histórico de
    // frames antigos). Resposta: [0x42, PID, frame#, dados...] — 3 bytes de
    // cabeçalho, diferente do Modo 01 (2 bytes: SID+PID).
    uint8_t req[3] = { 0x02, pid, 0x00 };
    uint8_t out[7];  // SF payload máximo (7 bytes) já cobre cabeçalho + dado

    int n = IsoTp::request(_can, OBD_REQUEST_ID, OBD_RESPONSE_ID, OBD_RESPONSE_ID_MAX,
                            req, sizeof(req), out, sizeof(out));
    if (n < 0) return -1;  // timeout, negative response (NRC 0x31) ou overflow -> "sem dado"
    if ((size_t)n < 3 || out[0] != 0x42 || out[1] != pid) return -1;

    size_t payloadBytes = (size_t)n - 3;
    size_t toCopy        = payloadBytes < maxLen ? payloadBytes : maxLen;
    memcpy(buf, &out[3], toCopy);
    return (int)toCopy;
}
