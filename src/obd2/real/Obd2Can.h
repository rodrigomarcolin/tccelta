#pragma once
#include "obd2/IObd2.h"
#include "can/ICanBus.h"

/**
 * Implementação IObd2 que se comunica com uma ECU real via CAN
 * usando ISO 15765-4 (OBD-II on CAN).
 *
 * Single-frame responses apenas (sufficient for mode 01 / 09 PIDs).
 * Multi-frame (ISO-TP segmented) responses não estão implementados.
 */
class Obd2Can : public IObd2 {
public:
    /**
     * @param can        Implementação ICanBus (Mcp2515Can ou TwaiCan)
     * @param timeoutMs  Timeout da resposta em Ms (default 200)
     */
    explicit Obd2Can(ICanBus* can, uint32_t timeoutMs = 200);

    bool begin() override;
    int  readPid(uint8_t service, uint8_t pid,
                 uint8_t* buf, size_t maxLen) override;

private:
    ICanBus*  _can;
    uint32_t  _timeoutMs;
};
