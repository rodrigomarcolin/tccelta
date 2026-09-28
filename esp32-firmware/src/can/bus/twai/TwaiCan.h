#pragma once
#include "can/ICanBus.h"
#include <driver/twai.h>

/**
 * Implementação ICanBus usando o periférico TWAI nativo da ESP32.
 * com transceiver externo SN65HVD230 (ou compatível).
 *
 * Pins-padrão são uma atribuição comum ao ESP32. É possível sobrescrever no construtor.
 */
class TwaiCan : public ICanBus {
public:
    /**
     * @param txPin GPIO connected to SN65HVD230 TXD (default 21)
     * @param rxPin GPIO connected to SN65HVD230 RXD (default 22)
     */
    explicit TwaiCan(int txPin = 21, int rxPin = 22);

    bool begin()   override;
    bool send(const CanFrame& frame)    override;
    bool receive(CanFrame& frame)       override;

private:
    int _txPin;
    int _rxPin;
};
