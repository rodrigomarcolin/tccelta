#include <Arduino.h>
#include "Obd2Mock.h"


// TODO: Revisar/Melhorar. Este arquivo foi gerado com IA.

bool Obd2Mock::begin() { return true; }

int Obd2Mock::readPid(uint8_t service, uint8_t pid,
                      uint8_t* buf, size_t maxLen) {
    if (service != 0x01) return -1;

    // Gentle oscillation: ±10 % variation over time so live-data screens
    // show something moving without overwhelming the log.
    uint32_t t = millis() / 1000u;  // seconds
    int8_t   wave = (int8_t)(10 * sin((float)t * 0.5f));  // -10 … +10

    switch (pid) {
        // ── Supported PIDs (0x01-0x20 bitfield) ──────────────────────
        // PIDs enabled: 0x04, 0x05, 0x0C, 0x0D, 0x0E, 0x11
        //   byte A: 0x18  (bits for 0x04 and 0x05)
        //   byte B: 0x1E  (bits for 0x0C, 0x0D, 0x0E, 0x0F)
        //   byte C: 0x80  (bit for 0x11)
        //   byte D: 0x00
        case 0x00:
            if (maxLen < 4) return -1;
            buf[0] = 0x18; buf[1] = 0x1E; buf[2] = 0x80; buf[3] = 0x00;
            return 4;

        // Engine load — formula: A / 2.55  → 40 %
        case 0x04:
            if (maxLen < 1) return -1;
            buf[0] = (uint8_t)(102 + wave);
            return 1;

        // Coolant temp — formula: A - 40  → 90 °C
        case 0x05:
            if (maxLen < 1) return -1;
            buf[0] = (uint8_t)(130 + wave);
            return 1;

        // Engine RPM — formula: (A*256 + B) / 4  → 1500 RPM
        case 0x0C:
            if (maxLen < 2) return -1;
            {
                uint16_t raw = (uint16_t)((1500 + wave * 15) * 4);
                buf[0] = (uint8_t)(raw >> 8);
                buf[1] = (uint8_t)(raw & 0xFF);
            }
            return 2;

        // Vehicle speed — formula: A  → 60 km/h
        case 0x0D:
            if (maxLen < 1) return -1;
            buf[0] = (uint8_t)(60 + wave);
            return 1;

        // Timing advance — formula: A/2 - 64  → 10 °
        case 0x0E:
            if (maxLen < 1) return -1;
            buf[0] = (uint8_t)(148 + wave);
            return 1;

        // Throttle position — formula: A / 2.55  → 20 %
        case 0x11:
            if (maxLen < 1) return -1;
            buf[0] = (uint8_t)(51 + wave);
            return 1;

        default:
            return -1;
    }
}
