#include "connectivity/pairing/PairButton.h"
#include "collector/Events.h"
#include "connectivity/pairing/PairTime.h"

namespace pairing {

PairButton::PairButton(uint32_t debounceMs, uint32_t holdMs) : _debounceMs(debounceMs), _holdMs(holdMs) {}

PairButton::Edge PairButton::update(bool raw, uint32_t now) {
    if (_state == State::Init) {
        _stable = raw;
        _candidate = raw;
        _candidateSince = now;
        if (raw) {
            _state = State::Blocked;
            collector::Event("PAIR_BTN").token("action", "blocked").num("held", 0).flag("fired", false);
        } else {
            _state = State::Released;
        }
        return Edge::None;
    }

    // Debounce: o nível só muda depois de `debounceMs` de estabilidade.
    bool changed = false;
    if (raw == _stable) {
        _candidate = _stable;
    } else {
        if (_candidate != raw) {
            _candidate = raw;
            _candidateSince = now;
        }
        if (reached(now, _candidateSince + _debounceMs)) {
            _stable = raw;
            changed = true;
        }
    }

    switch (_state) {
        case State::Blocked:
            if (changed && !_stable) _state = State::Released;
            return Edge::None;

        case State::Released:
            if (changed && _stable) {
                _state = State::Counting;
                _pressStart = _candidateSince;
                collector::Event("PAIR_BTN").token("action", "press").num("held", 0).flag("fired", false);
                return Edge::Pressed;
            }
            return Edge::None;

        case State::Counting:
        case State::Fired: {
            const bool fired = _state == State::Fired;
            if (changed && !_stable) {
                const int32_t held = elapsedMs(_candidateSince, _pressStart);
                _state = State::Released;
                collector::Event("PAIR_BTN").token("action", "release").num("held", held).flag("fired", fired);
                return Edge::Released;
            }
            if (!fired && reached(now, _pressStart + _holdMs)) {
                _state = State::Fired;
                collector::Event("PAIR_BTN").token("action", "hold").num("held", elapsedMs(now, _pressStart))
                    .flag("fired", true);
                return Edge::HoldFired;
            }
            return Edge::None;
        }

        case State::Init:
            break;
    }
    return Edge::None;
}

}  // namespace pairing
