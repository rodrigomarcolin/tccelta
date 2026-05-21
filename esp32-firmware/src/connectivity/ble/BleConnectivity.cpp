#include <Arduino.h>
#include "BleConnectivity.h"


/**
 * Implementação-base da funcionalidade BLE. 
 *
 * Ao inicializar: cria Server, Service, e características RX e TX, e começa a se anunciar.
 * 
 * Expõe as seguintes funções para sua configuração:
 *  setOnCommandReceivedCallback: define o callback a ser executado no recebimento de um valor no RX
 *  sendResponse: atualiza o valor do TX e notifica
 * 
 * Segundo as melhores práticas do BLE e freeRTOS, o callback deve ser lightweight
 * e se restringir a colocar os eventos (neste caso, o comando recebido) em uma fila, 
 * para uma task processar posteriormente.
 */

BleConnectivity::BleConnectivity(const char* deviceName)
    : _deviceName(deviceName) {}

bool BleConnectivity::begin() {
    NimBLEDevice::init(_deviceName);
    NimBLEDevice::setPower(ESP_PWR_LVL_P9);  // max TX power

    NimBLEServer* pServer = NimBLEDevice::createServer();
    pServer->setCallbacks(this);

    NimBLEService* pService = pServer->createService(BLE_SERVICE_UUID);

    // TX: dongle => client (notifiable)
    _txChar = pService->createCharacteristic(
        BLE_TX_UUID,
        NIMBLE_PROPERTY::NOTIFY);

    // RX: client => dongle (writable)
    NimBLECharacteristic* pRxChar = pService->createCharacteristic(
        BLE_RX_UUID,
        NIMBLE_PROPERTY::WRITE | NIMBLE_PROPERTY::WRITE_NR);
    pRxChar->setCallbacks(this);

    NimBLEAdvertising* pAdv = NimBLEDevice::getAdvertising();
    pAdv->addServiceUUID(BLE_SERVICE_UUID);
    pAdv->setName(_deviceName);
    pAdv->enableScanResponse(true);
    pAdv->start();

    Serial.println("[BLE] Advertising started");
    return true;
}

void BleConnectivity::setOnCommandReceivedCallback(CommandCallback cb) {
    _callback = cb;
}


void BleConnectivity::sendResponse(const uint8_t* data, size_t len) {
    if (!_txChar) return;
    _txChar->setValue(data, len);
    _txChar->notify();
}

// NimBLECharacteristicCallbacks

void BleConnectivity::onWrite(NimBLECharacteristic* pChar, NimBLEConnInfo& connInfo) {
    if (!_callback) return;
    std::string val = pChar->getValue();
    _callback(reinterpret_cast<const uint8_t*>(val.data()), val.size());
}

// NimBLEServerCallbacks

void BleConnectivity::onConnect(NimBLEServer* pServer, NimBLEConnInfo& connInfo) {
    Serial.println("[BLE] Client connected");
}

void BleConnectivity::onDisconnect(NimBLEServer* pServer, NimBLEConnInfo& connInfo, int reason) {
    Serial.println("[BLE] Client disconnected — restarting advertising");
    NimBLEDevice::startAdvertising();
}
