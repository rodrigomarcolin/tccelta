#include "Crc32.h"

namespace crypto {

namespace {
// Tabela de meio byte (16 entradas, poly 0xEDB88320 refletido). Escrita como
// literal para compilar também em C++11 (o build do ESP-IDF não usa C++14).
constexpr uint32_t kNibble[16] = {0x00000000, 0x1DB71064, 0x3B6E20C8, 0x26D930AC,
                                  0x76DC4190, 0x6B6B51F4, 0x4DB26158, 0x5005713C,
                                  0xEDB88320, 0xF00F9344, 0xD6D6A3E8, 0xCB61B38C,
                                  0x9B64C2B0, 0x86D3D2D4, 0xA00AE278, 0xBDBDF21C};
}  // namespace

uint32_t crc32(const uint8_t* data, size_t len, uint32_t previous) {
    uint32_t c = ~previous;
    for (size_t i = 0; i < len; i++) {
        c ^= data[i];
        c = kNibble[c & 0x0F] ^ (c >> 4);
        c = kNibble[c & 0x0F] ^ (c >> 4);
    }
    return ~c;
}

}  // namespace crypto
