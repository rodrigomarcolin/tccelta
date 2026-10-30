#pragma once
#include <mbedtls/ecdsa.h>
#include "collector/IMessageAuth.h"

namespace security {

/**
 * Perfil `asym` (desenvolvimento/benchmark; spec fase-1/01 §4): ECDSA P-256
 * determinística (RFC 6979), tag = r (32 B, big-endian) || s (32 B), 64 B de
 * tamanho fixo. A entrada é a mesma da tag HMAC: SHA-256("MALCONG" || 0x03 ||
 * DevId || dados). Não é reentrante: uma instância por tarefa.
 */
class EcdsaBlockAuth final : public collector::IMessageAuth {
public:
    EcdsaBlockAuth(const uint8_t kSig[32], const uint8_t devId[8]);
    ~EcdsaBlockAuth() override;
    EcdsaBlockAuth(const EcdsaBlockAuth&) = delete;
    EcdsaBlockAuth& operator=(const EcdsaBlockAuth&) = delete;

    size_t tagSize() const override { return 64; }
    void sign(const uint8_t* data, size_t len, uint8_t* tagOut) override;
    bool verify(const uint8_t* data, size_t len, const uint8_t* tag) override;

private:
    mbedtls_ecp_group _grp;
    mbedtls_mpi _d;
    uint8_t _devId[8];

    void digest(const uint8_t* data, size_t len, uint8_t out[32]) const;
};

}  // namespace security
