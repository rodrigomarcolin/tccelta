#include "Elm327Task.h"
#include <Arduino.h>
#include <cstring>

Elm327Task::Elm327Task(IConnectivity* connectivity, IObd2* obd2,
                       UBaseType_t queueDepth)
    : _connectivity(connectivity), _elm327(obd2) {
    _commandQueue = xQueueCreate(queueDepth, sizeof(CommandMessage));
    configASSERT(_commandQueue);
}

void Elm327Task::start(UBaseType_t priority, uint32_t stackSize) {
    // Registra o callback que coloca o comando recebido na fila para ser processado.
    // O callback no contexto da conectividade deve ser leve e rápido, por isso somente coloca na fila.
    _connectivity->setOnCommandReceivedCallback([this](const uint8_t* data, size_t len) {
        Serial.print("[ELM327] Command received: ");
        Serial.write(data, len);
        Serial.println();
        this->pushToQueue(data, len);
    });

    xTaskCreate(Elm327Task::taskFunc, "Elm327Task",
                stackSize, this, priority, nullptr);
}

// Internal helpers

void Elm327Task::pushToQueue(const uint8_t* data, size_t len) {
    CommandMessage msg = {};
    msg.len = len < sizeof(msg.data) ? len : sizeof(msg.data) - 1;
    memcpy(msg.data, data, msg.len);
    msg.data[msg.len] = '\0';  // null-terminate for String construction

    // Dropa se fila cheia (back-pressure, ECU lenta)
    xQueueSend(_commandQueue, &msg, 0);
}

// FreeRTOS task

void Elm327Task::taskFunc(void* arg) {
    static_cast<Elm327Task*>(arg)->run();
    vTaskDelete(nullptr);  // em teoria, esta linha é inalcançável
}

void Elm327Task::run() {
    CommandMessage msg;
    while (true) {
        if (xQueueReceive(_commandQueue, &msg, portMAX_DELAY) != pdTRUE) continue;
        // Quando há mensagem na fila, repassa para o processamento do ELM327; em seguida,
        // chama a função do _connectivity para retornar a resposta =)

        String cmd(reinterpret_cast<const char*>(msg.data), msg.len);
        String response = _elm327.process(cmd);

        _connectivity->sendResponse(
            reinterpret_cast<const uint8_t*>(response.c_str()),
            response.length());
    }
}
