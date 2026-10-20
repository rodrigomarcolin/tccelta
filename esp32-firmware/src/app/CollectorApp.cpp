#include "app/CollectorApp.h"
#include <Arduino.h>
#include "collector/BlockWriter.h"
#include "collector/Collector.h"
#include "collector/DataProtocol.h"
#include "collector/IMessageAuth.h"
#include "collector/SequentialPidReader.h"
#include "connectivity/SerialDataChannel.h"
#include "rtos/Adapters.h"
#include "rtos/CollectorContext.h"
#include "rtos/CollectorControl.h"
#include "rtos/QueuedObd2.h"
#include "storage/LittleFsBlockStore.h"
#include "storage/NvsStore.h"
#include "tasks/AcquisitionTask.h"
#include "tasks/Elm327Task.h"
#include "tasks/FlushTask.h"
#include "tasks/SessionTask.h"
#include "tasks/UploadTask.h"

namespace app {

namespace {

constexpr UBaseType_t kElm327Priority = 5;
constexpr uint32_t    kElm327Stack    = 8192;

void onConnection(rtos::CollectorContext& ctx, bool connected) {
    if (connected) {
        xEventGroupSetBits(ctx.events, rtos::APP_CONNECTED);
    } else {
        xEventGroupClearBits(ctx.events, rtos::APP_CONNECTED | rtos::INTERNET | rtos::SESSION_OK);
    }
}

}  // namespace

bool startCollector(BusFactory makeBus, BleConnectivity& ble) {
    // 1. NVS: config e boot_id deste boot.
    static NvsStore nvs;
    if (!nvs.begin()) return false;
    const collector::CollectorConfig config = nvs.loadConfig();
    const uint16_t bootId = nvs.nextBootId();

    // 2. Log de blocos e seq = max(NVS, log) + 1.
    static LittleFsBlockStore store;
    if (!store.begin()) return false;
    const collector::SeqCounter seq = collector::SeqCounter::recover(nvs.loadLastSeq(), store.newestSeq());
    Serial.printf("[collector] boot %u, next seq %lu\n", bootId, static_cast<unsigned long>(seq.peek()));

    // 3. Barramento no protocolo configurado.
    IObd2& bus = makeBus(config);
    if (!bus.begin()) return false;

    // 4. Filas, eventos e pool de blocos.
    static rtos::CollectorContext ctx;
    ctx.create();

    // 5. Objetos.
    static rtos::ArduinoClock clock;
    static rtos::SharedValues values;
    static rtos::SharedTmaxReport tmaxReport;
    static rtos::QueueBlockPool pool(ctx);
    static collector::SequentialPidReader reader(bus, clock);
    static collector::Collector collector(clock, reader, pool, values);
    static collector::BlockWriter writer(store, nvs, seq, bootId);
    static collector::NoAuth auth;  // TODO(segurança): autenticação Dongle ↔ Servidor
    static collector::DataProtocol protocol(auth);
    static SerialDataChannel dataChannel;  // TODO: canal BLE de dados (ver connectivity/README.md)
    static rtos::QueuedObd2 queuedObd2(ctx);
    static rtos::CollectorControl control(ctx, nvs, values, tmaxReport, pool, config);

    // 6. BLE, com as filas já prontas.
    ble.setConnectionCallback([](bool connected) { onConnection(ctx, connected); });
    static Elm327Task elm327Task(&ble, &queuedObd2, &control);
    if (!ble.begin()) return false;

    // 7. Tarefas.
    static FlushTask flushTask(ctx, writer, store);
    static AcquisitionTask acquisitionTask(ctx, collector, bus, clock, tmaxReport);
    static SessionTask sessionTask(ctx, protocol, dataChannel, bootId);
    static UploadTask uploadTask(ctx, store, protocol, dataChannel);
    flushTask.start();
    acquisitionTask.start(config);
    elm327Task.start(kElm327Priority, kElm327Stack);
    sessionTask.start();
    uploadTask.start();
    return true;
}

}  // namespace app
