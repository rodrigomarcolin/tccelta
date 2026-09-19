#include <Arduino.h>
#include "Obd2Can.h"
#include "IsoTpClient.h"
#include <cstring>

// TODO: Rever implementação. Este arquivo foi gerado com IA
// OBD-II CAN IDs (11-bit)
static constexpr uint32_t OBD_REQUEST_ID  = 0x7DF;  // functional broadcast
static constexpr uint32_t OBD_RESPONSE_ID = 0x7E8;  // ECU #1 response
static constexpr uint32_t OBD_RESPONSE_ID_MAX = 0x7EF;  // último ID de resposta física possível

Obd2Can::Obd2Can(ICanBus* can, uint32_t timeoutMs)
    : _can(can), _timeoutMs(timeoutMs) {}

bool Obd2Can::begin() { return _can->begin(); }

int Obd2Can::readPid(uint8_t service, uint8_t pid,
                     uint8_t* buf, size_t maxLen) {
    // Build ISO 15765-4 single-frame request
    CanFrame req = {};
    req.id      = OBD_REQUEST_ID;
    req.dlc     = 8;
    req.data[0] = 0x02;    // PCI: single frame, 2 payload bytes
    req.data[1] = service;
    req.data[2] = pid;
    // bytes [3..7] = 0x00 (padding)

    if (!_can->send(req)) return -1;

    // Poll for matching response within timeout
    uint32_t deadline = millis() + _timeoutMs;
    while (millis() < deadline) {
        CanFrame resp;
        if (!_can->receive(resp)) {
            taskYIELD();  // yield CPU while waiting
            continue;
        }

        // Accept functional response from any ECU (0x7E8–0x7EF)
        if (resp.id < 0x7E8 || resp.id > 0x7EF) continue;

        // Positive response: service byte = request service | 0x40
        uint8_t dataLen = resp.data[0] & 0x0F;  // lower nibble of PCI byte
        if (resp.data[1] != (service | 0x40u)) continue;
        if (resp.data[2] != pid)               continue;

        // data bytes start at resp.data[3]; payload is (dataLen - 2) bytes
        int payloadBytes = dataLen - 2;
        if (payloadBytes <= 0) return 0;

        size_t toCopy = (size_t)payloadBytes < maxLen
                        ? (size_t)payloadBytes : maxLen;
        memcpy(buf, &resp.data[3], toCopy);
        return (int)toCopy;
    }

    return -1;  // timeout
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
