#pragma once
#include "obd2/IObd2.h"
#include "obd2/real/ObdAddressing.h"
#include "can/ICanBus.h"
#include "can/isotp/IsoTpClient.h"

/**
 * Implementação IObd2 que se comunica com uma ECU real via CAN
 * usando ISO 15765-4 (OBD-II on CAN), sobre o cliente ISO-TP genérico
 * (`IsoTpClient`) — cobre Single Frame e multiframe (First/Consecutive
 * Frame), embora Modo 01/09 na prática nunca gere resposta >7 bytes.
 *
 * O diagnóstico usa requisição funcional; a coleta (`readEnginePid`) usa
 * requisição física ao motor. O tamanho do identificador (11 ou 29 bits)
 * vem de `ObdAddressing`.
 */
class Obd2Can : public IObd2 {
public:
    /**
     * @param can        Implementação ICanBus (Mcp2515Can ou TwaiCan)
     * @param addressing IDs de 11 ou 29 bits
     * @param timeoutMs  Timeout da resposta em Ms (default 200)
     */
    explicit Obd2Can(ICanBus* can,
                     const ObdAddressing& addressing = ObdAddressing::standard11Bit(),
                     uint32_t timeoutMs = 200);

    bool begin() override;
    void setResponseTimeoutMs(uint32_t timeoutMs) override { _timeoutMs = timeoutMs; }
    int  readPid(uint8_t service, uint8_t pid,
                 uint8_t* buf, size_t maxLen) override;
    int  readPidAll(uint8_t service, uint8_t pid,
                    Obd2ResponseSet& responses,
                    uint32_t timeoutMs = 200,
                    size_t expectedResponses = 0) override;
    int  readEnginePid(uint8_t pid, uint8_t* buf, size_t maxLen,
                       uint32_t timeoutMs) override;
    int  readDtc(uint8_t service, uint16_t* dtcCodes, size_t maxCount) override;
    int  readDtcAll(uint8_t service, Obd2ResponseSet& responses,
                    uint32_t timeoutMs = 200,
                    size_t expectedResponses = 0) override;
    int  readFreezeFramePid(uint8_t pid, uint8_t* buf, size_t maxLen) override;
    int  readFreezeFramePidAll(uint8_t pid, Obd2ResponseSet& responses,
                               uint32_t timeoutMs = 200,
                               size_t expectedResponses = 0) override;

private:
    ICanBus*      _can;
    ObdAddressing _addr;
    uint32_t      _timeoutMs;
    IsoTp::Client _isoTp;

    int requestFunctional(const uint8_t* req, size_t reqLen,
                          const IsoTp::ReplyPrefix& reply,
                          IsoTp::ResponseSet& raw, uint32_t timeoutMs,
                          size_t expectedResponses);
};
