#include <Arduino.h>
#include "BleConnectivity.h"
#include "collector/Events.h"
#include "connectivity/ble/BleSecurity.h"

/**
 * Implementação-base da funcionalidade BLE.
 *
 * Ao inicializar: configura o Security Manager (LESC Just Works), cria Server, Services e
 * características RX/TX do NUS e do SYNC. O advertising é ligado pela política de pareamento
 * (`applyAdvertising`), nunca aqui. Exceção: com `LINK_SECURITY_OPEN` (bancada) não há política;
 * o anúncio é normal desde o `begin` e toda conexão é autorizada ao conectar.
 *
 * Expõe as seguintes funções para sua configuração:
 *  setOnCommandReceivedCallback: define o callback a ser executado no recebimento de um valor no RX
 *  sendResponse: notifica o TX para a conexão autorizada
 *
 * Segundo as melhores práticas do BLE e freeRTOS, o callback deve ser lightweight
 * e se restringir a colocar os eventos (neste caso, o comando recebido) em uma fila,
 * para uma task processar posteriormente.
 */

namespace {

constexpr uint8_t kHciRemoteUserTerminated = BLE_ERR_REM_USER_CONN_TERM;  // 0x13
constexpr int     kHciErrorBase = 0x200;                                    // BLE_HS_ERR_HCI_BASE

const char* addrTypeName(uint8_t type) {
    return (type == BLE_ADDR_PUBLIC || type == BLE_ADDR_PUBLIC_ID) ? "pub" : "rnd";
}

/**
 * Monta anúncio e scan response do zero. O NimBLE anexa um novo campo de dados de fabricante em vez de
 * substituir o anterior; como o pacote de anúncio já está em 27 de 31 B, o campo novo iria para o scan
 * response e o do anúncio ficaria desatualizado. Por isso o dado é sempre reconstruído.
 */
void fillAdvertisingData(NimBLEAdvertising* adv, const char* name, bool windowOpen, bool hasBond) {
    adv->clearData();
    adv->setName(name);
    adv->addServiceUUID(BLE_SERVICE_UUID);
    uint8_t manufacturer[4];
    ble_security::BleSecurity::manufacturerData(windowOpen, hasBond, manufacturer);
    adv->setManufacturerData(manufacturer, sizeof(manufacturer));
}

}  // namespace

BleConnectivity::BleConnectivity(const char* deviceName)
    : _deviceName(deviceName) {}

bool BleConnectivity::begin() {
    NimBLEDevice::init(_deviceName);
    NimBLEDevice::setPower(ESP_PWR_LVL_P9);  // max TX power
    NimBLEDevice::setMTU(517);               // preferido local; o efetivo é o mínimo entre os lados
    ble_security::BleSecurity::configure();  // antes de criar o servidor ou anunciar

    _server = NimBLEDevice::createServer();
    _server->setCallbacks(this);
    _server->advertiseOnDisconnect(false);   // o anúncio é controlado pela política de pareamento

    NimBLEService* pService = _server->createService(BLE_SERVICE_UUID);

    // TX: dongle => client (notifiable; o CCCD exige enlace cifrado)
    _txChar = pService->createCharacteristic(BLE_TX_UUID, ble_security::kSecureTxProps);
    _txChar->setCallbacks(this);

    // RX: client => dongle (writable, só em enlace cifrado)
    NimBLECharacteristic* pRxChar = pService->createCharacteristic(BLE_RX_UUID, ble_security::kSecureRxProps);
    pRxChar->setCallbacks(this);

    // SYNC: criado depois do NUS, no mesmo servidor, com as mesmas exigências de enlace.
    if (_syncLink) {
        NimBLEService* pSync = _server->createService(SYNC_SERVICE_UUID);
        NimBLECharacteristic* syncTx = pSync->createCharacteristic(SYNC_TX_UUID, ble_security::kSecureTxProps);
        NimBLECharacteristic* syncRx = pSync->createCharacteristic(SYNC_RX_UUID, ble_security::kSecureRxProps, 64);
        syncTx->setCallbacks(&_syncTxCallbacks);
        syncRx->setCallbacks(&_syncRxCallbacks);
        _syncLink->attach(_server, syncTx);
    }

    // Ordem importa: com o scan response ligado, o nome vai para ele e o pacote de anúncio cabe
    // flags + UUID do NUS + dados de fabricante (27 de 31 B).
    NimBLEAdvertising* pAdv = NimBLEDevice::getAdvertising();
    pAdv->enableScanResponse(true);
    fillAdvertisingData(pAdv, _deviceName, false, false);

    if (ble_security::kLinkOpen) {
        Serial.printf("[BLE] NimBLE %s, ENLACE ABERTO (sem LESC, so bancada)\n", NimBLEDevice::getVersion());
        collector::Event("LINK_OPEN").token("warn", "no_lesc");
        applyAdvertising(pairing::AdvMode::Normal, false, false);
        return true;
    }

    Serial.printf("[BLE] NimBLE %s, LESC Just Works, bonds=%d, MAX_BONDS=%d\n", NimBLEDevice::getVersion(),
                  NimBLEDevice::getNumBonds(), MYNEWT_VAL(BLE_STORE_MAX_BONDS));
    return true;
}

void BleConnectivity::setOnCommandReceivedCallback(CommandCallback cb) {
    _callback = cb;
}

void BleConnectivity::sendResponse(const uint8_t* data, size_t len) {
    // Notificação sempre com destino explícito: a conexão autorizada, e só se ela se inscreveu.
    const uint16_t conn = _authorizedConn.load();
    if (!_txChar || conn == BLE_HS_CONN_HANDLE_NONE || _nusSubscribedConn.load() != conn) return;
    _txChar->notify(data, len, conn);
}

// NimBLECharacteristicCallbacks (NUS)

void BleConnectivity::onWrite(NimBLECharacteristic* pChar, NimBLEConnInfo& connInfo) {
    const NimBLEAttValue& val = pChar->getValue();
    if (!isAuthorized(connInfo.getConnHandle())) {
        collector::Event("WRITE_DROP").num("conn", connInfo.getConnHandle()).token("chr", "nus_rx")
            .num("len", static_cast<int64_t>(val.size()));
        return;
    }
    if (!_callback) return;
    _callback(val.data(), val.size());
}

void BleConnectivity::onSubscribe(NimBLECharacteristic* pChar, NimBLEConnInfo& connInfo, uint16_t subValue) {
    if (pChar != _txChar) return;
    if ((subValue & 1) != 0) {
        _nusSubscribedConn.store(connInfo.getConnHandle());
    } else {
        uint16_t expected = connInfo.getConnHandle();
        _nusSubscribedConn.compare_exchange_strong(expected, BLE_HS_CONN_HANDLE_NONE);
    }
}

// NimBLEServerCallbacks

void BleConnectivity::noteConnected(uint16_t handle) {
    for (LinkClock& c : _clocks) {
        if (c.handle == BLE_HS_CONN_HANDLE_NONE || c.handle == handle) {
            c.handle = handle;
            c.connectedAtMs = millis();
            return;
        }
    }
}

uint32_t BleConnectivity::connectedFor(uint16_t handle) const {
    for (const LinkClock& c : _clocks) {
        if (c.handle == handle) return millis() - c.connectedAtMs;
    }
    return 0;
}

void BleConnectivity::forget(uint16_t handle) {
    for (LinkClock& c : _clocks) {
        if (c.handle == handle) c.handle = BLE_HS_CONN_HANDLE_NONE;
    }
}

void BleConnectivity::onConnect(NimBLEServer* pServer, NimBLEConnInfo& connInfo) {
    const uint16_t handle = connInfo.getConnHandle();
    noteConnected(handle);
    if (collector::kEventsEnabled) {
        const NimBLEAddress air = connInfo.getAddress();
        uint8_t shown[6];  // ordem de exibição (a NimBLE guarda o endereço invertido)
        for (int i = 0; i < 6; ++i) shown[i] = air.getVal()[5 - i];
        collector::Event("LINK_UP").num("conn", handle).hex("addr", shown, sizeof(shown))
            .token("atype", addrTypeName(air.getType())).flag("window", _pairing && _pairing->windowOpen());
    }
    if (ble_security::kLinkOpen) {
        Serial.println("[BLE] Link up (enlace aberto, autorizado)");
        setAuthorized(handle);
        return;
    }
    Serial.println("[BLE] Link up (aguardando cifra)");
    if (!_pairing) return;
    _pairing->onLinkUp(handle);
    // Só quem já tem bond recebe o Security Request: o celular pareado cifra na hora, e um estranho não
    // ocupa o procedimento de segurança nem ganha um diálogo de pareamento fora de hora.
    if (NimBLEDevice::isBonded(connInfo.getIdAddress())) NimBLEDevice::startSecurity(handle);
}

void BleConnectivity::onAuthenticationComplete(NimBLEConnInfo& connInfo) {
    if (!_pairing) return;
    const uint16_t handle = connInfo.getConnHandle();
    pairing::EncryptionInfo info;
    info.status = ble_security::BleSecurity::lastEncStatus(handle);
    info.encrypted = connInfo.isEncrypted();
    info.bonded = connInfo.isBonded();
    info.keySize = connInfo.getSecKeySize();
    if (info.bonded) {
        const ble_security::BondCheck check = ble_security::BleSecurity::checkBond(connInfo.getIdAddress());
        info.lesc = check.found && check.lesc;
        info.persisted = check.found && check.persisted;
    }
    _pairing->onEncryptionChange(handle, info);
}

void BleConnectivity::onDisconnect(NimBLEServer* pServer, NimBLEConnInfo& connInfo, int reason) {
    const uint16_t handle = connInfo.getConnHandle();
    const bool wasAuthorized = isAuthorized(handle);
    if (collector::kEventsEnabled) {
        collector::Event e("LINK_DOWN");
        e.num("conn", handle).num("rc", reason);
        if (reason >= kHciErrorBase && reason < kHciErrorBase + 0x100) e.hex8("hci", static_cast<uint8_t>(reason - kHciErrorBase));
        else e.absent("hci");
        e.flag("auth", wasAuthorized).num("dur", connectedFor(handle));
    }
    Serial.println("[BLE] Link down");
    ble_security::BleSecurity::clearEncStatus(handle);
    forget(handle);
    if (ble_security::kLinkOpen) {
        // Sem política: encerra a sessão e religa o anúncio (advertiseOnDisconnect está desligado).
        clearAuthorized(handle);
        applyAdvertising(pairing::AdvMode::Normal, false, false);
        return;
    }
    // A política encerra a sessão (clearAuthorized) e religa o anúncio conforme o estado.
    if (_pairing) _pairing->onLinkDown(handle);
}

void BleConnectivity::onMTUChange(uint16_t mtu, NimBLEConnInfo&) {
    if (_syncLink) _syncLink->onMtuChange(mtu);
    // Evento no callback do NimBLE é aceitável aqui: não aloca e o MTU muda poucas vezes por conexão.
    collector::Event("SYNC_MTU").num("mtu", mtu);
}

// pairing::ILinkControl

size_t BleConnectivity::disconnectAll() {
    if (!_server) return 0;
    const std::vector<uint16_t> peers = _server->getPeerDevices();
    for (uint16_t handle : peers) _server->disconnect(handle, kHciRemoteUserTerminated);
    return peers.size();
}

bool BleConnectivity::deleteAllBonds(uint32_t& durMs) {
    return ble_security::BleSecurity::deleteAllBonds(durMs);
}

void BleConnectivity::deleteBond(uint16_t conn) {
    if (!_server) return;
    const NimBLEConnInfo info = _server->getPeerInfoByHandle(conn);
    if (info.getConnHandle() == conn) NimBLEDevice::deleteBond(info.getIdAddress());
}

void BleConnectivity::recoverStore() {
    ble_security::BleSecurity::eraseBondStoreAndRestart();
}

void BleConnectivity::reject(uint16_t conn, uint8_t hciReason) {
    if (_server) _server->disconnect(conn, hciReason);
}

void BleConnectivity::setAuthorized(uint16_t conn) {
    _authorizedConn.store(conn);
    if (_onConnection) _onConnection(true);  // incrementa a geração antes do SYNC postar Connected
    if (_syncLink) _syncLink->onConnected(conn);
}

void BleConnectivity::clearAuthorized(uint16_t conn) {
    uint16_t expected = conn;
    if (!_authorizedConn.compare_exchange_strong(expected, BLE_HS_CONN_HANDLE_NONE)) return;
    _nusSubscribedConn.store(BLE_HS_CONN_HANDLE_NONE);
    if (_onConnection) _onConnection(false);
    if (_syncLink) _syncLink->onDisconnected();
}

void BleConnectivity::setBondingAllowed(bool allowed) {
    ble_security::BleSecurity::setBondingAllowed(allowed);
}

void BleConnectivity::applyAdvertising(pairing::AdvMode mode, bool windowOpen, bool hasBond) {
    NimBLEAdvertising* adv = NimBLEDevice::getAdvertising();
    // A biblioteca pode religar o anúncio sozinha (CONNECT falho); dados e intervalos ficam sempre coerentes.
    adv->stop();
    fillAdvertisingData(adv, _deviceName, windowOpen, hasBond);
    const uint16_t interval = mode == pairing::AdvMode::Pairing ? ble_security::kAdvItvlPairing
                                                                : ble_security::kAdvItvlNormal;
    adv->setMinInterval(interval);
    adv->setMaxInterval(interval);
    if (mode != pairing::AdvMode::Off) adv->start();

    const bool changed = !_advApplied || mode != _advMode || windowOpen != _advWindow || hasBond != _advBond;
    _advApplied = true;
    _advMode = mode;
    _advWindow = windowOpen;
    _advBond = hasBond;
    if (changed) {
        const char* name = mode == pairing::AdvMode::Off ? "off" : (mode == pairing::AdvMode::Pairing ? "pairing" : "normal");
        collector::Event("ADV").token("mode", name).flag("window", windowOpen).flag("bonded", hasBond);
    }
}

int BleConnectivity::bondCount() {
    return ble_security::BleSecurity::bondCount();
}

// SYNC

void BleConnectivity::SyncRxCallbacks::onWrite(NimBLECharacteristic* pChar, NimBLEConnInfo& connInfo) {
    if (!_owner._syncLink) return;
    const NimBLEAttValue& val = pChar->getValue();
    if (!_owner.isAuthorized(connInfo.getConnHandle())) {
        collector::Event("WRITE_DROP").num("conn", connInfo.getConnHandle()).token("chr", "sync_rx")
            .num("len", static_cast<int64_t>(val.size()));
        return;
    }
    _owner._syncLink->onRx(val.data(), val.size(), connInfo.getConnHandle());
}

void BleConnectivity::SyncTxCallbacks::onSubscribe(NimBLECharacteristic*, NimBLEConnInfo& connInfo, uint16_t subValue) {
    if (!_owner._syncLink || !_owner.isAuthorized(connInfo.getConnHandle())) return;
    _owner._syncLink->onSubscribed(subValue);
}

void BleConnectivity::SyncTxCallbacks::onStatus(NimBLECharacteristic*, NimBLEConnInfo&, int code) {
    if (_owner._syncLink) _owner._syncLink->onNotifyStatus(code);
}
