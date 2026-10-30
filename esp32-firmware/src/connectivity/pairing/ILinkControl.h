#pragma once
#include <cstddef>
#include <cstdint>
#include "connectivity/pairing/PairingPolicy.h"

namespace pairing {

/**
 * Operações sobre o enlace BLE que a política pede (spec fase-2/02 §7).
 * Implementada pelo BleConnectivity; o PairingController chama fora do seu
 * mutex.
 */
class ILinkControl {
public:
    virtual ~ILinkControl() = default;
    virtual size_t disconnectAll() = 0;                          // 0x13; devolve quantas
    virtual bool   deleteAllBonds(uint32_t& durMs) = 0;
    virtual void   deleteBond(uint16_t conn) = 0;                // bond do peer desta conexão
    [[noreturn]] virtual void recoverStore() = 0;                // apaga nimble_bond e reinicia
    virtual void   reject(uint16_t conn, uint8_t hciReason) = 0;
    virtual void   setAuthorized(uint16_t conn) = 0;             // liga APP_CONNECTED (CONN up=1)
    virtual void   clearAuthorized(uint16_t conn) = 0;           // desliga, se `conn` é a autorizada
    virtual void   setBondingAllowed(bool allowed) = 0;
    virtual void   applyAdvertising(AdvMode mode, bool windowOpen, bool hasBond) = 0;
    virtual int    bondCount() = 0;
};

}  // namespace pairing
