#pragma once
#include "obd2/IObd2.h"

/**
 * Implementação IObd2 mockada.
 * Útil para permitir desenvolvimento somente com o ESP32, sem necessidade da
 * parafernália CAN como ECU (simulado ou não), transceivers etc.
 *
 * Simula um veículo leve a gasolina, cobrindo os PIDs do service 01 (Modo 01 /
 * SAE J1979) aplicáveis a esse perfil de veículo, na faixa 0x00-0x67. PIDs
 * fora desse conjunto (específicos de diesel/caminhão pesado, sensores
 * duplicados/inexistentes neste veículo simulado, etc.) retornam -1 ("NO
 * DATA"). Os bitmasks de "PIDs supported" (0x00, 0x20, 0x40, 0x60) são
 * derivados automaticamente a partir dos PIDs de fato implementados, nunca
 * mantidos à mão em paralelo.
 */
class Obd2Mock : public IObd2 {
public:
    bool begin() override;
    int  readPid(uint8_t service, uint8_t pid,
                 uint8_t* buf, size_t maxLen) override;
};
