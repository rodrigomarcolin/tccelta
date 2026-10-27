#pragma once
#include <cstddef>
#include <cstdint>

namespace crypto {

/**
 * CRC-32/ISO-HDLC (zlib): poly 0xEDB88320 refletido, init/xorout 0xFFFFFFFF.
 * Encadeia: `crc32(b, nb, crc32(a, na)) == crc32(a‖b)`.
 */
uint32_t crc32(const uint8_t* data, size_t len, uint32_t previous = 0);

}  // namespace crypto
