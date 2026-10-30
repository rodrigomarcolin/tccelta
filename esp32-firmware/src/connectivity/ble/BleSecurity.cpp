#include "connectivity/ble/BleSecurity.h"
#include <Arduino.h>
#include <atomic>
#include <esp_system.h>
#include <freertos/FreeRTOS.h>
#include <nvs.h>
#include <nvs_flash.h>
#include "collector/Events.h"

// Declarada à mão: o header é privado da NimBLE. Se a função sumir numa versão futura, o gate de
// aplicação (BleConnectivity::isAuthorized) continua garantindo que nada é notificado a não autorizado.
extern "C" void ble_gatts_set_clt_cfg_perm_flags(uint8_t flags);

static_assert(MYNEWT_VAL(BLE_STORE_MAX_BONDS) == 1,
              "Um bond por vez: compile com -DCONFIG_BT_NIMBLE_MAX_BONDS=1 (seção [pairing] do platformio.ini)");

namespace ble_security {

namespace {

constexpr uint8_t kCccdPerms = BLE_ATT_F_READ | BLE_ATT_F_WRITE | BLE_ATT_F_READ_ENC | BLE_ATT_F_WRITE_ENC;
constexpr size_t  kEncSlots = 4;

portMUX_TYPE gSecurityMux = portMUX_INITIALIZER_UNLOCKED;
BondStoreGuard gBondGuard;

struct EncSlot {
    std::atomic<uint16_t> handle{BLE_HS_CONN_HANDLE_NONE};
    std::atomic<int>      status{0};
};
EncSlot gEncSlots[kEncSlots];

}  // namespace

void BleSecurity::configure() {
    // Enlace aberto (bancada): sem LESC, sem bond e com o CCCD padrão da pilha (livre).
    if (kLinkOpen) return;

    // Bond desligado no boot: só a janela de pareamento o liga. MITM não (Just Works); SC sim (LESC).
    NimBLEDevice::setSecurityAuth(false, false, true);
    NimBLEDevice::setSecurityIOCap(BLE_HS_IO_NO_INPUT_OUTPUT);
    NimBLEDevice::setSecurityInitKey(BLE_SM_PAIR_KEY_DIST_ENC | BLE_SM_PAIR_KEY_DIST_ID);
    NimBLEDevice::setSecurityRespKey(BLE_SM_PAIR_KEY_DIST_ENC | BLE_SM_PAIR_KEY_DIST_ID);
    // ble_hs_cfg.sm_sc_only fica 0 de propósito: ligado, o ATT exigiria enlace autenticado (MITM).
    // O "LESC-only" é verificado no bond (checkBond: sc == 1, chave de 16 B).

    // Inscrever-se em notificações (escrever o CCCD) exige enlace cifrado. Lido só no ble_gatts_start().
    ble_gatts_set_clt_cfg_perm_flags(kCccdPerms);

    NimBLEDevice::setDeviceCallbacks(&gBondGuard);
    NimBLEDevice::setCustomGapHandler(&BleSecurity::gapListener, nullptr);
}

void BleSecurity::setBondingAllowed(bool allowed) {
    // Os campos sm_* são bitfields da mesma palavra: toda escrita passa pela mesma seção crítica.
    taskENTER_CRITICAL(&gSecurityMux);
    ble_hs_cfg.sm_bonding = allowed ? 1 : 0;
    taskEXIT_CRITICAL(&gSecurityMux);
}

bool BleSecurity::bondingAllowed() {
    taskENTER_CRITICAL(&gSecurityMux);
    const bool allowed = ble_hs_cfg.sm_bonding != 0;
    taskEXIT_CRITICAL(&gSecurityMux);
    return allowed;
}

int BleSecurity::bondCount() { return NimBLEDevice::getNumBonds(); }

BondCheck BleSecurity::checkBond(const NimBLEAddress& idAddress) {
    BondCheck out{false, false, false};
    ble_store_key_sec key = {};
    key.peer_addr = *idAddress.getBase();
    key.idx = 0;
    ble_store_value_sec value = {};
    if (ble_store_read_peer_sec(&key, &value) != 0) return out;
    out.found = true;
    out.lesc = value.sc == 1 && value.key_size == 16;
    out.persisted = value.ltk_present == 1;
    return out;
}

bool BleSecurity::deleteAllBonds(uint32_t& durMs) {
    const uint32_t start = millis();
    const bool ok = NimBLEDevice::deleteAllBonds();
    durMs = millis() - start;
    return ok;
}

void BleSecurity::eraseBondStoreAndRestart() {
    collector::Event("BOND_DELETE").token("reason", "recover").num("count", NimBLEDevice::getNumBonds())
        .flag("ok", true).num("attempts", 1).num("dur", 0);
    nvs_handle_t handle;
    if (nvs_open("nimble_bond", NVS_READWRITE, &handle) == ESP_OK) {
        nvs_erase_all(handle);
        nvs_commit(handle);
        nvs_close(handle);
    }
    delay(100);  // deixa o logger esvaziar a fila
    esp_restart();
    for (;;) {}
}

void BleSecurity::sanitizeBondsAtBoot() {
    nvs_stats_t stats = {};
    if (nvs_get_stats(nullptr, &stats) == ESP_OK) {
        collector::Event("NVS").num("free", stats.free_entries).num("used", stats.used_entries)
            .num("total", stats.total_entries);
    }

    const int count = NimBLEDevice::getNumBonds();
    bool dirty = count > 1;
    for (int i = 0; i < count && !dirty; ++i) {
        const BondCheck check = checkBond(NimBLEDevice::getBondedAddress(i));
        if (!check.found || !check.lesc) dirty = true;
    }
    if (!dirty) return;

    uint32_t durMs = 0;
    const bool ok = deleteAllBonds(durMs);
    collector::Event("BOND_DELETE").token("reason", "policy").num("count", count).flag("ok", ok).num("attempts", 1)
        .num("dur", durMs);
}

int BleSecurity::lastEncStatus(uint16_t connHandle) {
    for (EncSlot& slot : gEncSlots) {
        if (slot.handle.load() == connHandle) return slot.status.load();
    }
    return 0;
}

void BleSecurity::clearEncStatus(uint16_t connHandle) {
    for (EncSlot& slot : gEncSlots) {
        uint16_t expected = connHandle;
        slot.handle.compare_exchange_strong(expected, BLE_HS_CONN_HANDLE_NONE);
    }
}

void BleSecurity::manufacturerData(bool windowOpen, bool hasBond, uint8_t out[4]) {
    out[0] = static_cast<uint8_t>(kCompanyId & 0xFF);
    out[1] = static_cast<uint8_t>(kCompanyId >> 8);
    out[2] = kAdvFormatVersion;
    out[3] = static_cast<uint8_t>((windowOpen ? 0x01 : 0x00) | (hasBond ? 0x02 : 0x00));
}

/** Único listener de GAP do dispositivo: guarda o status do ENC_CHANGE, que o callback do servidor não recebe. */
int BleSecurity::gapListener(ble_gap_event* event, void*) {
    if (event->type != BLE_GAP_EVENT_ENC_CHANGE) return 0;
    const uint16_t handle = event->enc_change.conn_handle;
    EncSlot* target = nullptr;
    for (EncSlot& slot : gEncSlots) {
        if (slot.handle.load() == handle) {
            target = &slot;
            break;
        }
    }
    if (!target) {
        for (EncSlot& slot : gEncSlots) {
            uint16_t expected = BLE_HS_CONN_HANDLE_NONE;
            if (slot.handle.compare_exchange_strong(expected, handle)) {
                target = &slot;
                break;
            }
        }
    }
    if (target) target->status.store(event->enc_change.status);
    return 0;
}

int BondStoreGuard::onStoreStatus(ble_store_status_event* event, void* arg) {
    const bool windowOpen = BleSecurity::bondingAllowed();
    if (event->event_code == BLE_STORE_EVENT_FULL) {
        // Todo Pairing Request com o store cheio passa por aqui, antes de qualquer procedimento ou REPEAT_PAIRING.
        const bool reject = !windowOpen;
        collector::Event("BOND_FULL").token("event", "full").token("action", reject ? "reject" : "allow")
            .num("obj", BLE_STORE_OBJ_TYPE_PEER_SEC).num("conn", event->full.conn_handle).flag("window", windowOpen);
        return reject ? BLE_HS_ESTORE_CAP : 0;
    }
    if (event->event_code == BLE_STORE_EVENT_OVERFLOW) {
        const int obj = event->overflow.obj_type;
        if (obj == BLE_STORE_OBJ_TYPE_OUR_SEC || obj == BLE_STORE_OBJ_TYPE_PEER_SEC) {
            // Não grava o segundo bond e não apaga o existente.
            collector::Event("BOND_FULL").token("event", "overflow").token("action", "reject").num("obj", obj)
                .absent("conn").flag("window", windowOpen);
            return BLE_HS_ESTORE_CAP;
        }
    }
    return ble_store_util_status_rr(event, arg);
}

}  // namespace ble_security
