#include <Arduino.h>
#include "Obd2Mock.h"

// Mock de uma ECU de veículo leve a gasolina (Service 01 / SAE J1979).
// Cobre a faixa de PIDs 0x00-0x67 aplicável a esse perfil de veículo — ver
// docblock de Obd2Mock.h para o racional de quais PIDs foram incluídos.

// ── Helpers de codificação (formas de fórmula repetidas em vários PIDs) ──────

// Reverso de "valor = 100*A/255" (percentuais 0-100%).
static uint8_t encodePercent(float pct) {
    if (pct < 0.0f)   pct = 0.0f;
    if (pct > 100.0f) pct = 100.0f;
    return (uint8_t)(pct * 255.0f / 100.0f + 0.5f);
}

// Reverso de "valor = A - 40" (temperaturas em °C).
static uint8_t encodeTempC(float celsius) {
    return (uint8_t)(celsius + 40.0f);
}

// Reverso de "valor = 100*A/128 - 100" (trims/erros percentuais, ex.: fuel trim).
static uint8_t encodeTrimPercent(float pct) {
    return (uint8_t)((pct + 100.0f) * 128.0f / 100.0f);
}

static void encodeU16(uint16_t v, uint8_t* buf) {
    buf[0] = (uint8_t)(v >> 8);
    buf[1] = (uint8_t)(v & 0xFF);
}

// ── Despacho central de PIDs ──────────────────────────────────────────────
//
// Toda a lógica de "qual PID retorna o quê" mora aqui. isPidSupported() e os
// PIDs de bitmask (0x00/0x20/0x40/0x60) reaproveitam esta mesma função para
// descobrir o que está de fato implementado, então o bitmask anunciado nunca
// pode dessincronizar dos PIDs realmente respondidos.
static int dispatchPid(uint8_t pid, uint8_t* buf, size_t maxLen, uint32_t t, int8_t wave);

static bool isPidSupported(uint8_t pid, uint32_t t, int8_t wave) {
    uint8_t scratch[8];
    return dispatchPid(pid, scratch, sizeof(scratch), t, wave) >= 0;
}

// Constrói o bitmask de 4 bytes de "PIDs supported" para os 32 PIDs seguintes
// a basePid (ex.: basePid=0x00 → cobre 0x01-0x20), MSB do byte A = basePid+1.
static int buildSupportBitmask(uint8_t basePid, uint8_t* buf, size_t maxLen,
                                uint32_t t, int8_t wave) {
    if (maxLen < 4) return -1;
    buf[0] = buf[1] = buf[2] = buf[3] = 0;
    for (int i = 0; i < 32; ++i) {
        uint8_t candidate = (uint8_t)(basePid + 1 + i);
        if (isPidSupported(candidate, t, wave)) {
            buf[i / 8] |= (uint8_t)(0x80 >> (i % 8));
        }
    }
    return 4;
}

static int dispatchPid(uint8_t pid, uint8_t* buf, size_t maxLen, uint32_t t, int8_t wave) {
    switch (pid) {

        // ── PIDs supported bitmasks (derivados automaticamente) ──────────
        case 0x00: return buildSupportBitmask(0x00, buf, maxLen, t, wave);
        case 0x20: return buildSupportBitmask(0x20, buf, maxLen, t, wave);
        case 0x40: return buildSupportBitmask(0x40, buf, maxLen, t, wave);
        case 0x60: return buildSupportBitmask(0x60, buf, maxLen, t, wave);

        // Monitor status since DTCs cleared — MIL desligado, 0 DTCs, monitores completos.
        case 0x01:
            if (maxLen < 4) return -1;
            buf[0] = 0x00; buf[1] = 0x07; buf[2] = 0x00; buf[3] = 0x00;
            return 4;

        // DTC que causou o freeze frame — nenhum.
        case 0x02:
            if (maxLen < 2) return -1;
            buf[0] = 0x00; buf[1] = 0x00;
            return 2;

        // Fuel system status — banco 1 em malha fechada, banco 2 n/a.
        case 0x03:
            if (maxLen < 2) return -1;
            buf[0] = 0x02; buf[1] = 0x00;
            return 2;

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

        // Short term fuel trim — Bank 1 — oscila ±3 % em torno de 0.
        case 0x06:
            if (maxLen < 1) return -1;
            buf[0] = encodeTrimPercent(wave * 0.3f);
            return 1;

        // Long term fuel trim — Bank 1 — oscila pequeno em torno de 0.
        case 0x07:
            if (maxLen < 1) return -1;
            buf[0] = encodeTrimPercent(wave * 0.15f);
            return 1;

        // Fuel pressure (gauge) — formula: 3*A → ~300 kPa
        case 0x0A:
            if (maxLen < 1) return -1;
            buf[0] = 100;
            return 1;

        // Intake manifold absolute pressure — formula: A → oscila com a carga.
        case 0x0B:
            if (maxLen < 1) return -1;
            buf[0] = (uint8_t)(60 + wave * 2);
            return 1;

        // Engine RPM — formula: (A*256 + B) / 4  → 1500 RPM
        case 0x0C:
            if (maxLen < 2) return -1;
            {
                uint16_t raw = (uint16_t)((1500 + wave * 15) * 4);
                encodeU16(raw, buf);
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

        // Intake air temperature — formula: A - 40  → ~25 °C
        case 0x0F:
            if (maxLen < 1) return -1;
            buf[0] = encodeTempC(25 + wave / 5);
            return 1;

        // MAF — formula: (256A+B)/100 → oscila com RPM/carga.
        case 0x10:
            if (maxLen < 2) return -1;
            {
                float maf = 8.0f + wave * 0.5f; // g/s
                encodeU16((uint16_t)(maf * 100), buf);
            }
            return 2;

        // Throttle position — formula: A / 2.55  → 20 %
        case 0x11:
            if (maxLen < 1) return -1;
            buf[0] = (uint8_t)(51 + wave);
            return 1;

        // O2 sensors present (2 banks) — banco 1, sensores 1 e 2.
        case 0x13:
            if (maxLen < 1) return -1;
            buf[0] = 0x03;
            return 1;

        // O2 Sensor 1 (banco1/sensor1) — voltagem A/200, trim curto-prazo B.
        case 0x14:
            if (maxLen < 2) return -1;
            {
                float volts = 0.45f + wave * 0.01f;
                buf[0] = (uint8_t)(volts * 200.0f);
                buf[1] = encodeTrimPercent(wave * 0.2f);
            }
            return 2;

        // O2 Sensor 2 (banco1/sensor2, pós-catalisador) — trim n/a (0xFF).
        case 0x15:
            if (maxLen < 2) return -1;
            {
                float volts = 0.7f + wave * 0.02f;
                buf[0] = (uint8_t)(volts * 200.0f);
                buf[1] = 0xFF;
            }
            return 2;

        // OBD standard — 1 = OBD-II conforme CARB.
        case 0x1C:
            if (maxLen < 1) return -1;
            buf[0] = 1;
            return 1;

        // Auxiliary input status (PTO) — inativo.
        case 0x1E:
            if (maxLen < 1) return -1;
            buf[0] = 0x00;
            return 1;

        // Run time since engine start — segundos reais desde o boot.
        case 0x1F:
            if (maxLen < 2) return -1;
            encodeU16((uint16_t)(millis() / 1000), buf);
            return 2;

        // Distance traveled with MIL on — 0 (sem MIL).
        case 0x21:
            if (maxLen < 2) return -1;
            buf[0] = 0; buf[1] = 0;
            return 2;

        // Commanded EGR — formula: 100A/255 → oscila ~5-11 %.
        case 0x2C:
            if (maxLen < 1) return -1;
            buf[0] = encodePercent(8.0f + wave * 0.3f);
            return 1;

        // EGR Error — formula: 100A/128-100 → oscila pequeno em torno de 0.
        case 0x2D:
            if (maxLen < 1) return -1;
            buf[0] = encodeTrimPercent(wave * 0.2f);
            return 1;

        // Commanded evaporative purge — formula: 100A/255 → oscila ~20-40 %.
        case 0x2E:
            if (maxLen < 1) return -1;
            buf[0] = encodePercent(30.0f + wave);
            return 1;

        // Fuel Tank Level Input — formula: 100A/255 → ~65 %.
        case 0x2F:
            if (maxLen < 1) return -1;
            buf[0] = encodePercent(65.0f);
            return 1;

        // Warm-ups since codes cleared.
        case 0x30:
            if (maxLen < 1) return -1;
            buf[0] = 10;
            return 1;

        // Distance traveled since codes cleared — formula: 256A+B → ~500 km.
        case 0x31:
            if (maxLen < 2) return -1;
            encodeU16(500, buf);
            return 2;

        // Evap. System Vapor Pressure — formula: (256A+B)/4 → ~0 Pa (sistema selado).
        case 0x32:
            if (maxLen < 2) return -1;
            encodeU16(0, buf);
            return 2;

        // Absolute Barometric Pressure — formula: A → ~101 kPa (nível do mar).
        case 0x33:
            if (maxLen < 1) return -1;
            buf[0] = 101;
            return 1;

        // Catalyst Temperature: Bank 1, Sensor 1 — formula: (256A+B)/10-40 → ~400-440 °C.
        case 0x3C:
            if (maxLen < 2) return -1;
            {
                float tempC = 420.0f + wave * 2.0f;
                encodeU16((uint16_t)((tempC + 40.0f) * 10.0f), buf);
            }
            return 2;

        // Monitor status this drive cycle — byte A sempre 0x00 pela spec.
        case 0x41:
            if (maxLen < 4) return -1;
            buf[0] = 0x00; buf[1] = 0x07; buf[2] = 0x00; buf[3] = 0x00;
            return 4;

        // Control module voltage — formula: (256A+B)/1000 → ~14.2 V.
        case 0x42:
            if (maxLen < 2) return -1;
            encodeU16(14200, buf);
            return 2;

        // Absolute load value — formula: 100(256A+B)/255 → oscila como engine load.
        case 0x43:
            if (maxLen < 2) return -1;
            {
                float load = 40.0f + wave;
                encodeU16((uint16_t)(load * 255.0f / 100.0f), buf);
            }
            return 2;

        // Commanded Air-Fuel Equivalence Ratio — formula: 2(256A+B)/65536 → ~1.0.
        case 0x44:
            if (maxLen < 2) return -1;
            {
                float ratio = 1.0f + wave * 0.01f;
                encodeU16((uint16_t)(ratio * 65536.0f / 2.0f), buf);
            }
            return 2;

        // Relative throttle position — formula: 100A/255 → oscila ~10-30 %.
        case 0x45:
            if (maxLen < 1) return -1;
            buf[0] = encodePercent(20.0f + wave);
            return 1;

        // Ambient air temperature — formula: A-40 → ~25 °C.
        case 0x46:
            if (maxLen < 1) return -1;
            buf[0] = encodeTempC(25);
            return 1;

        // Absolute throttle position B — igual à posição da borboleta principal.
        case 0x47:
            if (maxLen < 1) return -1;
            buf[0] = (uint8_t)(48 + wave);
            return 1;

        // Accelerator pedal position D — pedal drive-by-wire.
        case 0x49:
            if (maxLen < 1) return -1;
            buf[0] = (uint8_t)(55 + wave);
            return 1;

        // Commanded throttle actuator — igual ao alvo de borboleta.
        case 0x4C:
            if (maxLen < 1) return -1;
            buf[0] = (uint8_t)(51 + wave);
            return 1;

        // Time run with MIL on — 0.
        case 0x4D:
            if (maxLen < 2) return -1;
            encodeU16(0, buf);
            return 2;

        // Time since trouble codes cleared — ~600 min.
        case 0x4E:
            if (maxLen < 2) return -1;
            encodeU16(600, buf);
            return 2;

        // Maximum value for Fuel-Air ratio/O2 voltage/O2 current/MAP — valores plausíveis.
        case 0x4F:
            if (maxLen < 4) return -1;
            buf[0] = 5; buf[1] = 8; buf[2] = 255; buf[3] = 25; // D×10 → 250 kPa
            return 4;

        // Maximum value for air flow rate (MAF) — A×10 → 250 g/s; B,C,D reservados.
        case 0x50:
            if (maxLen < 4) return -1;
            buf[0] = 25; buf[1] = 0; buf[2] = 0; buf[3] = 0;
            return 4;

        // Fuel Type — 1 = gasolina.
        case 0x51:
            if (maxLen < 1) return -1;
            buf[0] = 1;
            return 1;

        // Ethanol fuel % — 0 % (gasolina pura, não flex).
        case 0x52:
            if (maxLen < 1) return -1;
            buf[0] = encodePercent(0.0f);
            return 1;

        // Relative accelerator pedal position — igual ao pedal D.
        case 0x5A:
            if (maxLen < 1) return -1;
            buf[0] = (uint8_t)(55 + wave);
            return 1;

        // Engine oil temperature — formula: A-40 → ~90-100 °C.
        case 0x5C:
            if (maxLen < 1) return -1;
            buf[0] = encodeTempC(95.0f + wave / 2.0f);
            return 1;

        // Fuel injection timing — formula: (256A+B)/128-210 → oscila próximo de 0°.
        case 0x5D:
            if (maxLen < 2) return -1;
            {
                float deg = wave * 0.5f;
                encodeU16((uint16_t)((deg + 210.0f) * 128.0f), buf);
            }
            return 2;

        // Engine fuel rate — formula: (256A+B)/20 → oscila ~1-7 L/h.
        case 0x5E:
            if (maxLen < 2) return -1;
            {
                float lph = 4.0f + wave * 0.3f;
                encodeU16((uint16_t)(lph * 20.0f), buf);
            }
            return 2;

        // Driver's demand engine - percent torque — formula: A-125 → oscila ~20-40 %.
        case 0x61:
            if (maxLen < 1) return -1;
            buf[0] = (uint8_t)(30 + wave + 125);
            return 1;

        // Actual engine - percent torque — acoplado à carga.
        case 0x62:
            if (maxLen < 1) return -1;
            buf[0] = (uint8_t)(25 + wave + 125);
            return 1;

        // Engine reference torque — formula: 256A+B → 300 N·m.
        case 0x63:
            if (maxLen < 2) return -1;
            encodeU16(300, buf);
            return 2;

        // Engine percent torque data (idle, pt1-4) — curva plausível fixa.
        case 0x64:
            if (maxLen < 5) return -1;
            buf[0] = 125; buf[1] = 150; buf[2] = 175; buf[3] = 200; buf[4] = 225;
            return 5;

        // Auxiliary input/output supported — nada além do padrão.
        case 0x65:
            if (maxLen < 2) return -1;
            buf[0] = 0x00; buf[1] = 0x00;
            return 2;

        // Mass air flow sensor (dual) — só sensor A presente, acoplado ao PID 0x10.
        case 0x66:
            if (maxLen < 5) return -1;
            {
                float maf = 8.0f + wave * 0.5f;
                uint16_t raw = (uint16_t)(maf * 32.0f);
                buf[0] = 0x80; // só sensor A suportado
                buf[1] = (uint8_t)(raw >> 8);
                buf[2] = (uint8_t)(raw & 0xFF);
                buf[3] = 0; buf[4] = 0;
            }
            return 5;

        // Engine coolant temperature (dual sensor) — só sensor 1, igual ao PID 0x05.
        case 0x67:
            if (maxLen < 3) return -1;
            buf[0] = 0x80; // só sensor 1 suportado
            buf[1] = (uint8_t)(130 + wave);
            buf[2] = 0;
            return 3;

        default:
            return -1;
    }
}

// ── IObd2 interface ─────────────────────────────────────────────────────────

bool Obd2Mock::begin() { return true; }

int Obd2Mock::readPid(uint8_t service, uint8_t pid,
                      uint8_t* buf, size_t maxLen) {
    if (service != 0x01) return -1;

    // Gentle oscillation: ±10 % variation over time so live-data screens
    // show something moving without overwhelming the log.
    uint32_t t = millis() / 1000u;  // seconds
    int8_t   wave = (int8_t)(10 * sin((float)t * 0.5f));  // -10 … +10

    return dispatchPid(pid, buf, maxLen, t, wave);
}
