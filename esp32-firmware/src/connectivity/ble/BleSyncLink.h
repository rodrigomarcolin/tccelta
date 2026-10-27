#pragma once
#include <atomic>
#include <freertos/FreeRTOS.h>
#include <freertos/semphr.h>
#include <NimBLEDevice.h>
#include "rtos/CollectorContext.h"
#include "sync/ISyncLink.h"

#ifndef SYNC_TX_MAX_CREDITS
#define SYNC_TX_MAX_CREDITS 4
#endif

/**
 * Transporte do serviço SYNC sobre NimBLE (spec fase-1/04 §7): `ISyncLink`
 * com controle de fluxo por créditos de notificação e a ponte NimBLE ->
 * fila `syncRx`. Os callbacks (`on*`) rodam na tarefa do NimBLE e só copiam
 * e enfileiram; `send()` é chamado só pela SyncTask.
 */
class BleSyncLink final : public datasync::ISyncLink {
public:
    explicit BleSyncLink(rtos::CollectorContext& ctx);

    void attach(NimBLEServer* server, NimBLECharacteristic* tx);

    // Tarefa do NimBLE (via BleConnectivity):
    void onConnected(uint16_t connHandle);
    void onDisconnected();
    void onSubscribed(uint16_t subValue);
    void onMtuChange(uint16_t mtu);
    void onRx(const uint8_t* data, size_t len, uint16_t connHandle);
    void onNotifyStatus(int code);

    // SyncTask:
    bool send(const uint8_t* tlv, size_t len, datasync::SendStats& stats) override;
    bool ready() const override;
    uint16_t mtu() const override;

private:
    static constexpr UBaseType_t kMaxCredits = SYNC_TX_MAX_CREDITS;  // teto; o efetivo depende do MTU
    static constexpr uint32_t kCreditTimeoutMs = 1000;                // < 2 s do app: o dongle aborta antes
    static constexpr int kNotifyRetries = 20;
    static constexpr uint32_t kNotifyRetryMs = 5;

    rtos::CollectorContext& _ctx;
    NimBLEServer* _server = nullptr;
    NimBLECharacteristic* _tx = nullptr;
    std::atomic<uint16_t> _conn{BLE_HS_CONN_HANDLE_NONE};
    std::atomic<bool> _subscribed{false};
    SemaphoreHandle_t _credits;  // contador, máx. kMaxCredits
    uint8_t _frag[517];          // só a SyncTask usa

    static UBaseType_t creditsFor(uint16_t mtu);
    void resetWindow(uint16_t mtu);
    void post(rtos::SyncInputKind kind, uint16_t connHandle, const uint8_t* data, size_t len);
};
