#include "security/mbed/EcdsaBlockAuth.h"

#include <esp_system.h>
#include <mbedtls/sha256.h>
#include <cstring>
#include "crypto/ConstantTime.h"
#include "security/DeviceKeys.h"

namespace security {

namespace {
// Cegamento da multiplicação escalar pelo RNG do ESP32; a assinatura em si é determinística.
int blindingRng(void*, unsigned char* out, size_t len) {
    esp_fill_random(out, len);
    return 0;
}
}  // namespace

EcdsaBlockAuth::EcdsaBlockAuth(const uint8_t kSig[32], const uint8_t devId[8]) {
    mbedtls_ecp_group_init(&_grp);
    mbedtls_mpi_init(&_d);
    mbedtls_ecp_group_load(&_grp, MBEDTLS_ECP_DP_SECP256R1);
    mbedtls_mpi_read_binary(&_d, kSig, 32);
    memcpy(_devId, devId, sizeof(_devId));
}

EcdsaBlockAuth::~EcdsaBlockAuth() {
    mbedtls_mpi_free(&_d);  // zera o escalar
    mbedtls_ecp_group_free(&_grp);
}

void EcdsaBlockAuth::digest(const uint8_t* data, size_t len, uint8_t out[32]) const {
    const uint8_t type = kTypeBlock;
    mbedtls_sha256_context ctx;
    mbedtls_sha256_init(&ctx);
    mbedtls_sha256_starts_ret(&ctx, 0);
    mbedtls_sha256_update_ret(&ctx, kDomainLabel, sizeof(kDomainLabel));
    mbedtls_sha256_update_ret(&ctx, &type, 1);
    mbedtls_sha256_update_ret(&ctx, _devId, sizeof(_devId));
    mbedtls_sha256_update_ret(&ctx, data, len);
    mbedtls_sha256_finish_ret(&ctx, out);
    mbedtls_sha256_free(&ctx);
}

void EcdsaBlockAuth::sign(const uint8_t* data, size_t len, uint8_t* tagOut) {
    uint8_t hash[32];
    digest(data, len, hash);
    mbedtls_mpi r, s;
    mbedtls_mpi_init(&r);
    mbedtls_mpi_init(&s);
    const int rc = mbedtls_ecdsa_sign_det_ext(&_grp, &r, &s, &_d, hash, sizeof(hash), MBEDTLS_MD_SHA256,
                                              blindingRng, nullptr);
    if (rc == 0) {
        mbedtls_mpi_write_binary(&r, tagOut, 32);
        mbedtls_mpi_write_binary(&s, tagOut + 32, 32);
    } else {
        memset(tagOut, 0, 64);  // tag inválida: o servidor rejeita
    }
    mbedtls_mpi_free(&r);
    mbedtls_mpi_free(&s);
}

bool EcdsaBlockAuth::verify(const uint8_t* data, size_t len, const uint8_t* tag) {
    uint8_t hash[32];
    digest(data, len, hash);
    mbedtls_ecp_point q;
    mbedtls_mpi r, s;
    mbedtls_ecp_point_init(&q);
    mbedtls_mpi_init(&r);
    mbedtls_mpi_init(&s);
    bool ok = mbedtls_ecp_mul(&_grp, &q, &_d, &_grp.G, blindingRng, nullptr) == 0 &&
              mbedtls_mpi_read_binary(&r, tag, 32) == 0 && mbedtls_mpi_read_binary(&s, tag + 32, 32) == 0 &&
              mbedtls_ecdsa_verify(&_grp, hash, sizeof(hash), &q, &r, &s) == 0;
    mbedtls_ecp_point_free(&q);
    mbedtls_mpi_free(&r);
    mbedtls_mpi_free(&s);
    return ok;
}

}  // namespace security
