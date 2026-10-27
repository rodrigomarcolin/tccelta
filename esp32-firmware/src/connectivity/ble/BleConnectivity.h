#pragma once
#include <atomic>
#include "connectivity/IConnectivity.h"
#include "connectivity/ble/BleSyncLink.h"
#include "connectivity/pairing/ILinkControl.h"
#include "connectivity/pairing/PairingController.h"
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
 *   RX characteristic  (WRITE | WRITE_NR | WRITE_ENC): app => comandos dongle
 *   TX characteristic  (NOTIFY, CCCD cifrado):         dongle => resposta ao app
 *
 * Serviço SYNC (TLV, fragmentação por MTU; ver BleSyncLink):
 *   SYNC_RX (WRITE):  app => dongle, um TLV por escrita (até 64 B)
 *   SYNC_TX (NOTIFY): dongle => app, byte de controle + pedaço do TLV
 *
 * Segurança de enlace (spec fase-2/01): o dongle só entrega dados do NUS e do
 * SYNC à conexão *autorizada*, isto é, cifrada com um bond LESC válido. A
 * decisão é da `pairing::PairingPolicy` (via `PairingController`), tomada no
 * evento de cifração; aqui ficam o gate de aplicação (descartar escritas e
 * notificações de/para quem não está autorizado) e as operações sobre o
 * enlace que a política pede (`pairing::ILinkControl`).
 *
 * Esta classe realiza o transporte, repassando o comando recebido às claras
 * (já decifrado pelo enlace) para a classe que irá de fato processá-lo.
 */
class BleConnectivity : public IConnectivity,
                        public pairing::ILinkControl,
                        private NimBLECharacteristicCallbacks,
                        private NimBLEServerCallbacks {
public:
    explicit BleConnectivity(const char* deviceName = "OBD2Dongle");

    /**
     * Inicializa a pilha, o servidor e as características. Não inicia o advertising:
     * quem o liga é `PairingController::begin()` (via `applyAdvertising`).
     */
    bool begin() override;
    void sendResponse(const uint8_t* data, size_t len) override;
    void setOnCommandReceivedCallback(CommandCallback cb) override;

    /** Chamado com true ao autorizar a sessão e false quando a conexão autorizada cai. */
    void setConnectionCallback(std::function<void(bool)> cb) { _onConnection = std::move(cb); }

    /** Transporte do SYNC; deve ser definido antes de `begin()`. */
    void setSyncLink(BleSyncLink* link) { _syncLink = link; }

    /** Política de pareamento; deve ser definida antes de `begin()`. */
    void setPairingController(pairing::PairingController* controller) { _pairing = controller; }

    /** Conexão autorizada (cifrada + bond LESC válido), ou BLE_HS_CONN_HANDLE_NONE. Thread-safe. */
    uint16_t authorizedConn() const { return _authorizedConn.load(); }
    bool     isAuthorized(uint16_t connHandle) const {
        return connHandle != BLE_HS_CONN_HANDLE_NONE && _authorizedConn.load() == connHandle;
    }

    // pairing::ILinkControl
    size_t disconnectAll() override;
    bool   deleteAllBonds(uint32_t& durMs) override;
    void   deleteBond(uint16_t conn) override;
    [[noreturn]] void recoverStore() override;
    void   reject(uint16_t conn, uint8_t hciReason) override;
    void   setAuthorized(uint16_t conn) override;
    void   clearAuthorized(uint16_t conn) override;
    void   setBondingAllowed(bool allowed) override;
    void   applyAdvertising(pairing::AdvMode mode, bool windowOpen, bool hasBond) override;
    int    bondCount() override;

private:
    static constexpr size_t kTrackedLinks = 4;

    const char*           _deviceName;
    CommandCallback       _callback;
    std::function<void(bool)> _onConnection;
    NimBLEServer*         _server = nullptr;
    NimBLECharacteristic* _txChar = nullptr;
    BleSyncLink*          _syncLink = nullptr;
    pairing::PairingController* _pairing = nullptr;
    std::atomic<uint16_t> _authorizedConn{BLE_HS_CONN_HANDLE_NONE};
    std::atomic<uint16_t> _nusSubscribedConn{BLE_HS_CONN_HANDLE_NONE};

    // Instante de conexão por handle, para o `dur` de LINK_DOWN (só a tarefa host do NimBLE escreve).
    struct LinkClock {
        uint16_t handle = BLE_HS_CONN_HANDLE_NONE;
        uint32_t connectedAtMs = 0;
    };
    LinkClock _clocks[kTrackedLinks];

    // Último estado de anúncio aplicado (para só emitir ADV quando muda).
    bool                _advApplied = false;
    pairing::AdvMode    _advMode = pairing::AdvMode::Off;
    bool                _advWindow = false;
    bool                _advBond = false;

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

    // NimBLECharacteristicCallbacks (RX e TX do NUS)
    void onWrite(NimBLECharacteristic* pChar, NimBLEConnInfo& connInfo) override;
    void onSubscribe(NimBLECharacteristic* pChar, NimBLEConnInfo& connInfo, uint16_t subValue) override;

    // NimBLEServerCallbacks
    void onConnect(NimBLEServer* pServer, NimBLEConnInfo& connInfo)    override;
    void onDisconnect(NimBLEServer* pServer, NimBLEConnInfo& connInfo, int reason) override;
    void onAuthenticationComplete(NimBLEConnInfo& connInfo) override;
    void onMTUChange(uint16_t mtu, NimBLEConnInfo& connInfo) override;

    void noteConnected(uint16_t handle);
    uint32_t connectedFor(uint16_t handle) const;
    void forget(uint16_t handle);
};
