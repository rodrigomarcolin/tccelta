#pragma once
#include <cstdint>
#include "connectivity/pairing/PairingPolicy.h"

/**
 * Parâmetros do pareamento por build flag (spec fase-2/02 §5). Os padrões
 * moram aqui para um .ini sem a seção [pairing] ainda compilar. Só é incluído
 * pelo código do ESP32.
 */

#ifndef PAIR_BUTTON_PIN
#define PAIR_BUTTON_PIN 0
#endif
#ifndef PAIR_BUTTON_ACTIVE_LOW
#define PAIR_BUTTON_ACTIVE_LOW 1
#endif
#ifndef PAIR_BUTTON_HOLD_MS
#define PAIR_BUTTON_HOLD_MS 5000
#endif
#ifndef PAIR_DEBOUNCE_MS
#define PAIR_DEBOUNCE_MS 50
#endif
#ifndef PAIR_WINDOW_MS
#define PAIR_WINDOW_MS 45000
#endif
#ifndef PAIR_AUTH_TIMEOUT_MS
#define PAIR_AUTH_TIMEOUT_MS 35000
#endif
#ifndef LINK_AUTH_TIMEOUT_MS
#define LINK_AUTH_TIMEOUT_MS 5000
#endif
#ifndef PAIR_LED_PIN
#define PAIR_LED_PIN -1
#endif

static_assert(PAIR_BUTTON_PIN >= 0 && PAIR_BUTTON_PIN <= 39, "PAIR_BUTTON_PIN fora de 0-39");
static_assert(PAIR_BUTTON_ACTIVE_LOW == 0 || PAIR_BUTTON_ACTIVE_LOW == 1, "PAIR_BUTTON_ACTIVE_LOW deve ser 0 ou 1");
static_assert(PAIR_BUTTON_HOLD_MS >= 1000 && PAIR_BUTTON_HOLD_MS <= 30000, "PAIR_BUTTON_HOLD_MS fora de 1000-30000");
static_assert(PAIR_DEBOUNCE_MS >= 10 && PAIR_DEBOUNCE_MS <= 200, "PAIR_DEBOUNCE_MS fora de 10-200");
static_assert(PAIR_WINDOW_MS >= 10000 && PAIR_WINDOW_MS <= 300000, "PAIR_WINDOW_MS fora de 10000-300000");
static_assert(PAIR_AUTH_TIMEOUT_MS >= 10000 && PAIR_AUTH_TIMEOUT_MS <= 60000,
              "PAIR_AUTH_TIMEOUT_MS fora de 10000-60000");
static_assert(LINK_AUTH_TIMEOUT_MS >= 1000 && LINK_AUTH_TIMEOUT_MS <= 30000,
              "LINK_AUTH_TIMEOUT_MS fora de 1000-30000");
static_assert(PAIR_LED_PIN == -1 || (PAIR_LED_PIN >= 0 && PAIR_LED_PIN <= 33), "PAIR_LED_PIN deve ser -1 ou 0-33");

/** Parâmetros da tarefa que amostra o botão. */
struct PairingTaskConfig {
    int      pin;
    bool     activeLow;
    uint32_t debounceMs;
    uint32_t holdMs;
    uint32_t periodMs;
    int      ledPin;

    static PairingTaskConfig fromBuildFlags() {
        return PairingTaskConfig{PAIR_BUTTON_PIN, PAIR_BUTTON_ACTIVE_LOW != 0, PAIR_DEBOUNCE_MS,
                                 PAIR_BUTTON_HOLD_MS, 20, PAIR_LED_PIN};
    }
};

inline pairing::Timings timingsFromConfig() {
    pairing::Timings t{};
    t.windowMs = PAIR_WINDOW_MS;
    t.linkAuthTimeoutMs = LINK_AUTH_TIMEOUT_MS;
    t.pairAuthTimeoutMs = PAIR_AUTH_TIMEOUT_MS;
    return t;
}
