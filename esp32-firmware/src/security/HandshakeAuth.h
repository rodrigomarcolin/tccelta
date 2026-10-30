#pragma once
#include <cstddef>
#include <cstdint>

#include "../crypto/HmacSha256.h"

namespace security {

/** Autenticação das mensagens de handshake 0x01/0x02/0x04 (tag de 16 B). */
class IHandshakeAuth {
public:
    virtual ~IHandshakeAuth() = default;
    /** tag = HMAC-SHA256_{K_auth}("MALCONG" || type || DevId || payload)[0:16] */
    virtual void tag(uint8_t type, const uint8_t* payload, size_t len, uint8_t out[16]) = 0;
    /** Recalcula e compara em tempo constante. */
    virtual bool verify(uint8_t type, const uint8_t* payload, size_t len, const uint8_t tag[16]) = 0;
};

class HmacHandshakeAuth final : public IHandshakeAuth {
public:
    HmacHandshakeAuth(crypto::IHmacSha256& hmacAuth, const uint8_t devId[8]);
    void tag(uint8_t type, const uint8_t* payload, size_t len, uint8_t out[16]) override;
    bool verify(uint8_t type, const uint8_t* payload, size_t len, const uint8_t tag[16]) override;

private:
    crypto::IHmacSha256& _hmac;
    uint8_t _devId[8];
};

/** Perfil `none`: tag de 16 zeros, nunca verificada. */
class NoHandshakeAuth final : public IHandshakeAuth {
public:
    void tag(uint8_t, const uint8_t*, size_t, uint8_t out[16]) override;
    bool verify(uint8_t, const uint8_t*, size_t, const uint8_t[16]) override { return true; }
};

}  // namespace security
