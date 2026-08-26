#include <Arduino.h>
#include <cstring>
#include <esp_random.h>
#include <mbedtls/gcm.h>
#include "SecurePskBleConnectivity.h"

// ── Compile-time key validation ───────────────────────────────────────────────

#ifndef SECURE_PSK_HEX
#  error "SECURE_PSK_HEX is not set. Define it in your secrets.ini build_flags."
#endif

// Stringify the macro value so we can work with it as a C string at runtime.
#define _PSK_STR_IMPL(x) #x
#define _PSK_STR(x)      _PSK_STR_IMPL(x)
static const char* kPskHex = _PSK_STR(SECURE_PSK_HEX);

// ── Constants ─────────────────────────────────────────────────────────────────

static constexpr size_t kIvLen  = 12;  ///< GCM standard IV length
static constexpr size_t kTagLen = 16;  ///< AES-GCM authentication tag length

// Wire format lengths (hex-encoded)
static constexpr size_t kIvHexLen  = kIvLen  * 2;   // 24
static constexpr size_t kTagHexLen = kTagLen * 2;    // 32
static constexpr size_t kMinWireLen = kIvHexLen + kTagHexLen; // 56 hex chars

// ── Constructor ───────────────────────────────────────────────────────────────

SecurePskBleConnectivity::SecurePskBleConnectivity(IConnectivity* inner)
    : SecureBleConnectivity(inner)
{
    size_t hexLen = strlen(kPskHex);
    // Strip optional surrounding quotes that the preprocessor might add.
    const char* hex = kPskHex;
    if (hexLen >= 2 && hex[0] == '"') { hex++; hexLen -= 2; }

    configASSERT(hexLen == 64 &&
                 "SECURE_PSK_HEX must be exactly 64 hex characters (32 bytes / 256-bit key)");

    bool ok = hexDecode(hex, hexLen, _key, sizeof(_key));
    configASSERT(ok && "SECURE_PSK_HEX contains invalid hex characters");
}

// ── onRawFrameReceived ────────────────────────────────────────────────────────

/**
 * Wire format (ASCII hex, optional trailing '\n' stripped):
 *   [0  .. 23]   24 hex chars → 12-byte IV
 *   [24 .. N-32] variable    → ciphertext (may be 0 bytes for empty plaintext)
 *   [N-32 .. N]  32 hex chars → 16-byte GCM tag
 *
 * On success: calls deliverPlaintext(plain, plainLen).
 * On any error: logs warning, drops frame.
 */
void SecurePskBleConnectivity::onRawFrameReceived(const uint8_t* data, size_t len) {

    // strip trailing newline or CRLF
    while (len > 0 &&
           (data[len - 1] == '\r' || data[len - 1] == '\n')) {
        --len;
    }

    Serial.printf("[PSK] RX len=%u\n", (unsigned)len);

    Serial.print("[PSK] RX bytes: ");
    for (size_t i = 0; i < len; ++i) {
        Serial.printf("%02X ", data[i]);
    }
    Serial.println();

    Serial.print("[PSK] RX string: '");
    for (size_t i = 0; i < len; ++i) {
        char c = static_cast<char>(data[i]);

        if (c >= 32 && c <= 126)
            Serial.print(c);
        else
            Serial.printf("\\x%02X", data[i]);
    }
    Serial.println("'");

    if (len < kMinWireLen) {
        Serial.printf("[PSK] Frame too short (%u < %u) — dropped\n",
                      (unsigned)len, (unsigned)kMinWireLen);
        return;
    }
    if (len % 2 != 0) {
        Serial.println("[PSK] Odd hex length — dropped");
        return;
    }

    const char* hex = reinterpret_cast<const char*>(data);

    // ── Decode IV ──────────────────────────────────────────────────────────
    uint8_t iv[kIvLen];
    if (!hexDecode(hex, kIvHexLen, iv, kIvLen)) {
        Serial.println("[PSK] Bad hex in IV — dropped");
        return;
    }

    // ── Decode tag ─────────────────────────────────────────────────────────
    uint8_t tag[kTagLen];
    if (!hexDecode(hex + (len - kTagHexLen), kTagHexLen, tag, kTagLen)) {
        Serial.println("[PSK] Bad hex in tag — dropped");
        return;
    }

    // ── Decode ciphertext ──────────────────────────────────────────────────
    size_t cipherHexLen  = len - kIvHexLen - kTagHexLen;
    size_t cipherLen     = cipherHexLen / 2;
    uint8_t cipherBuf[kSecureMaxMsgLen];
    if (cipherLen > sizeof(cipherBuf)) {
        Serial.println("[PSK] Ciphertext too large — dropped");
        return;
    }
    if (cipherLen > 0 && !hexDecode(hex + kIvHexLen, cipherHexLen, cipherBuf, cipherLen)) {
        Serial.println("[PSK] Bad hex in ciphertext — dropped");
        return;
    }

    // ── AES-256-GCM decrypt ────────────────────────────────────────────────
    uint8_t plainBuf[kSecureMaxMsgLen];
    mbedtls_gcm_context ctx;
    mbedtls_gcm_init(&ctx);

    int ret = mbedtls_gcm_setkey(&ctx, MBEDTLS_CIPHER_ID_AES, _key, 256);
    if (ret != 0) {
        Serial.printf("[PSK] GCM setkey failed: -0x%04X\n", (unsigned)(-ret));
        mbedtls_gcm_free(&ctx);
        return;
    }

    ret = mbedtls_gcm_auth_decrypt(
        &ctx,
        cipherLen,
        iv,   kIvLen,
        nullptr, 0,      // no additional authenticated data
        tag,  kTagLen,
        cipherBuf, plainBuf
    );
    mbedtls_gcm_free(&ctx);

    if (ret != 0) {
        // MBEDTLS_ERR_GCM_AUTH_FAILED = -0x0012 — authentication tag mismatch
        Serial.printf("[PSK] GCM auth failed (0x%04X) — dropped\n", (unsigned)(-ret));
        return;
    }

    deliverPlaintext(plainBuf, cipherLen);
}

// ── sendSecured ───────────────────────────────────────────────────────────────

/**
 * Generates a fresh 12-byte random IV, encrypts with AES-256-GCM, then
 * hex-encodes IV ‖ ciphertext ‖ tag and calls sendRaw() with a '\n' appended.
 */
void SecurePskBleConnectivity::sendSecured(const uint8_t* plain, size_t len) {
    if (len > kSecureMaxMsgLen) {
        Serial.println("[PSK] sendSecured: plaintext too large — dropped");
        return;
    }

    // ── Generate IV ────────────────────────────────────────────────────────
    uint8_t iv[kIvLen];
    esp_fill_random(iv, kIvLen);

    // ── AES-256-GCM encrypt ────────────────────────────────────────────────
    uint8_t cipherBuf[kSecureMaxMsgLen];
    uint8_t tag[kTagLen];

    mbedtls_gcm_context ctx;
    mbedtls_gcm_init(&ctx);

    int ret = mbedtls_gcm_setkey(&ctx, MBEDTLS_CIPHER_ID_AES, _key, 256);
    if (ret != 0) {
        Serial.printf("[PSK] GCM setkey failed: -0x%04X\n", (unsigned)(-ret));
        mbedtls_gcm_free(&ctx);
        return;
    }

    ret = mbedtls_gcm_crypt_and_tag(
        &ctx,
        MBEDTLS_GCM_ENCRYPT,
        len,
        iv,      kIvLen,
        nullptr, 0,          // no additional authenticated data
        plain,   cipherBuf,
        kTagLen, tag
    );
    mbedtls_gcm_free(&ctx);

    if (ret != 0) {
        Serial.printf("[PSK] GCM encrypt failed: -0x%04X\n", (unsigned)(-ret));
        return;
    }

    // ── Hex-encode IV ‖ ciphertext ‖ tag ──────────────────────────────────
    // Buffer: 24 + len*2 + 32 + 1 ('\n') + 1 ('\0')
    size_t outBufSize = kIvHexLen + len * 2 + kTagHexLen + 2;
    char   outBuf[kIvHexLen + kSecureMaxMsgLen * 2 + kTagHexLen + 2];

    hexEncode(iv,        kIvLen,  outBuf);
    hexEncode(cipherBuf, len,     outBuf + kIvHexLen);
    hexEncode(tag,       kTagLen, outBuf + kIvHexLen + len * 2);

    size_t wireLen = kIvHexLen + len * 2 + kTagHexLen;
    outBuf[wireLen]     = '\n';
    outBuf[wireLen + 1] = '\0';

    sendRaw(reinterpret_cast<const uint8_t*>(outBuf), wireLen + 1);
}

// ── Hex helpers ───────────────────────────────────────────────────────────────

int SecurePskBleConnectivity::hexCharToNibble(char c) {
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return 10 + c - 'a';
    if (c >= 'A' && c <= 'F') return 10 + c - 'A';
    return -1;
}

bool SecurePskBleConnectivity::hexDecode(const char* hex, size_t hexLen,
                                          uint8_t* out, size_t outLen)
{
    if (hexLen != outLen * 2) return false;
    for (size_t i = 0; i < outLen; ++i) {
        int hi = hexCharToNibble(hex[i * 2]);
        int lo = hexCharToNibble(hex[i * 2 + 1]);
        if (hi < 0 || lo < 0) return false;
        out[i] = static_cast<uint8_t>((hi << 4) | lo);
    }
    return true;
}

void SecurePskBleConnectivity::hexEncode(const uint8_t* in, size_t inLen, char* out) {
    static const char kHex[] = "0123456789abcdef";
    for (size_t i = 0; i < inLen; ++i) {
        out[i * 2]     = kHex[in[i] >> 4];
        out[i * 2 + 1] = kHex[in[i] & 0x0F];
    }
    out[inLen * 2] = '\0';
}
