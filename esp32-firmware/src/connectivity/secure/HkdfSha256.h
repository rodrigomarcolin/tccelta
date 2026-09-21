#pragma once
#include <cstddef>
#include <cstdint>
#include <cstring>
#include <mbedtls/md.h>

// RFC 5869 HKDF-SHA256, standing in for mbedtls_hkdf(). The precompiled
// Arduino-ESP32 mbedtls build ships hkdf.h but not the hkdf.c object
// (MBEDTLS_HKDF_C is off in its sdkconfig), so mbedtls_hkdf() is an
// undefined symbol at link time. HMAC-SHA256 itself is linked, so extract
// and expand are built directly on mbedtls_md's HMAC API.
inline void hkdfSha256(const uint8_t* salt, size_t saltLen,
                        const uint8_t* ikm, size_t ikmLen,
                        const uint8_t* info, size_t infoLen,
                        uint8_t* okm, size_t okmLen) {
    const mbedtls_md_info_t* md = mbedtls_md_info_from_type(MBEDTLS_MD_SHA256);
    mbedtls_md_context_t ctx;
    mbedtls_md_init(&ctx);
    mbedtls_md_setup(&ctx, md, 1 /* HMAC */);

    uint8_t prk[32];
    mbedtls_md_hmac_starts(&ctx, salt, saltLen);
    mbedtls_md_hmac_update(&ctx, ikm, ikmLen);
    mbedtls_md_hmac_finish(&ctx, prk);

    uint8_t t[32];
    size_t tLen = 0;
    for (size_t off = 0, counter = 1; off < okmLen; ++counter) {
        uint8_t c = static_cast<uint8_t>(counter);
        mbedtls_md_hmac_starts(&ctx, prk, sizeof(prk));
        mbedtls_md_hmac_update(&ctx, t, tLen);
        mbedtls_md_hmac_update(&ctx, info, infoLen);
        mbedtls_md_hmac_update(&ctx, &c, 1);
        mbedtls_md_hmac_finish(&ctx, t);
        tLen = sizeof(t);
        size_t chunk = (okmLen - off < tLen) ? (okmLen - off) : tLen;
        memcpy(okm + off, t, chunk);
        off += chunk;
    }
    mbedtls_md_free(&ctx);
}
