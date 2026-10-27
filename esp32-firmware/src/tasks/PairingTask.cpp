#include "tasks/PairingTask.h"
#include <Arduino.h>
#include "rtos/Adapters.h"

namespace {
constexpr uint32_t kStackBytes = 4096;
constexpr UBaseType_t kPriority = 2;
constexpr BaseType_t kCore = 0;
}  // namespace

PairingTask::PairingTask(pairing::PairingController& controller, const PairingTaskConfig& config)
    : _controller(controller), _config(config), _button(config.debounceMs, config.holdMs) {}

TaskHandle_t PairingTask::start() {
    pinMode(_config.pin, INPUT_PULLUP);
    if (_config.ledPin >= 0) {
        pinMode(_config.ledPin, OUTPUT);
        digitalWrite(_config.ledPin, LOW);
    }
    return rtos::startTask(this, "Pairing", kStackBytes, kPriority, kCore);
}

void PairingTask::run() {
    TickType_t last = xTaskGetTickCount();
    const TickType_t period = pdMS_TO_TICKS(_config.periodMs);
    for (;;) {
        const uint32_t now = millis();
        const bool level = digitalRead(_config.pin) != 0;
        const bool pressed = _config.activeLow ? !level : level;
        if (_button.update(pressed, now) == pairing::PairButton::Edge::HoldFired) _controller.onButtonHold();
        _controller.tick();
        updateLed(now);
        vTaskDelayUntil(&last, period);
    }
}

/** 5 Hz contando o botão, 2 Hz com a janela aberta, aceso com sessão autorizada. */
void PairingTask::updateLed(uint32_t nowMs) {
    if (_config.ledPin < 0) return;
    bool on = false;
    if (_button.counting()) {
        on = (nowMs / 100) % 2 == 0;
    } else if (_controller.windowOpen()) {
        on = (nowMs / 250) % 2 == 0;
    } else if (_controller.authorized()) {
        on = true;
    }
    digitalWrite(_config.ledPin, on ? HIGH : LOW);
}
