#include "BlockAuth.h"

#include <cstring>

#include "../crypto/ConstantTime.h"
#include "DeviceKeys.h"

namespace security {

HmacBlockAuth::HmacBlockAuth(crypto::IHmacSha256& hmacRec, const uint8_t devId[8]) : _hmac(hmacRec) {
    memcpy(_devId, devId, sizeof(_devId));
}

void HmacBlockAuth::sign(const uint8_t* data, size_t len, uint8_t* tagOut) {
    const uint8_t type = kTypeBlock;
    _hmac.begin();
    _hmac.update(kDomainLabel, sizeof(kDomainLabel));
    _hmac.update(&type, 1);
    _hmac.update(_devId, sizeof(_devId));
    _hmac.update(data, len);
    _hmac.finish(tagOut);
}

bool HmacBlockAuth::verify(const uint8_t* data, size_t len, const uint8_t* tag) {
    uint8_t expected[32];
    sign(data, len, expected);
    bool ok = crypto::ctEqual(expected, tag, sizeof(expected));
    crypto::secureZero(expected, sizeof(expected));
    return ok;
}

}  // namespace security
