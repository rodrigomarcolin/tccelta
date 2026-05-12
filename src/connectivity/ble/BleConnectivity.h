#pragma once
#include "connectivity/IConnectivity.h"
#include <NimBLEDevice.h>

// Os UUIDs abaixo correspondem aos convencionados no Nordic UART Service 
// (https://docs.nordicsemi.com/bundle/ncs-3.2.0/page/nrf/libraries/bluetooth/services/nus.html)

static const char* BLE_SERVICE_UUID = "6E400001-B5A3-F393-E0A9-E50E24DCCA9E";
static const char* BLE_RX_UUID      = "6E400002-B5A3-F393-E0A9-E50E24DCCA9E";
static const char* BLE_TX_UUID      = "6E400003-B5A3-F393-E0A9-E50E24DCCA9E";

/**
 * Implementa IConnectivity através do Bluetooth Low Energy (NimBLE-Arduino).
 *
 * Topologia:
 *   RX characteristic  (WRITE | WRITE_NR): app => comandos dongle
 *   TX characteristic (NOTIFY):           dongle => resposta ao app
 *
 * Esta classe realiza puramente o transporte, repassando o comando recebido
 * às claras para a classe que irá de fato processá-lo.
 *
 * Deverá ser extendida via padrão decorator para implementação de autenticação/encriptação. 
 *  SecureBleConnectivity encapsulará esta classe. 
 */
class 
BleConnectivity : public IConnectivity,
                        private NimBLECharacteristicCallbacks,
                        private NimBLEServerCallbacks {
public:
    explicit BleConnectivity(const char* deviceName = "OBD2Dongle");

    bool begin() override;
    void sendResponse(const uint8_t* data, size_t len) override;
    void setOnCommandReceivedCallback(CommandCallback cb) override;

private:
    const char*           _deviceName;
    CommandCallback       _callback;
    NimBLECharacteristic* _txChar = nullptr;

    // NimBLECharacteristicCallbacks
    void onWrite(NimBLECharacteristic* pChar, NimBLEConnInfo& connInfo) override;

    // NimBLEServerCallbacks
    void onConnect(NimBLEServer* pServer, NimBLEConnInfo& connInfo)    override;
    void onDisconnect(NimBLEServer* pServer, NimBLEConnInfo& connInfo, int reason) override;
};
