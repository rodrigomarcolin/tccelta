#include <Arduino.h>
#include "tasks/Elm327Task.h"
#include "connectivity/ble/BleConnectivity.h"

// Build-time Dependency Injection
// TODO: Adicionar condições e procedimentos para instanciar a implementação segura ou a plain-text

#if defined(USE_MCP2515)
    #include "can/mcp2515/Mcp2515Can.h"
    #include "obd2/real/Obd2Can.h"
#elif defined(USE_TWAI)
    #include "can/twai/TwaiCan.h"
    #include "obd2/real/Obd2Can.h"
#elif defined(USE_MOCK)
    #include "obd2/mock/Obd2Mock.h"
#else
    #error "No build environment defined. Use: pio run -e mcp2515|twai|mock"
#endif

void setup() {
    Serial.begin(115200);
    delay(200);
    Serial.println("\n=== OBD2 Dongle ===");

#if defined(USE_MCP2515)
    static Mcp2515Can        can;           // CSif=5, 500 kbps, 8 MHz
    static Obd2Can           obd2(&can);
    Serial.println("[main] CAN backend : MCP2515 (SPI)");

#elif defined(USE_TWAI)
    static TwaiCan           can;           // TX=21, RX=22
    static Obd2Can           obd2(&can);
    Serial.println("[main] CAN backend : TWAI (native)");

#elif defined(USE_MOCK)
    static Obd2Mock          obd2;
    Serial.println("[main] CAN backend : MOCK (simulated)");
#endif

static BleConnectivity   plainBle("OBD2Dongle");

#if defined(USE_SECURE_CONNECTIVITY)
SecureBleConnectivity ble(&plainBle);
static BleConnectivity* ble = &secureBle;
#else
static BleConnectivity* ble = &plainBle;
#endif
    static Elm327Task        task(ble, &obd2);

    if (!obd2.begin()) {
        Serial.println("[main] OBD2 init failed — halting");
        while (true) delay(1000);
    }

    if (!ble->begin()) {
        Serial.println("[main] BLE init failed — halting");
        while (true) delay(1000);
    }

    // A task abaixo reage à chegada de mensagens no RX do BLE.
    task.start(/* priority */ 5, /* stackSize */ 8192);
    Serial.println("[main] Elm327Task started");
}

// ── loop ──────────────────────────────────────────────────────────────────

void loop() {
    // O processamento é feito na tarefa FreeRTOS Elm327Task.
    vTaskDelay(portMAX_DELAY);
}
