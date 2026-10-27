#include "connectivity/ble/BleSyncLink.h"
#include <esp_timer.h>
#include <algorithm>
#include <cstring>
#include "sync/Fragmenter.h"

namespace {

#ifdef CONFIG_BT_NIMBLE_MSYS1_BLOCK_COUNT
constexpr unsigned kPoolBlocks = CONFIG_BT_NIMBLE_MSYS1_BLOCK_COUNT;
#else
constexpr unsigned kPoolBlocks = 12;  // padrão do NimBLE-Arduino
#endif

constexpr uint16_t kMinMtu = 23;

}  // namespace

BleSyncLink::BleSyncLink(rtos::CollectorContext& ctx) : _ctx(ctx) {
    _credits = xSemaphoreCreateCounting(kMaxCredits, 0);
    configASSERT(_credits);
}

void BleSyncLink::attach(NimBLEServer* server, NimBLECharacteristic* tx) {
    _server = server;
    _tx = tx;
}

/** Janela derivada do MTU: cada fragmento ocupa ceil((MTU - 3 + 20) / 236) blocos MSYS_1 de 256 B. */
UBaseType_t BleSyncLink::creditsFor(uint16_t mtu) {
    const unsigned perFragment = (static_cast<unsigned>(mtu) - 3 + 20 + 235) / 236;
    const unsigned usable = kPoolBlocks > 4 ? kPoolBlocks - 4 : 1;
    const unsigned maxCredits = kMaxCredits;  // cópia: std::min/max tomam referência (ODR)
    return static_cast<UBaseType_t>(std::min<unsigned>(std::max<unsigned>(usable / perFragment, 1), maxCredits));
}

void BleSyncLink::resetWindow(uint16_t mtu) {
    while (xSemaphoreTake(_credits, 0) == pdTRUE) {}
    const UBaseType_t credits = creditsFor(std::max(mtu, kMinMtu));
    for (UBaseType_t i = 0; i < credits; ++i) xSemaphoreGive(_credits);
}

void BleSyncLink::post(rtos::SyncInputKind kind, uint16_t connHandle, const uint8_t* data, size_t len) {
    rtos::SyncInput input = {};
    input.kind = kind;
    input.connHandle = connHandle;
    input.value = _ctx.connections;
    input.tUs = static_cast<uint32_t>(esp_timer_get_time());
    if (data) {
        input.len = static_cast<uint8_t>(len);
        memcpy(input.data, data, len);
    }
    if (xQueueSend(_ctx.syncRx, &input, 0) != pdTRUE) ++_ctx.counters.syncRxDropped;
}

// ── Tarefa do NimBLE: só copiar e enfileirar ─────────────────────────────

void BleSyncLink::onConnected(uint16_t connHandle) {
    _conn = connHandle;
    _subscribed = false;
    resetWindow(kMinMtu);
    post(rtos::SyncInputKind::Connected, connHandle, nullptr, 0);
}

void BleSyncLink::onDisconnected() {
    _conn = BLE_HS_CONN_HANDLE_NONE;
    _subscribed = false;
    resetWindow(kMinMtu);
    post(rtos::SyncInputKind::Disconnected, BLE_HS_CONN_HANDLE_NONE, nullptr, 0);
}

void BleSyncLink::onSubscribed(uint16_t subValue) {
    const bool on = (subValue & 1) != 0;  // bit 0 = notificações
    _subscribed = on;
    post(on ? rtos::SyncInputKind::Subscribed : rtos::SyncInputKind::Unsubscribed, _conn, nullptr, 0);
}

void BleSyncLink::onMtuChange(uint16_t mtu) { resetWindow(mtu); }

void BleSyncLink::onRx(const uint8_t* data, size_t len, uint16_t connHandle) {
    if (len > datasync::kSyncRxMax) return;
    post(rtos::SyncInputKind::Rx, connHandle, data, len);
}

void BleSyncLink::onNotifyStatus(int) { xSemaphoreGive(_credits); }  // contador com teto: o excedente é ignorado

// ── SyncTask ─────────────────────────────────────────────────────────────

bool BleSyncLink::ready() const { return _conn != BLE_HS_CONN_HANDLE_NONE && _subscribed; }

uint16_t BleSyncLink::mtu() const {
    if (!_server || _conn == BLE_HS_CONN_HANDLE_NONE) return 0;
    return _server->getPeerMTU(_conn);
}

bool BleSyncLink::send(const uint8_t* tlv, size_t len, datasync::SendStats& stats) {
    stats = {};
    if (!ready() || !_tx) return false;
    const uint16_t conn = _conn;
    const uint16_t peerMtu = std::max(mtu(), kMinMtu);
    datasync::Fragmenter fragmenter(tlv, len, peerMtu);
    stats.fragments = fragmenter.count();
    stats.mtu = peerMtu;
    const int64_t t0 = esp_timer_get_time();

    size_t n = 0;
    bool ok = true;
    while (ok && fragmenter.next(_frag, n)) {
        bool sent = false;
        for (int attempt = 0; attempt <= kNotifyRetries && !sent; ++attempt) {
            // Sem crédito em 1 s: aborta (o app detecta a mensagem incompleta e manda 0x04).
            if (xSemaphoreTake(_credits, pdMS_TO_TICKS(kCreditTimeoutMs)) != pdTRUE || !ready()) {
                ok = false;
                break;
            }
            if (_tx->notify(_frag, n, conn)) {
                sent = true;  // o crédito volta em onNotifyStatus
            } else {
                xSemaphoreGive(_credits);  // sem mbuf: devolve, espera e tenta de novo
                ++stats.retries;
                vTaskDelay(pdMS_TO_TICKS(kNotifyRetryMs));
            }
        }
        ok = ok && sent;
    }
    stats.durUs = static_cast<uint32_t>(esp_timer_get_time() - t0);
    stats.ok = ok;
    return ok;
}
