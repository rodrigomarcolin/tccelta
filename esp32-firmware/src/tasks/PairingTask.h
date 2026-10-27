#pragma once
#include <freertos/FreeRTOS.h>
#include <freertos/task.h>
#include "connectivity/pairing/PairButton.h"
#include "connectivity/pairing/PairingConfig.h"
#include "connectivity/pairing/PairingController.h"

/**
 * Amostra o botão de pareamento (GPIO0/BOOT) a cada 20 ms, aciona a janela
 * pelo PairingController e anima o LED opcional (spec fase-2/02 §4.5). Núcleo
 * 0, prioridade 2: fica abaixo do protocolo com o app e longe da coleta.
 */
class PairingTask {
public:
    PairingTask(pairing::PairingController& controller, const PairingTaskConfig& config);

    TaskHandle_t start();
    void run();

private:
    pairing::PairingController& _controller;
    PairingTaskConfig           _config;
    pairing::PairButton         _button;

    void updateLed(uint32_t nowMs);
};
