#include "tasks/SyncTask.h"
#include <esp_timer.h>
#include "collector/Events.h"
#include "rtos/Adapters.h"
#include "security/SecurityProfile.h"

using rtos::SyncInputKind;

namespace {

// ECDSA do LOSS (perfil asym) pede mais pilha (spec fase-1/05 §10).
constexpr uint32_t    kStackBytes = security::kProfile == security::Profile::Asym ? 12288 : 8192;
constexpr UBaseType_t kPriority   = 3;
constexpr BaseType_t  kCore       = 0;
constexpr uint32_t    kTickMs     = 1000;

uint32_t nowUs32() { return static_cast<uint32_t>(esp_timer_get_time()); }

}  // namespace

TaskHandle_t SyncTask::start() { return rtos::startTask(this, "Sync", kStackBytes, kPriority, kCore); }

void SyncTask::run() {
    _engine.begin();
    uint32_t lastTick = millis();
    rtos::SyncInput in;
    while (true) {
        const TickType_t wait =
            _engine.batchActive() ? 0 : pdMS_TO_TICKS(collector::remaining(millis(), lastTick + kTickMs));
        if (xQueueReceive(_ctx.syncRx, &in, wait) == pdTRUE) {
            do {
                dispatch(in);
            } while (xQueueReceive(_ctx.syncRx, &in, 0) == pdTRUE);  // drena antes do próximo bloco
        }
        if (_ctx.noticePending.exchange(false)) _engine.onFlushed(0);
        if (_engine.batchActive()) _engine.sendNextBlock();
        if (collector::reached(millis(), lastTick + kTickMs)) {
            reportQueueDrops();
            _engine.tick();
            lastTick = millis();
        }
        _status.set(_engine.status());
    }
}

void SyncTask::dispatch(const rtos::SyncInput& in) {
    // Conexão que o SyncEngine ainda não viu (Connected perdido na fila): abre primeiro.
    if ((in.kind == SyncInputKind::Rx || in.kind == SyncInputKind::Subscribed) &&
        in.value != _engine.generation()) {
        _engine.onConnected(in.value);
    }
    switch (in.kind) {
        case SyncInputKind::Connected:    _engine.onConnected(in.value); break;
        case SyncInputKind::Disconnected: _engine.onDisconnected(); break;
        case SyncInputKind::Subscribed:   _engine.onSubscribed(true); break;
        case SyncInputKind::Unsubscribed: _engine.onSubscribed(false); break;
        case SyncInputKind::Rx:           _engine.onMessage(in.data, in.len, nowUs32() - in.tUs); break;
        case SyncInputKind::Flushed:      _engine.onFlushed(in.value); break;
        case SyncInputKind::InjectAck:    _engine.injectAck(in.value); break;
    }
}

// O callback do NimBLE não emite eventos: a SyncTask conta o que ele perdeu.
void SyncTask::reportQueueDrops() {
    const uint32_t total = _ctx.counters.syncRxDropped;
    if (total == _reportedDrops) return;
    collector::Event("SYNC_RX").absent("type").num("len", total - _reportedDrops).us("q_us", 0)
        .flag("ok", false).token("reason", "queue_full");
    _reportedDrops = total;
}
