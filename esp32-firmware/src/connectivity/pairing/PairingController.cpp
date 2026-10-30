#include "connectivity/pairing/PairingController.h"
#include <Arduino.h>
#include "collector/Events.h"

namespace pairing {

namespace {
constexpr int kDeleteAttempts = 2;
}

PairingController::PairingController(ILinkControl& link, const Timings& timings, uint8_t maxConnections)
    : _link(link), _policy(timings, maxConnections), _mutex(xSemaphoreCreateMutex()) {
    configASSERT(_mutex);
}

void PairingController::begin() {
    Actions actions;
    const bool hasBond = _link.bondCount() > 0;
    xSemaphoreTake(_mutex, portMAX_DELAY);
    _policy.begin(millis(), hasBond, actions);
    xSemaphoreGive(_mutex);
    execute(actions);
}

void PairingController::onLinkUp(uint16_t conn) {
    Actions actions;
    xSemaphoreTake(_mutex, portMAX_DELAY);
    _policy.onLinkUp(conn, millis(), actions);
    xSemaphoreGive(_mutex);
    execute(actions);
}

void PairingController::onEncryptionChange(uint16_t conn, const EncryptionInfo& info) {
    Actions actions;
    xSemaphoreTake(_mutex, portMAX_DELAY);
    _policy.onEncryptionChange(conn, info, millis(), actions);
    xSemaphoreGive(_mutex);
    execute(actions);
}

void PairingController::onLinkDown(uint16_t conn) {
    Actions actions;
    xSemaphoreTake(_mutex, portMAX_DELAY);
    _policy.onLinkDown(conn, millis(), actions);
    xSemaphoreGive(_mutex);
    execute(actions);
}

void PairingController::onButtonHold() {
    Actions actions;
    xSemaphoreTake(_mutex, portMAX_DELAY);
    _policy.onButtonHold(millis(), actions);
    xSemaphoreGive(_mutex);
    execute(actions);
}

void PairingController::tick() {
    Actions actions;
    xSemaphoreTake(_mutex, portMAX_DELAY);
    _policy.tick(millis(), actions);
    xSemaphoreGive(_mutex);
    execute(actions);
}

bool PairingController::windowOpen() const {
    xSemaphoreTake(_mutex, portMAX_DELAY);
    const bool open = _policy.window() == WindowState::Open;
    xSemaphoreGive(_mutex);
    return open;
}

bool PairingController::authorized() const {
    xSemaphoreTake(_mutex, portMAX_DELAY);
    const bool value = _policy.authorizedConn() != kNoConn;
    xSemaphoreGive(_mutex);
    return value;
}

/** Ordem fixa das ações (spec fase-2/02 §4.5). Roda sem o mutex. */
void PairingController::execute(const Actions& a) {
    if (a.advertisingOffFirst) _link.applyAdvertising(AdvMode::Off, a.windowOpen, a.hasBond);

    if (a.disconnectAll) {
        const size_t dropped = _link.disconnectAll();
        collector::Event("CONN_DROP").token("reason", "button").num("count", static_cast<int64_t>(dropped));
    }

    if (a.deleteAllBonds) {
        const int before = _link.bondCount();
        bool ok = false;
        int attempts = 0;
        uint32_t totalMs = 0;
        while (!ok && attempts < kDeleteAttempts) {
            uint32_t durMs = 0;
            ++attempts;
            ok = _link.deleteAllBonds(durMs);
            totalMs += durMs;
        }
        collector::Event("BOND_DELETE").token("reason", "button").num("count", before).flag("ok", ok)
            .num("attempts", attempts).num("dur", totalMs);

        // O resultado define o novo estado (janela aberta ou recuperação): o resto das ações antigas fica obsoleto.
        Actions next;
        xSemaphoreTake(_mutex, portMAX_DELAY);
        _policy.onBondsDeleted(ok, millis(), next);
        xSemaphoreGive(_mutex);
        execute(next);
        return;
    }

    for (uint8_t i = 0; i < a.deleteBondCount; ++i) _link.deleteBond(a.deleteBond[i]);
    for (uint8_t i = 0; i < a.rejectCount; ++i) _link.reject(a.reject[i].conn, a.reject[i].hciReason);

    if (a.authorize != kNoConn) _link.setAuthorized(a.authorize);
    if (a.deauthorize) _link.clearAuthorized(a.deauthorizeConn);
    if (a.setBonding) _link.setBondingAllowed(a.bondingAllowed);
    if (a.setAdvertising) _link.applyAdvertising(a.adv, a.windowOpen, a.hasBond);
    if (a.recoverStore) _link.recoverStore();
}

}  // namespace pairing
