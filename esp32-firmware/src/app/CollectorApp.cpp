#include "app/CollectorApp.h"
#include <Arduino.h>
#include <esp_timer.h>
#include "collector/BlockWriter.h"
#include "collector/Collector.h"
#include "collector/Events.h"
#include "collector/IMessageAuth.h"
#include "collector/MarkerBlock.h"
#include "collector/SeqReserve.h"
#include "collector/SequentialPidReader.h"
#include "connectivity/ble/BleSecurity.h"
#include "connectivity/ble/BleSyncLink.h"
#include "connectivity/ble/LinkSecurityMode.h"
#include "connectivity/pairing/PairingConfig.h"
#include "connectivity/pairing/PairingController.h"
#include "rtos/Adapters.h"
#include "rtos/CollectorContext.h"
#include "rtos/CollectorControl.h"
#include "rtos/QueuedObd2.h"
#include "rtos/SerialEventLog.h"
#include "rtos/SharedSyncStatus.h"
#include "security/BlockAuth.h"
#include "security/HandshakeAuth.h"
#include "security/SecurityProfile.h"
#include "security/mbed/EcdsaBlockAuth.h"
#include "security/mbed/MbedHmacSha256.h"
#include "storage/KeyStore.h"
#include "storage/LittleFsBlockStore.h"
#include "storage/NvsStore.h"
#include "sync/SyncEngine.h"
#include "tasks/AcquisitionTask.h"
#include "tasks/Elm327Task.h"
#include "tasks/FlushTask.h"
#include "tasks/PairingTask.h"
#include "tasks/SyncTask.h"

namespace app {

namespace {

#if defined(COLLECTOR_WIPE_ON_BOOT)
constexpr bool kWipeOnBoot = true;   // teste "de fábrica": apaga blocos, seq e last_cs a cada boot
#else
constexpr bool kWipeOnBoot = false;
#endif

constexpr UBaseType_t kElm327Priority = 5;
constexpr uint32_t    kElm327Stack    = 8192;

void onConnection(rtos::CollectorContext& ctx, bool connected) {
    if (connected) {
        ++ctx.connections;  // geração da conexão: a SyncTask descarta o que sobrou da anterior
        xEventGroupSetBits(ctx.events, rtos::APP_CONNECTED);
    } else {
        xEventGroupClearBits(ctx.events, rtos::APP_CONNECTED | rtos::INTERNET);
    }
    collector::Event("CONN").flag("up", connected).num("gen", ctx.connections);
}

/** Autenticação dos blocos conforme o perfil (spec fase-1/01 §6); cada tarefa tem a sua instância. */
collector::IMessageAuth* makeBlockAuth(const security::DeviceKeys& keys) {
    switch (security::kProfile) {
        case security::Profile::None:
            return new collector::NoAuth();
        case security::Profile::Hmac:
            return new security::HmacBlockAuth(*new security::MbedHmacSha256(keys.kRec, sizeof(keys.kRec)),
                                               keys.devId);
        case security::Profile::Asym:
            return new security::EcdsaBlockAuth(keys.kSig, keys.devId);
    }
    return nullptr;
}

security::IHandshakeAuth* makeHandshakeAuth(const security::DeviceKeys& keys) {
    if (security::kProfile == security::Profile::None) return new security::NoHandshakeAuth();
    return new security::HmacHandshakeAuth(*new security::MbedHmacSha256(keys.kAuth, sizeof(keys.kAuth)),
                                           keys.devId);
}

}  // namespace

bool startCollector(BusFactory makeBus, BleConnectivity& ble) {
    // 0. Eventos de depuração (só com COLLECTOR_EVENTS), antes de tudo.
    static rtos::SerialEventLog eventLog;
    if (collector::kEventsEnabled && !eventLog.begin()) return false;

    // 0.5 Chaves (partição secnvs) e instâncias de autenticação: o BOOT já é assinado.
    static KeyStore keyStore;
    static security::DeviceKeys keys;  // estático: nunca na pilha
    security::KeySource keySource;
    keyStore.printBootLine();
    const int64_t keysStartUs = esp_timer_get_time();
    if (!keyStore.load(keys, keySource)) return false;
    const uint32_t keysLoadUs = static_cast<uint32_t>(esp_timer_get_time() - keysStartUs);
    if (collector::kEventsEnabled) {
        const KeyStore::HardwareState hw = keyStore.hardwareState();
        collector::Event("KEYS")
            .token("profile", security::kProfileName)
            .token("source", security::keySourceName(keySource))
            .hex("dev_id", keys.devId, sizeof(keys.devId))
            .num("tag_len", security::kBlockTagSize)
            .token("fe", hw.flashEncryption)
            .flag("sb", hw.secureBoot)
            .us("load_us", keysLoadUs);
    }
    collector::IMessageAuth* flushAuth = makeBlockAuth(keys);
    collector::IMessageAuth* syncAuth = makeBlockAuth(keys);
    security::IHandshakeAuth* handshakeAuth = makeHandshakeAuth(keys);

    // 1. NVS: config e boot_id deste boot (pula 0).
    static NvsStore nvs;
    if (!nvs.begin()) return false;
    const collector::CollectorConfig config = nvs.loadConfig();
    const uint16_t bootId = nvs.nextBootId();
    const bool wipe = kWipeOnBoot || nvs.wipeRequested();

    // 2. Log de blocos (v2, com migração do /log). O wipe também zera seq_rsv, last_cs e wipe_req.
    static LittleFsBlockStore store;
    if (!store.begin(wipe, security::kBlockTagSize)) return false;
    if (wipe) nvs.clearSeqState();

    // 3. Recuperação de seq (spec 03 §3): o BOOT declara [último na flash + 1, R - 1].
    const uint32_t legacyLast = nvs.loadLegacyLastSeq();
    const collector::SeqRecovery rec =
        collector::SeqReserve::recover(nvs.loadSeqReserve(), legacyLast, store.newestSeq());

    // 4-5. BlockWriter e BOOT antes de qualquer bloco de dados.
    static rtos::ArduinoClock clock;
    static collector::BlockWriter writer(store, nvs, *flushAuth, clock, collector::SeqReserve::afterRecovery(rec),
                                         bootId);
    collector::Event("MARKER").token("kind", "boot").num("seq", rec.bootSeq).num("from", rec.fromSeq)
        .num("to", rec.toSeq).token("reason", "boot");
    collector::WriteReport bootReport;
    const uint32_t bootStartMs = millis();
    const bool bootWritten = writer.writeBootMarker(rec.fromSeq, rec.toSeq, bootReport);
    FlushTask::emitFlush(-1, writer.bootBlock(), bootReport, bootStartMs);

    // 6. Migração concluída: o last_seq legado não é mais lido.
    if (rec.migrated && bootWritten) nvs.removeLegacyLastSeq();

    // 7. Evento BOOT.
    if (collector::kEventsEnabled) {
        collector::Event e("BOOT");
        e.num("boot", bootId).flag("wiped", wipe).num("rsv", rec.reserveRead).num("rsv_new", writer.reserve());
        if (legacyLast > 0) e.num("legacy_last", legacyLast);
        else                e.absent("legacy_last");
        e.num("boot_seq", rec.bootSeq).num("from", rec.fromSeq).num("to", rec.toSeq)
            .num("seq_next", writer.nextSeq()).num("log_oldest", store.oldestSeq())
            .num("log_newest", store.newestSeq()).num("lost", store.lostBlocks())
            .num("log_max", store.maxBlocks());
    }
    Serial.printf("[collector] boot %u, next seq %lu\n", bootId, static_cast<unsigned long>(writer.nextSeq()));

    // 8. Barramento no protocolo configurado.
    IObd2& bus = makeBus(config);
    if (!bus.begin()) return false;

    // 9. Filas, eventos e pool de blocos.
    static rtos::CollectorContext ctx;
    ctx.create();

    // 10. Objetos.
    static rtos::SharedValues values;
    static rtos::SharedTmaxReport tmaxReport;
    static rtos::SharedSyncStatus syncStatus;
    static rtos::QueueBlockPool pool(ctx);
    static collector::SequentialPidReader reader(bus, clock);
    static collector::Collector collector(clock, reader, pool, values);
    static BleSyncLink syncLink(ctx);
    static datasync::SyncEngine engine({store, nvs, *handshakeAuth, *syncAuth, syncLink, clock, keys.devId, bootId,
                                    nullptr /* diagnóstico pelo desafio adiado, spec 05 §11 */});
    static rtos::QueuedObd2 queuedObd2(ctx);
    static rtos::CollectorControl control(ctx, nvs, values, tmaxReport, pool, store, syncStatus, config);

    // 11. BLE (NUS + SYNC) e pareamento, com as filas já prontas.
#if !defined(LINK_SECURITY_OPEN)
    static pairing::PairingController pairing(ble, timingsFromConfig(), CONFIG_BT_NIMBLE_MAX_CONNECTIONS);
    ble.setPairingController(&pairing);
#endif
    ble.setConnectionCallback([](bool connected) { onConnection(ctx, connected); });
    ble.setSyncLink(&syncLink);
    static Elm327Task elm327Task(&ble, &queuedObd2, &control);
    if (!ble.begin()) return false;
#if !defined(LINK_SECURITY_OPEN)
    ble_security::BleSecurity::sanitizeBondsAtBoot();
    pairing.begin();  // bonding desligado e anúncio conforme o estado; só o botão abre a janela
#endif

    // 12. Tarefas.
    static FlushTask flushTask(ctx, writer, store);
    static AcquisitionTask acquisitionTask(ctx, collector, bus, clock, tmaxReport);
    static SyncTask syncTask(ctx, engine, syncStatus);
#if !defined(LINK_SECURITY_OPEN)
    static PairingTask pairingTask(pairing, PairingTaskConfig::fromBuildFlags());
#endif
    eventLog.watch("flush", flushTask.start());
    eventLog.watch("acquisition", acquisitionTask.start(config));
    eventLog.watch("elm327", elm327Task.start(kElm327Priority, kElm327Stack));
    eventLog.watch("sync", syncTask.start());
#if !defined(LINK_SECURITY_OPEN)
    eventLog.watch("pairing", pairingTask.start());
#endif
    return true;
}

}  // namespace app
