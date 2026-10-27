#include "HandshakeAuth.h"

#include <cstring>

#include "../crypto/ConstantTime.h"
#include "DeviceKeys.h"

namespace security {

HmacHandshakeAuth::HmacHandshakeAuth(crypto::IHmacSha256& hmacAuth, const uint8_t devId[8])
    : _hmac(hmacAuth) {
    memcpy(_devId, devId, sizeof(_devId));
}

void HmacHandshakeAuth::tag(uint8_t type, const uint8_t* payload, size_t len, uint8_t out[16]) {
    uint8_t full[crypto::kSha256Size];
    _hmac.begin();
    _hmac.update(kDomainLabel, sizeof(kDomainLabel));
    _hmac.update(&type, 1);
    _hmac.update(_devId, sizeof(_devId));
    _hmac.update(payload, len);
    _hmac.finish(full);
    memcpy(out, full, 16);
    crypto::secureZero(full, sizeof(full));
}

bool HmacHandshakeAuth::verify(uint8_t type, const uint8_t* payload, size_t len, const uint8_t tag_[16]) {
    uint8_t expected[16];
    tag(type, payload, len, expected);
    bool ok = crypto::ctEqual(expected, tag_, sizeof(expected));
    crypto::secureZero(expected, sizeof(expected));
    return ok;
}

void NoHandshakeAuth::tag(uint8_t, const uint8_t*, size_t, uint8_t out[16]) { memset(out, 0, 16); }

}  // namespace security
