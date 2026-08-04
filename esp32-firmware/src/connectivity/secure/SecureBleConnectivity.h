#pragma once
#include <cstring>
#include <freertos/FreeRTOS.h>
#include <freertos/queue.h>
#include <freertos/task.h>
#include "connectivity/IConnectivity.h"

/**
 * Decorador para IConnectivity.
 *
 * Encapsula qualquer IConnectivity*, interceptando os métodos sendResponse() e
 * setOnCommandReceivedCallback() para aplicar autenticação ou encriptação
 * de maneira transparente.
 *
 * Intercepta a mensagem recebida, colocando-a na fila para desencriptação antes de repassar
 * para o callback EML327 registrado.
 * 
 * Intercepta a mensagem a ser enviada, encriptando-a antes do envio.
 * 
 *   IConnectivity
 *   ├── BleConnectivity           (transporte puro; já implementado)
 *   └── SecureBleConnectivity     (este arquivo)
 *
 * Como utilizar (na main.cpp):
 *   BleConnectivity       raw("OBD2Dongle");
 *   Elm327Task            task(&secure, &obd2);  // segurança é transparente
 *
 * Fluxo interno:
 *   BleConnectivity::onWrite()
 *       └─► _rawQueue          (callback leve, só enfileira)
 *               └─► [_secureTask]  decrypt → _userCallback(plaintext)
 *                                               └─► [Elm327Task queue]
 * 
 */

static constexpr size_t kSecureMaxMsgLen   = 512;
static constexpr size_t kSecureQueueDepth  = 8;
static constexpr uint32_t kSecureTaskStack = 4096;
static constexpr UBaseType_t kSecureTaskPriority = 5;

class SecureBleConnectivity : public IConnectivity {
public:
    explicit SecureBleConnectivity(IConnectivity* inner);
    ~SecureBleConnectivity() override;
    
    bool begin() override;
    void setOnCommandReceivedCallback(CommandCallback cb) override;
    void sendResponse(const uint8_t* data, size_t len) override;
    void* getSessionContext() override;

private:
    IConnectivity*  _inner;
    CommandCallback _userCallback;
    QueueHandle_t   _rawQueue  = nullptr;
    TaskHandle_t    _taskHandle = nullptr;

    struct RawMessage {
        uint8_t buf[kSecureMaxMsgLen];
        size_t  len;
    };

    static void secureTaskEntry(void* arg);
    void secureTask();
};