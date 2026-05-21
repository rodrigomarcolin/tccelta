#pragma once
#include <cstdint>

struct CanFrame {
    uint32_t id;
    uint8_t  dlc;
    uint8_t  data[8];
};

/**
 * Abstração da camada CAN.
 *
 * Fornece funções para enviar e receber frames.
 */
class ICanBus {
public:
    virtual ~ICanBus() = default;

    virtual bool begin() = 0;

    /** Transmit a frame. Returns true on success. */
    virtual bool send(const CanFrame& frame) = 0;

    /** Non-blocking receive. Returns true if a frame was available before timeout. */
    virtual bool receive(CanFrame& frame) = 0;
};
