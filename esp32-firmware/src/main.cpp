#include <Arduino.h>
#include "tasks/Elm327Task.h"
#include "connectivity/ble/BleConnectivity.h"

// ── Build-time connectivity selection 

//
// Set exactly ONE of the following flags in your chosen PlatformIO environment
// (or via platformio_secrets.ini — see README):
//
//   -DUSE_SECURE_PSK          AES-256-GCM with a static pre-shared key
//   -DUSE_SECURE_HANDSHAKE    PSK handshake + HKDF session key (stub)
//   (none)                    Plain BleConnectivity — no encryption
//
#if defined(USE_SECURE_PSK)
    #include "connectivity/secure/psk/SecurePskBleConnectivity.h"
#elif defined(USE_SECURE_HANDSHAKE)
    #include "connectivity/secure/handshake/SecureHandshakeBleConnectivity.h"
#endif

// ── Build-time CAN / OBD2 backend selection 
#if defined(USE_MCP2515)
    #include "can/mcp2515/Mcp2515Can.h"
    #include "obd2/real/Obd2Can.h"
#elif defined(USE_TWAI)
    #include "can/twai/TwaiCan.h"
    #include "obd2/real/Obd2Can.h"
#elif defined(USE_MOCK)
    #include "obd2/mock/Obd2Mock.h"
#else
    #error "No CAN backend defined. Use: pio run -e mcp2515|twai|mock (or a variant like mock_psk)"
#endif

void setup() {
    Serial.begin(115200);
    delay(200);
    Serial.println("\n=== OBD2 Dongle ===");

    // ── CAN / OBD2 backend ───────────────────────────────────────────────────
#if defined(USE_MCP2515)
    static Mcp2515Can  can;
    static Obd2Can     obd2(&can);
    Serial.println("[main] CAN backend : MCP2515 (SPI)");

#elif defined(USE_TWAI)
    static TwaiCan     can;
    static Obd2Can     obd2(&can);
    Serial.println("[main] CAN backend : TWAI (native)");

#elif defined(USE_MOCK)
    static Obd2Mock    obd2;
    Serial.println("[main] CAN backend : MOCK (simulated)");
#endif

    // ── Connectivity stack 
    static BleConnectivity raw("TCCeltaDongle");

#if defined(USE_SECURE_PSK)
    static SecurePskBleConnectivity     secureLayer(&raw);
    IConnectivity* ble = &secureLayer;
    Serial.println("[main] Security     : AES-256-GCM PSK");

#elif defined(USE_SECURE_HANDSHAKE)
    static SecureHandshakeBleConnectivity secureLayer(&raw);
    IConnectivity* ble = &secureLayer;
    Serial.println("[main] Security     : Handshake (stub)");

#else
    IConnectivity* ble = &raw;
    Serial.println("[main] Security     : none (plain transport)");
#endif

    // ── Wire everything together ─────────────────────────────────────────────
    static Elm327Task task(ble, &obd2);

    if (!obd2.begin()) {
        Serial.println("[main] OBD2 init failed — halting");
        while (true) delay(1000);
    }

    if (!ble->begin()) {
        Serial.println("[main] BLE init failed — halting");
        while (true) delay(1000);
    }

    task.start(/* priority */ 5, /* stackSize */ 8192);
    Serial.println("[main] Elm327Task started");
}



void loop() {
    // Processing happens in the Elm327Task FreeRTOS task.
    vTaskDelay(portMAX_DELAY);
}
