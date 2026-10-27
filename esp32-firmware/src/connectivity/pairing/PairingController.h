#pragma once
#include <freertos/FreeRTOS.h>
#include <freertos/semphr.h>
#include "connectivity/pairing/ILinkControl.h"
#include "connectivity/pairing/PairingPolicy.h"

namespace pairing {

/**
 * Cola a PairingPolicy (pura) ao RTOS e ao enlace (spec fase-2/02 §4.5). É
 * chamado da tarefa host do NimBLE e da PairingTask: a política fica sob um
 * mutex e as ações pedidas são executadas depois de soltá-lo, para o nosso
 * lock nunca ser segurado enquanto a NimBLE pega o dela.
 */
class PairingController {
public:
    PairingController(ILinkControl& link, const Timings& timings, uint8_t maxConnections);

    void begin();                                                      // CollectorApp
    void onLinkUp(uint16_t conn);                                      // tarefa host NimBLE
    void onEncryptionChange(uint16_t conn, const EncryptionInfo& info);
    void onLinkDown(uint16_t conn);
    void onButtonHold();                                               // PairingTask
    void tick();

    bool windowOpen() const;  // LED e LINK_UP
    bool authorized() const;  // LED

private:
    void execute(const Actions& actions);

    ILinkControl&             _link;
    PairingPolicy             _policy;
    mutable SemaphoreHandle_t _mutex;
};

}  // namespace pairing
