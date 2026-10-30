#pragma once
#include "../collector/IMessageAuth.h"
#include "../crypto/HmacSha256.h"

namespace security {

/**
 * Tag do bloco = HMAC-SHA256_{K_rec}("MALCONG" || 0x03 || DevId || dados),
 * 32 B (spec fase-1/01 §4). Não é reentrante: uma instância por tarefa.
 */
class HmacBlockAuth final : public collector::IMessageAuth {
public:
    HmacBlockAuth(crypto::IHmacSha256& hmacRec, const uint8_t devId[8]);
    size_t tagSize() const override { return 32; }
    bool sign(const uint8_t* data, size_t len, uint8_t* tagOut) override;
    bool verify(const uint8_t* data, size_t len, const uint8_t* tag) override;

private:
    crypto::IHmacSha256& _hmac;
    uint8_t _devId[8];
};

}  // namespace security
