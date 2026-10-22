#pragma once
#include "collector/DataProtocol.h"
#include "collector/TimeSync.h"
#include "connectivity/IDataChannel.h"
#include "rtos/CollectorContext.h"

/**
 * Sessão com o servidor (núcleo 0): liga `SESSION_OK` e sincroniza a hora
 * uma vez por conexão. O handshake de segurança entra aqui.
 */
class SessionTask {
public:
    SessionTask(rtos::CollectorContext& ctx, collector::DataProtocol& protocol,
                IDataChannel& channel, uint16_t bootId)
        : _ctx(ctx), _protocol(protocol), _channel(channel), _bootId(bootId) {}

    TaskHandle_t start();
    void run();

private:
    rtos::CollectorContext&  _ctx;
    collector::DataProtocol& _protocol;
    IDataChannel&            _channel;
    uint16_t                 _bootId;
    collector::TimeSync      _sync;
    uint8_t                  _message[32] = {};

    void synchronizeTime();
    void waitForDisconnect();
};
