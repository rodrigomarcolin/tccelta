#include <Arduino.h>
#include "app/CollectorApp.h"
#include "connectivity/ble/BleConnectivity.h"

// ── Camada CAN, escolhida pelo ambiente do PlatformIO ───────────────────────
#if defined(USE_MOCK)
    #include "obd2/mock/Obd2Mock.h"
#elif defined(USE_TWAI)
    #include "can/bus/twai/TwaiCan.h"
    #include "obd2/real/Obd2Can.h"
#elif defined(USE_MCP2515)
    #include "can/bus/mcp2515/Mcp2515Can.h"
    #include "obd2/real/Obd2Can.h"
#else
    #error "Nenhuma camada CAN definida. Use: pio run -e collector_mock|collector_twai|collector_mcp2515"
#endif

#ifndef SERIAL_BAUD
#define SERIAL_BAUD 115200
#endif

// Backend no protocolo configurado (AT SP): taxa e tamanho de ID vêm da config.
static IObd2& makeBus(const collector::CollectorConfig& config) {
#if defined(USE_MOCK)
    (void)config;
    static Obd2Mock obd2;
#else
    const CanBitrate bitrate = collector::uses500Kbps(config.protocol) ? CanBitrate::Kbps500
                                                                        : CanBitrate::Kbps250;
    const ObdAddressing addressing = collector::usesExtendedIds(config.protocol)
                                         ? ObdAddressing::extended29Bit()
                                         : ObdAddressing::standard11Bit();
  #if defined(USE_TWAI)
    static TwaiCan can(bitrate);
  #else
    static Mcp2515Can can(bitrate);
  #endif
    static Obd2Can obd2(&can, addressing);
#endif
    return obd2;
}

void setup() {
    Serial.begin(SERIAL_BAUD);
    delay(200);
    Serial.println("\n=== OBD2 Dongle ===");

    static BleConnectivity ble("TCCeltaDongle");
    if (!app::startCollector(makeBus, ble)) {
        Serial.println("[main] Collector init failed — halting");
        while (true) delay(1000);
    }
    Serial.println("[main] Collector started");
}

void loop() {
    // Todo o processamento acontece nas tarefas FreeRTOS.
    vTaskDelay(portMAX_DELAY);
}
