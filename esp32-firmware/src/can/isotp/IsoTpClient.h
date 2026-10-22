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
 * Também não depende de nenhuma convenção de protocolo de diagnóstico: quem
 * cria o cliente fornece, na construção, `negativeResponseMarker` (para
 * reconhecer erro), `fcTargetFor` (para endereçar o Flow Control ao peer
 * certo) e se os IDs são de 29 bits — fixos para toda a vida do cliente,
 * pois não variam entre requisições na mesma sessão. `expectedReply` (para
 * correlacionar resposta ↔ requisição) já muda a cada chamada, então
 * continua sendo parâmetro de `request`/`requestAll`. Essas convenções são
 * de quem usa ISO-TP por cima (ex.: OBD-II/UDS), não do transporte em si —
 * ver `Obd2Can`, que preenche esses parâmetros com os valores da SAE J1979.
 *
 * Implementa emissão e recepção completas (Single Frame, First Frame,
 * Consecutive Frame, Flow Control), incluindo interpretação de FlowStatus
 * (CTS/Wait/Overflow), Block Size e STmin do lado emissor. O lado receptor
 * sempre responde com FC (CTS, BS=0, STmin=0): o dongle não tem motivo para
 * pedir pausas ao peer.
 */
namespace IsoTp {

constexpr size_t MAX_RESPONSES = 8;
constexpr size_t MAX_RESPONSE_BYTES = 256;

struct Response {
    uint32_t ecuId = 0;
    uint8_t data[MAX_RESPONSE_BYTES] = {};
    size_t len = 0;
};

struct ResponseSet {
    Response items[MAX_RESPONSES];
    size_t count = 0;
};

// Timeouts de rede, ISO 15765-2 (N_As/N_Bs/N_Cr = 1000ms cada).
constexpr uint32_t N_AS_MS = 1000;  // emissor: tempo para transmitir 1 frame
constexpr uint32_t N_BS_MS = 1000;  // emissor: espera pelo próximo FC
constexpr uint32_t N_CR_MS = 1000;  // receptor: espera pelo próximo CF

// Limite de FCs "Wait" consecutivos antes de desistir (o padrão não impõe um
// número máximo; este limite evita loop efetivamente infinito se o peer
// mandar Wait indefinidamente — mesma cautela adotada pelo altelch/iso-tp).
constexpr uint8_t MAX_FC_WAIT = 8;

/**
 * Bytes iniciais que uma resposta precisa ter para pertencer à transação
 * corrente — ex.: `{0x41, 0x0C}` (SID + PID). Conferir o PID, e não só o SID,
 * faz com que a resposta atrasada de uma requisição anterior seja descartada
 * em vez de encerrar a espera da atual.
 */
struct ReplyPrefix {
    uint8_t bytes[2] = {};
    uint8_t len = 0;

    ReplyPrefix(uint8_t sid) : bytes{sid, 0}, len(1) {}  // NOLINT: conversão implícita intencional
    ReplyPrefix(uint8_t sid, uint8_t pid) : bytes{sid, pid}, len(2) {}
};

/** Calcula o ID para onde mandar o Flow Control a partir do ID de quem respondeu. */
using FcTargetFn = uint32_t (*)(uint32_t responseId);

/** Endereçamento de 11 bits: ID físico = ID de resposta + Offset (ex.: -8). */
template <int32_t Offset>
uint32_t fcTargetByOffset(uint32_t responseId) {
    return static_cast<uint32_t>(static_cast<int64_t>(responseId) + Offset);
}

enum Result : int {
    TIMEOUT           = -1,
    NEGATIVE_RESPONSE = -2,  // outBuf = [origService, NRC]
    OVERFLOW_ABORT    = -3,
};

/**
 * Cliente vinculado a um `ICanBus` e às convenções de matching/endereçamento
 * do protocolo de aplicação que roda por cima — fixadas uma vez na
 * construção, já que não mudam entre requisições na mesma sessão (ex.: uma
 * instância por `Obd2Can`).
 */
class Client {
public:
    Client(ICanBus* can, uint8_t negativeResponseMarker, FcTargetFn fcTargetFor,
           bool extendedIds = false)
        : _can(can), _negativeResponseMarker(negativeResponseMarker),
          _fcTargetFor(fcTargetFor), _extendedIds(extendedIds) {}

    /**
     * Envia `req` (SF se <=7 bytes; senão FF+CF, obedecendo o FC do peer) em
     * `reqId`, depois aguarda a resposta funcional em [respIdMin, respIdMax]
     * (SF ou FF+CF, gerando o FC de volta).
     *
     * @param expectedReply prefixo que a resposta deve ter (SID, ou SID +
     *                      PID) para ser aceita como desta transação
     *                      (descarta sobras de transações anteriores).
     * @param outBuf     recebe o payload já reassemblado (SID de resposta +
     *                   dados, sem bytes de PCI/ISO-TP)
     * @param timeoutMs  tempo de espera pela primeira resposta (N_Bs). Default
     *                   N_BS_MS (1000ms, valor da norma) — chamadores que
     *                   esperam "NO DATA" com frequência (ex.: varredura de
     *                   PIDs de telemetria) podem passar um valor menor para
     *                   não pagar 1s por PID não suportado.
     * @return  nº de bytes escritos em outBuf (>=0), ou um valor de Result (<0)
     */
    int request(uint32_t reqId, uint32_t respIdMin, uint32_t respIdMax,
                const uint8_t* req, size_t reqLen,
                const ReplyPrefix& expectedReply,
                uint8_t* outBuf, size_t maxLen,
                uint32_t timeoutMs = N_BS_MS);

    int requestAll(uint32_t reqId, uint32_t respIdMin, uint32_t respIdMax,
                   const uint8_t* req, size_t reqLen,
                   const ReplyPrefix& expectedReply,
                   ResponseSet& responses,
                   uint32_t timeoutMs = N_BS_MS,
                   size_t expectedResponses = 0);

private:
    ICanBus* _can;
    uint8_t    _negativeResponseMarker;
    FcTargetFn _fcTargetFor;
    bool       _extendedIds;
};

}  // namespace IsoTp
