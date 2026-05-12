#pragma once
#include "connectivity/IConnectivity.h"
#include "elm327/Elm327.h"

#include <freertos/FreeRTOS.h>
#include <freertos/queue.h>
#include <freertos/task.h>

// Queue message types 

struct CommandMessage {
    uint8_t data[128];
    size_t  len;
};

struct ResponseMessage {
    uint8_t data[256];
    size_t  len;
};

// Elm327Task

/**
 * Task FreeRTOS que conecta a camada de Connectivity com a camada ELM327 
 *
 * Fluxo:
 *   Comando recebido na IConnectivity => Elm327Task::pushToQueue => [Fila] => Elm327Task::run (reage à chegada na fila) => Elm327::process => IObd2 => ICanBus
 *   Retorno do IObd2 => connectivity->sendResponse()
 */
class Elm327Task {
public:
    /**
     * @param connectivity  Camada de Transporte IConnectivity
     * @param obd2          Camada OBD2
     * @param queueDepth    Qt máxima de comandos pendentes (default 8)
     */
    Elm327Task(IConnectivity* connectivity, IObd2* obd2,
               UBaseType_t queueDepth = 8);

    /**
     * @param priority   FreeRTOS task priority (default 5)
     * @param stackSize  Stack in bytes (default 8192)
     */
    void start(UBaseType_t priority = 5, uint32_t stackSize = 8192);

private:
    IConnectivity* _connectivity;
    Elm327         _elm327;
    QueueHandle_t  _commandQueue;

    static void taskFunc(void* arg);
    void run();
    void pushToQueue(const uint8_t* data, size_t len);
};
