#include <Arduino.h>
#include "Obd2Can.h"
#include <cstring>

// TODO: Rever implementação. Este arquivo foi gerado com IA
// OBD-II CAN IDs (11-bit)
static constexpr uint32_t OBD_REQUEST_ID  = 0x7DF;  // functional broadcast
static constexpr uint32_t OBD_RESPONSE_ID = 0x7E8;  // ECU #1 response

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
