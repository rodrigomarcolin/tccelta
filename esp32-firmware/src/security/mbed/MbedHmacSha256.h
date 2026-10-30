#pragma once
#include <mbedtls/md.h>
#include "crypto/HmacSha256.h"

namespace security {

/**
 * HMAC-SHA256 do mbedTLS com SHA em hardware (só ESP32; spec fase-1/01 §3).
 * A chave é carregada uma vez por instância e cada MAC usa
 * `mbedtls_md_hmac_reset`, que reaproveita o estado ipad/opad. Um contexto
 * por instância: não é reentrante, uma instância por tarefa.
 */
class MbedHmacSha256 final : public crypto::IHmacSha256 {
public:
    MbedHmacSha256(const uint8_t* key, size_t keyLen);
    ~MbedHmacSha256() override;
    MbedHmacSha256(const MbedHmacSha256&) = delete;
    MbedHmacSha256& operator=(const MbedHmacSha256&) = delete;

    void begin() override;
    void update(const uint8_t* data, size_t len) override;
    void finish(uint8_t out[crypto::kSha256Size]) override;

private:
    mbedtls_md_context_t _ctx;
};

}  // namespace security
