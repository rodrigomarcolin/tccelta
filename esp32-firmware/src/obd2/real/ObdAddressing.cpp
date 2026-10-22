#include "obd2/real/ObdAddressing.h"

namespace {

// 29 bits: 0x18DA<destino><origem>. O Flow Control vai para a ECU que
// respondeu, então troca os dois bytes de endereço (0x18DAF110 → 0x18DA10F1).
uint32_t swapNormalFixedAddresses(uint32_t responseId) {
    const uint32_t target = (responseId >> 8) & 0xFFu;
    const uint32_t source = responseId & 0xFFu;
    return (responseId & 0xFFFF0000u) | (source << 8) | target;
}

}  // namespace

ObdAddressing ObdAddressing::standard11Bit() {
    // ID físico de requisição = ID de resposta - 8 (0x7E0/0x7E8, ...).
    return {false, 0x7DF, 0x7E0, 0x7E8, 0x7E8, 0x7EF, IsoTp::fcTargetByOffset<-8>};
}

ObdAddressing ObdAddressing::extended29Bit() {
    return {true, 0x18DB33F1, 0x18DA10F1, 0x18DAF110, 0x18DAF100, 0x18DAF1FF,
            swapNormalFixedAddresses};
}
