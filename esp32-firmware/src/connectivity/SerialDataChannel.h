#pragma once
#include <Arduino.h>
#include "connectivity/IDataChannel.h"

/**
 * Stub de bancada até o canal BLE existir: imprime cada mensagem em hexa
 * na serial ("[DATA] 01 05 00 ..."), para conferir Upload e Sessão.
 */
class SerialDataChannel : public IDataChannel {
public:
    bool send(const uint8_t* message, size_t len) override {
        Serial.print("[DATA]");
        for (size_t i = 0; i < len; ++i) Serial.printf(" %02X", message[i]);
        Serial.println();
        return true;
    }
};
