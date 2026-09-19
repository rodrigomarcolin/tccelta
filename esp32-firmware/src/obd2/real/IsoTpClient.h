#pragma once
#include <cstdint>
#include <cstddef>
#include "can/ICanBus.h"

/**
 * Cliente ISO-TP (ISO 15765-2) sobre ICanBus.
 *
 * Não depende de nenhuma implementação concreta de CAN — ao contrário da lib
 * `altelch/iso-tp` usada no simulador (acoplada a `MCP_CAN*`), este cliente
 * funciona com qualquer ICanBus (MCP2515, TWAI, ou um fake em teste).
 *
 * Implementa emissão e recepção completas (Single Frame, First Frame,
 * Consecutive Frame, Flow Control), incluindo interpretação de FlowStatus
 * (CTS/Wait/Overflow), Block Size e STmin do lado emissor. O lado receptor
 * sempre responde com FC (CTS, BS=0, STmin=0): o dongle não tem motivo para
 * pedir pausas ao peer.
 */
namespace IsoTp {

// Timeouts de rede, ISO 15765-2 (N_As/N_Bs/N_Cr = 1000ms cada).
constexpr uint32_t N_AS_MS = 1000;  // emissor: tempo para transmitir 1 frame
constexpr uint32_t N_BS_MS = 1000;  // emissor: espera pelo próximo FC
constexpr uint32_t N_CR_MS = 1000;  // receptor: espera pelo próximo CF

// Limite de FCs "Wait" consecutivos antes de desistir (o padrão não impõe um
// número máximo; este limite evita loop efetivamente infinito se o peer
// mandar Wait indefinidamente — mesma cautela adotada pelo altelch/iso-tp).
constexpr uint8_t MAX_FC_WAIT = 8;

enum Result : int {
    TIMEOUT           = -1,
    NEGATIVE_RESPONSE = -2,  // outBuf = [origService, NRC]
    OVERFLOW_ABORT    = -3,
};

/**
 * Envia `req` (SF se <=7 bytes; senão FF+CF, obedecendo o FC do peer) em
 * `reqId`, depois aguarda a resposta funcional em [respIdMin, respIdMax]
 * (SF ou FF+CF, gerando o FC de volta).
 *
 * @param outBuf  recebe o payload OBD já reassemblado (SID de resposta +
 *                dados, sem bytes de PCI/ISO-TP)
 * @return  nº de bytes escritos em outBuf (>=0), ou um valor de Result (<0)
 */
int request(ICanBus* can, uint32_t reqId, uint32_t respIdMin, uint32_t respIdMax,
            const uint8_t* req, size_t reqLen,
            uint8_t* outBuf, size_t maxLen);

}  // namespace IsoTp
