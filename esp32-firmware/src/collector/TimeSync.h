#pragma once
#include <cstdint>
#include "collector/DataProtocol.h"

namespace collector {

constexpr uint32_t kTimeSyncTimeoutMs = 5000;

/**
 * Lado Dongle do algoritmo de Cristian (§2.3): anota t1 ao pedir a hora, t2
 * ao receber a resposta, e relata (boot_id, t1, t2) ao servidor, que calcula
 * a âncora. A resposta só é aceita se ecoar o boot_id e o t1 do pedido
 * pendente.
 */
class TimeSync {
public:
    TimeExchange start(uint16_t bootId, uint32_t nowMs);
    bool accept(const TimeExchange& response, uint32_t nowMs);

    bool pending() const { return _pending; }
    bool done() const { return _done; }
    bool timedOut(uint32_t nowMs) const;
    const TimeExchange& report() const { return _exchange; }

private:
    TimeExchange _exchange = {};
    bool _pending = false;
    bool _done    = false;
};

}  // namespace collector
