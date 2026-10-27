#include "security/mbed/MbedHmacSha256.h"

namespace security {

MbedHmacSha256::MbedHmacSha256(const uint8_t* key, size_t keyLen) {
    mbedtls_md_init(&_ctx);
    mbedtls_md_setup(&_ctx, mbedtls_md_info_from_type(MBEDTLS_MD_SHA256), 1);
    mbedtls_md_hmac_starts(&_ctx, key, keyLen);
}

MbedHmacSha256::~MbedHmacSha256() { mbedtls_md_free(&_ctx); }

void MbedHmacSha256::begin() { mbedtls_md_hmac_reset(&_ctx); }

void MbedHmacSha256::update(const uint8_t* data, size_t len) { mbedtls_md_hmac_update(&_ctx, data, len); }

void MbedHmacSha256::finish(uint8_t out[crypto::kSha256Size]) { mbedtls_md_hmac_finish(&_ctx, out); }

}  // namespace security
