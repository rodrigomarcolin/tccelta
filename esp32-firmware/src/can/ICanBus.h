#pragma once
#include <cstdint>

enum class CanBitrate : uint8_t { Kbps250, Kbps500 };

struct CanFrame {
    uint32_t id;
    uint8_t  dlc;
    uint8_t  data[8];
    bool     extended = false;  // true = identificador de 29 bits
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
