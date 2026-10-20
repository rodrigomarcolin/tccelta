#pragma once
#include "collector/DataProtocol.h"
#include "collector/IBlockStore.h"
#include "collector/UploadCursor.h"
#include "connectivity/IDataChannel.h"
#include "rtos/CollectorContext.h"

/**
 * Envia os blocos da flash ao servidor e apaga os confirmados (núcleo 0,
 * prioridade baixa). Só roda com app conectado, Internet e sessão.
 */
class UploadTask {
public:
    UploadTask(rtos::CollectorContext& ctx, collector::IBlockStore& store,
               collector::DataProtocol& protocol, IDataChannel& channel)
        : _ctx(ctx), _store(store), _protocol(protocol), _channel(channel) {}

    TaskHandle_t start();
    void run();

private:
    rtos::CollectorContext&  _ctx;
    collector::IBlockStore&  _store;
    collector::DataProtocol& _protocol;
    IDataChannel&            _channel;
    collector::UploadCursor  _cursor;
    collector::Block         _block = {};
    uint8_t                  _message[collector::kMaxMessageSize] = {};

    void sendWindow();
    void receiveAck();
    void updatePendingBit();
};
