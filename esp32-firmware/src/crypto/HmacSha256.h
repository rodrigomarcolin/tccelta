#pragma once
#include <cstddef>
#include <cstdint>

#include "Sha256.h"

namespace crypto {

/** HMAC-SHA256 com a chave fixada na construção; reutilizável. */
class IHmacSha256 {
public:
    virtual ~IHmacSha256() = default;
    virtual void begin() = 0;  // novo MAC
    virtual void update(const uint8_t* data, size_t len) = 0;
    virtual void finish(uint8_t out[kSha256Size]) = 0;
};

/** RFC 2104 sobre `Sha256`; guarda ipad/opad já misturados com a chave. */
class PortableHmacSha256 final : public IHmacSha256 {
public:
    PortableHmacSha256(const uint8_t* key, size_t keyLen);
    ~PortableHmacSha256() override;  // zera ipad/opad
    void begin() override;
    void update(const uint8_t* data, size_t len) override;
    void finish(uint8_t out[kSha256Size]) override;

private:
    uint8_t _ipad[kSha256Block];
    uint8_t _opad[kSha256Block];
    Sha256 _inner;
};

}  // namespace crypto
