#pragma once
#include "obd2/IObd2.h"

/**
 * Implementação IObd2 mockada.
 * Útil para permitir desenvolvimento somente com o ESP32, sem necessidade da
 * parafernália CAN como ECU (simulado ou não), transceivers etc.
 *
 * Suporta PIDs do service 01:
 *   0x00 (supported PIDs),  0x04 (engine load),  0x05 (coolant temp),
 *   0x0C (RPM),  0x0D (speed),  0x0E (timing advance),  0x11 (throttle)
 */
class Obd2Mock : public IObd2 {
public:
    bool begin() override;
    int  readPid(uint8_t service, uint8_t pid,
                 uint8_t* buf, size_t maxLen) override;
};
