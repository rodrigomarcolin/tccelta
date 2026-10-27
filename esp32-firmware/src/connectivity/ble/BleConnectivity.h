#pragma once
#include "connectivity/IConnectivity.h"
#include "connectivity/ble/BleSyncLink.h"
#include <NimBLEDevice.h>

// Os UUIDs abaixo correspondem aos convencionados no Nordic UART Service 
// (https://docs.nordicsemi.com/bundle/ncs-3.2.0/page/nrf/libraries/bluetooth/services/nus.html)

static const char* BLE_SERVICE_UUID = "6E400001-B5A3-F393-E0A9-E50E24DCCA9E";
static const char* BLE_RX_UUID      = "6E400002-B5A3-F393-E0A9-E50E24DCCA9E";
static const char* BLE_TX_UUID      = "6E400003-B5A3-F393-E0A9-E50E24DCCA9E";

// Serviço SYNC (spec fase-1/04 §1): mensagens binárias TLV do protocolo de segurança.
// O SYNC não vai no anúncio (não cabe junto do NUS nos 31 B); o app o descobre na conexão.
static const char* SYNC_SERVICE_UUID = "7A3C0001-9B1E-4F2A-8C5D-2E6F1A0B3C4D";
static const char* SYNC_RX_UUID      = "7A3C0002-9B1E-4F2A-8C5D-2E6F1A0B3C4D";
static const char* SYNC_TX_UUID      = "7A3C0003-9B1E-4F2A-8C5D-2E6F1A0B3C4D";

/**
 * Implementa IConnectivity através do Bluetooth Low Energy (NimBLE-Arduino).
 *
 * Topologia:
 *   RX characteristic  (WRITE | WRITE_NR): app => comandos dongle
 *   TX characteristic (NOTIFY):           dongle => resposta ao app
 *
 * Serviço SYNC (TLV, fragmentação por MTU; ver BleSyncLink):
 *   SYNC_RX (WRITE):  app => dongle, um TLV por escrita (até 64 B)
 *   SYNC_TX (NOTIFY): dongle => app, byte de controle + pedaço do TLV
 *
 * Esta classe realiza puramente o transporte, repassando o comando recebido
 * às claras para a classe que irá de fato processá-lo.
 *
 */
class BleConnectivity : public IConnectivity,
                        private NimBLECharacteristicCallbacks,
                        private NimBLEServerCallbacks {
public:
    explicit BleConnectivity(const char* deviceName = "OBD2Dongle");

    bool begin() override;
    void sendResponse(const uint8_t* data, size_t len) override;
    void setOnCommandReceivedCallback(CommandCallback cb) override;

    /** Chamado com true ao conectar e false ao desconectar. */
    void setConnectionCallback(std::function<void(bool)> cb) { _onConnection = std::move(cb); }

    /** Transporte do SYNC; deve ser definido antes de `begin()`. */
    void setSyncLink(BleSyncLink* link) { _syncLink = link; }

private:
    const char*           _deviceName;
    CommandCallback       _callback;
    std::function<void(bool)> _onConnection;
    NimBLECharacteristic* _txChar = nullptr;
    BleSyncLink*          _syncLink = nullptr;

    /** Callbacks pequenos do SYNC: só repassam ao BleSyncLink (nada de trabalho na tarefa do NimBLE). */
    class SyncRxCallbacks : public NimBLECharacteristicCallbacks {
    public:
        explicit SyncRxCallbacks(BleConnectivity& owner) : _owner(owner) {}
        void onWrite(NimBLECharacteristic* pChar, NimBLEConnInfo& connInfo) override;
    private:
        BleConnectivity& _owner;
    };
    class SyncTxCallbacks : public NimBLECharacteristicCallbacks {
    public:
        explicit SyncTxCallbacks(BleConnectivity& owner) : _owner(owner) {}
        void onSubscribe(NimBLECharacteristic* pChar, NimBLEConnInfo& connInfo, uint16_t subValue) override;
        // Só a sobrecarga nova: a antiga está deprecated e a pilha chama as duas (dois créditos por notificação).
        void onStatus(NimBLECharacteristic* pChar, NimBLEConnInfo& connInfo, int code) override;
    private:
        BleConnectivity& _owner;
    };
    SyncRxCallbacks _syncRxCallbacks{*this};
    SyncTxCallbacks _syncTxCallbacks{*this};

    // NimBLECharacteristicCallbacks
    void onWrite(NimBLECharacteristic* pChar, NimBLEConnInfo& connInfo) override;

    // NimBLEServerCallbacks
    void onConnect(NimBLEServer* pServer, NimBLEConnInfo& connInfo)    override;
    void onDisconnect(NimBLEServer* pServer, NimBLEConnInfo& connInfo, int reason) override;
    void onMTUChange(uint16_t mtu, NimBLEConnInfo& connInfo) override;
};
