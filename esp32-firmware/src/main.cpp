#include <Arduino.h>
#include "tasks/Elm327Task.h"
#include "connectivity/ble/BleConnectivity.h"

// Backend no protocolo configurado (AT SP): taxa e tamanho de ID vêm da config.
static IObd2& makeCollectorBus(const collector::CollectorConfig& config) {
#if defined(USE_MOCK)
    (void)config;
    static Obd2Mock obd2;
#else
    const CanBitrate bitrate = collector::uses500Kbps(config.protocol) ? CanBitrate::Kbps500
                                                                        : CanBitrate::Kbps250;
    const ObdAddressing addressing = collector::usesExtendedIds(config.protocol)
                                         ? ObdAddressing::extended29Bit()
                                         : ObdAddressing::standard11Bit();
  #if defined(USE_MCP2515)
    static Mcp2515Can can(bitrate);
  #else
    static TwaiCan can(bitrate);
  #endif
    static Obd2Can obd2(&can, addressing);
#endif
    return obd2;
}

static void halt(const char* reason) {
    Serial.printf("[main] %s — halting\n", reason);
    while (true) delay(1000);
}
#endif

#ifndef SERIAL_BAUD
#define SERIAL_BAUD 115200
#endif

void setup() {
    Serial.begin(SERIAL_BAUD);
    delay(200);
    Serial.println("\n=== OBD2 Dongle ===");

    // Coleta de telemetria: a configuração do CAN vem da NVS.
    static BleConnectivity collectorBle("TCCeltaDongle");
    if (!app::startCollector(makeCollectorBus, collectorBle)) halt("Collector init failed");
    Serial.println("[main] Collector started");
    return;
}



void loop() {
    // Processing happens in the Elm327Task FreeRTOS task.
    vTaskDelay(portMAX_DELAY);
}
