#pragma once
#include <cstdint>

namespace pairing {

/**
 * Detector do botão de pareamento (spec fase-2/02 §4.2). Classe pura,
 * alimentada por amostras `(pressionado, nowMs)` a cada ~20 ms. O debounce é
 * por estabilidade: uma mudança só vale depois de o nível cru ficar diferente
 * do estável por `debounceMs`. Segurar `holdMs` dispara `HoldFired` uma única
 * vez por pressão. Se a primeira amostra já vier pressionada, o detector fica
 * bloqueado até ver o botão solto.
 */
class PairButton {
public:
    enum class Edge : uint8_t { None, Pressed, Released, HoldFired };

    PairButton(uint32_t debounceMs, uint32_t holdMs);

    /** Uma amostra crua (true = pressionado). Emite PAIR_BTN nas transições. */
    Edge update(bool rawPressed, uint32_t nowMs);

    bool counting() const { return _state == State::Counting; }  // pressionado, ainda sem disparar
    bool blocked() const { return _state == State::Blocked; }    // subiu pressionado; espera soltar

private:
    enum class State : uint8_t { Init, Blocked, Released, Counting, Fired };

    uint32_t _debounceMs;
    uint32_t _holdMs;
    State    _state = State::Init;
    bool     _stable = false;      // nível estável (debounced)
    bool     _candidate = false;   // nível cru em observação
    uint32_t _candidateSince = 0;  // início da instabilidade
    uint32_t _pressStart = 0;
};

}  // namespace pairing
