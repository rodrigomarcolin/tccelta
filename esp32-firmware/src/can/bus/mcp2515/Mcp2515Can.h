#pragma once
#include "can/ICanBus.h"
#include <mcp2515.h>

/**
 * Implementação ICanBus usando o controlador MCP2515 SPI CAN.
 *
 * Conexões:
 *   GPIO18 → SCK,  GPIO19 ← MISO,  GPIO23 → MOSI,  GPIO5 → CS
 *   120 Ω termination between CANH and CANL on both ends (ou seja, conectar o resistor na placa MCP2515).
 *
 * Library: autowp/MCP2515
 */
class Mcp2515Can : public ICanBus {
public:
    /**
     * @param csPin   SPI chip-select GPIO (default 5)
     * @param bitrate CAN_500KBPS for HS-CAN, CAN_250KBPS for MS-CAN
     * @param clock   MCP_8MHZ or MCP_16MHZ depending on crystal
     */
    explicit Mcp2515Can(uint8_t csPin   = 5,
                        CAN_SPEED bitrate = CAN_500KBPS,
                        CAN_CLOCK clock   = MCP_8MHZ);

    bool begin()   override;
    bool send(const CanFrame& frame)    override;
    bool receive(CanFrame& frame)       override;

private:
    MCP2515   _mcp;
    CAN_SPEED _bitrate;
    CAN_CLOCK _clock;
};
