#include "HmacSha256.h"

#include <cstring>

#include "ConstantTime.h"

namespace crypto {

PortableHmacSha256::PortableHmacSha256(const uint8_t* key, size_t keyLen) {
    uint8_t k[kSha256Block] = {0};
    if (keyLen > kSha256Block) {
        Sha256 s;
        s.update(key, keyLen);
        s.finish(k);
    } else if (keyLen > 0) {
        memcpy(k, key, keyLen);
    }
    for (size_t i = 0; i < kSha256Block; i++) {
        _ipad[i] = k[i] ^ 0x36;
        _opad[i] = k[i] ^ 0x5c;
    }
    secureZero(k, sizeof(k));
    begin();
}

PortableHmacSha256::~PortableHmacSha256() {
    secureZero(_ipad, sizeof(_ipad));
    secureZero(_opad, sizeof(_opad));
}

void PortableHmacSha256::begin() {
    _inner.begin();
    _inner.update(_ipad, kSha256Block);
}

void PortableHmacSha256::update(const uint8_t* data, size_t len) { _inner.update(data, len); }

void PortableHmacSha256::finish(uint8_t out[kSha256Size]) {
    uint8_t innerHash[kSha256Size];
    _inner.finish(innerHash);
    Sha256 outer;
    outer.update(_opad, kSha256Block);
    outer.update(innerHash, kSha256Size);
    outer.finish(out);
    secureZero(innerHash, sizeof(innerHash));
}

}  // namespace crypto
