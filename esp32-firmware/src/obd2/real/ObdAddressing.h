#pragma once
#include <cstdint>
#include "can/isotp/IsoTpClient.h"

/**
 * IDs CAN do OBD-II (ISO 15765-4) para cada tamanho de identificador.
 *
 * - Funcional: requisição em broadcast, respondida por todas as ECUs que
 *   suportam o pedido. Usada no diagnóstico.
 * - Física ao motor: requisição endereçada só à ECU do motor (ECU #1),
 *   usada pela coleta para não receber respostas de várias ECUs.
 */
struct ObdAddressing {
    bool              extended;
    uint32_t          functionalRequestId;
    uint32_t          engineRequestId;
    uint32_t          engineResponseId;
    uint32_t          responseIdMin;
    uint32_t          responseIdMax;
    IsoTp::FcTargetFn fcTargetFor;

    static ObdAddressing standard11Bit();
    static ObdAddressing extended29Bit();
};
