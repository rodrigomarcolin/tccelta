#pragma once
#include <cstddef>
#include <cstdint>

namespace crypto {

constexpr size_t kSha256Size = 32;
constexpr size_t kSha256Block = 64;

/** SHA-256 portátil (FIPS 180-4). Usado no native e como referência. */
class Sha256 {
public:
    Sha256() { begin(); }
    void begin();
    void update(const uint8_t* data, size_t len);
    void finish(uint8_t out[kSha256Size]);

private:
    void compress(const uint8_t block[kSha256Block]);

    uint32_t _h[8];
    uint64_t _bits;
    uint8_t _buf[kSha256Block];
    size_t _used;
};

}  // namespace crypto
