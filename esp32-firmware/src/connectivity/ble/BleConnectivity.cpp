#include <Arduino.h>
#include "BleConnectivity.h"
#include "collector/Events.h"


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
    NimBLEDevice::setMTU(517);               // preferido local; o efetivo é o mínimo entre os lados

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

    // SYNC: criado depois do NUS, no mesmo servidor. A Fase 2 acrescenta as permissões
    // cifradas (WRITE_ENC, CCCD cifrado) exatamente nestes dois createCharacteristic.
    if (_syncLink) {
        NimBLEService* pSync = pServer->createService(SYNC_SERVICE_UUID);
        NimBLECharacteristic* syncTx = pSync->createCharacteristic(SYNC_TX_UUID, NIMBLE_PROPERTY::NOTIFY);
        NimBLECharacteristic* syncRx = pSync->createCharacteristic(SYNC_RX_UUID, NIMBLE_PROPERTY::WRITE, 64);
        syncTx->setCallbacks(&_syncTxCallbacks);
        syncRx->setCallbacks(&_syncRxCallbacks);
        _syncLink->attach(pServer, syncTx);
    }

    NimBLEAdvertising* pAdv = NimBLEDevice::getAdvertising();
    pAdv->addServiceUUID(BLE_SERVICE_UUID);
    pAdv->setName(_deviceName);
    pAdv->enableScanResponse(true);
    pAdv->start();

    Serial.println("[BLE] Advertising started for device: " + String(_deviceName));
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
    if (_onConnection) _onConnection(true);  // incrementa a geração antes do SYNC postar Connected
    if (_syncLink) _syncLink->onConnected(connInfo.getConnHandle());
}

void BleConnectivity::onDisconnect(NimBLEServer* pServer, NimBLEConnInfo& connInfo, int reason) {
    Serial.println("[BLE] Client disconnected — restarting advertising");
    if (_onConnection) _onConnection(false);
    if (_syncLink) _syncLink->onDisconnected();
    NimBLEDevice::startAdvertising();
}

void BleConnectivity::onMTUChange(uint16_t mtu, NimBLEConnInfo&) {
    if (_syncLink) _syncLink->onMtuChange(mtu);
    // Evento no callback do NimBLE é aceitável aqui: não aloca e o MTU muda poucas vezes por conexão.
    collector::Event("SYNC_MTU").num("mtu", mtu);
}

// SYNC

void BleConnectivity::SyncRxCallbacks::onWrite(NimBLECharacteristic* pChar, NimBLEConnInfo& connInfo) {
    if (!_owner._syncLink) return;
    const NimBLEAttValue& val = pChar->getValue();
    _owner._syncLink->onRx(val.data(), val.size(), connInfo.getConnHandle());
}

void BleConnectivity::SyncTxCallbacks::onSubscribe(NimBLECharacteristic*, NimBLEConnInfo&, uint16_t subValue) {
    if (_owner._syncLink) _owner._syncLink->onSubscribed(subValue);
}

void BleConnectivity::SyncTxCallbacks::onStatus(NimBLECharacteristic*, NimBLEConnInfo&, int code) {
    if (_owner._syncLink) _owner._syncLink->onNotifyStatus(code);
}
