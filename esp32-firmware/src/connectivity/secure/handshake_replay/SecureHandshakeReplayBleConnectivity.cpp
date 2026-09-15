#include <Arduino.h>
#include <cstring>
#include <esp_random.h>
#include <mbedtls/gcm.h>
#include <mbedtls/md.h>
#include <mbedtls/hkdf.h>
#include "SecureHandshakeReplayBleConnectivity.h"

// ── Compile-time key validation (shares SECURE_PSK_HEX with the other builds) ─

#ifndef SECURE_PSK_HEX
#  error "SECURE_PSK_HEX is not set. Define it in your secrets.ini build_flags."
#endif

#define _PSK_STR_IMPL(x) #x
#define _PSK_STR(x)      _PSK_STR_IMPL(x)
static const char* kPskHex = _PSK_STR(SECURE_PSK_HEX);

// ── Wire constants ──────────────────────────────────────────────────────────

static constexpr size_t kIvLen  = 12;  ///< GCM standard IV length
static constexpr size_t kTagLen = 16;  ///< AES-GCM authentication tag length
static constexpr size_t kCounterHexLen = 16;  // 8 bytes
static constexpr size_t kIvHexLen  = kIvLen  * 2;
static constexpr size_t kTagHexLen = kTagLen * 2;
static constexpr size_t kMinWireLen = kCounterHexLen + kIvHexLen + kTagHexLen;

// ── Constructor ─────────────────────────────────────────────────────────────

SecureHandshakeReplayBleConnectivity::SecureHandshakeReplayBleConnectivity(IConnectivity* inner)
    : SecureBleConnectivity(inner), _state(HandshakeReplayState::IDLE)
{
    size_t hexLen = strlen(kPskHex);
    const char* hex = kPskHex;
    if (hexLen >= 2 && hex[0] == '"') { hex++; hexLen -= 2; }

    configASSERT(hexLen == 64 &&
                 "SECURE_PSK_HEX must be exactly 64 hex characters (32 bytes / 256-bit key)");

    bool ok = hexDecode(hex, hexLen, _psk, sizeof(_psk));
    configASSERT(ok && "SECURE_PSK_HEX contains invalid hex characters");

    Serial.println("[HandshakeReplay] Initialised — waiting for HELLO");
}

// ── onRawFrameReceived ───────────────────────────────────────────────────────

void SecureHandshakeReplayBleConnectivity::onRawFrameReceived(const uint8_t* data, size_t len) {
    while (len > 0 && (data[len - 1] == '\r' || data[len - 1] == '\n')) {
        --len;
    }

    const char* text = reinterpret_cast<const char*>(data);

    if (_state == HandshakeReplayState::ESTABLISHED) {
        // ── Encrypted data frame ────────────────────────────────────────────
        if (len < kMinWireLen || len % 2 != 0) {
            Serial.println("[HandshakeReplay] Malformed data frame — dropped");
            return;
        }

        uint8_t counterBytes[kCounterLen];
        if (!hexDecode(text, kCounterHexLen, counterBytes, kCounterLen)) {
            Serial.println("[HandshakeReplay] Bad hex in counter — dropped");
            return;
        }
        uint64_t counter = bytesToCounter(counterBytes);

        // Strict monotonicity: reject anything not newer than the last
        // accepted frame from the app. Checked BEFORE spending a GCM
        // verification on it, but the counter is not yet "consumed"
        // (rxCounter advanced) until authentication actually succeeds.
        if (_rxCounterInitialized && counter <= _rxCounter) {
            Serial.printf("[HandshakeReplay] Replay/out-of-order counter %llu <= %llu — dropped\n",
                          (unsigned long long)counter, (unsigned long long)_rxCounter);
            return;
        }

        const char* rest = text + kCounterHexLen;
        size_t restLen   = len - kCounterHexLen;

        uint8_t iv[kIvLen];
        uint8_t tag[kTagLen];
        if (!hexDecode(rest, kIvHexLen, iv, kIvLen) ||
            !hexDecode(rest + (restLen - kTagHexLen), kTagHexLen, tag, kTagLen)) {
            Serial.println("[HandshakeReplay] Bad hex in IV/tag — dropped");
            return;
        }

        size_t cipherHexLen = restLen - kIvHexLen - kTagHexLen;
        size_t cipherLen    = cipherHexLen / 2;
        uint8_t cipherBuf[kSecureMaxMsgLen];
        if (cipherLen > sizeof(cipherBuf)) {
            Serial.println("[HandshakeReplay] Ciphertext too large — dropped");
            return;
        }
        if (cipherLen > 0 && !hexDecode(rest + kIvHexLen, cipherHexLen, cipherBuf, cipherLen)) {
            Serial.println("[HandshakeReplay] Bad hex in ciphertext — dropped");
            return;
        }

        // AAD = counter (8 bytes) ‖ direction byte (app → dongle)
        uint8_t aad[kCounterLen + 1];
        memcpy(aad, counterBytes, kCounterLen);
        aad[kCounterLen] = kDirAppToDongle;

        uint8_t plainBuf[kSecureMaxMsgLen];
        mbedtls_gcm_context ctx;
        mbedtls_gcm_init(&ctx);
        int ret = mbedtls_gcm_setkey(&ctx, MBEDTLS_CIPHER_ID_AES, _sessionKey, 256);
        if (ret == 0) {
            ret = mbedtls_gcm_auth_decrypt(&ctx, cipherLen, iv, kIvLen, aad, sizeof(aad),
                                            tag, kTagLen, cipherBuf, plainBuf);
        }
        mbedtls_gcm_free(&ctx);

        if (ret != 0) {
            Serial.printf("[HandshakeReplay] GCM auth failed (0x%04X) — dropped\n", (unsigned)(-ret));
            return;
        }

        _rxCounter = counter;
        _rxCounterInitialized = true;

        deliverPlaintext(plainBuf, cipherLen);
        return;
    }

    // ── Handshake control frames ────────────────────────────────────────────
    if (len >= 6 && strncmp(text, "HELLO ", 6) == 0) {
        handleHello(text + 6, len - 6);
        return;
    }
    if (len >= 6 && strncmp(text, "PROOF ", 6) == 0) {
        handlePreProof(text + 6, len - 6);
        return;
    }

    Serial.printf("[HandshakeReplay] Unexpected frame in state=%d — ignored\n", (int)_state);
}

void SecureHandshakeReplayBleConnectivity::handleHello(const char* body, size_t bodyLen) {
    // Accept HELLO in any state — a retried/late HELLO restarts the handshake
    // and invalidates any half-finished or established session (including
    // its counters, so a stale session can never be revived by reusing them).
    if (bodyLen != kNonceLen * 2 || !hexDecode(body, bodyLen, _appNonce, kNonceLen)) {
        Serial.println("[HandshakeReplay] Malformed HELLO — ignored");
        return;
    }

    resetToIdle();

    esp_fill_random(_dongleNonce, kNonceLen);

    char hex[kNonceLen * 2 + 1];
    hexEncode(_dongleNonce, kNonceLen, hex);

    char frame[8 + kNonceLen * 2 + 2];
    int n = snprintf(frame, sizeof(frame), "CHALLENGE %s\n", hex);
    sendRaw(reinterpret_cast<const uint8_t*>(frame), n);

    _state = HandshakeReplayState::CHALLENGE_SENT;
    Serial.println("[HandshakeReplay] HELLO received — CHALLENGE sent");
}

void SecureHandshakeReplayBleConnectivity::handlePreProof(const char* body, size_t bodyLen) {
    if (_state != HandshakeReplayState::CHALLENGE_SENT) {
        Serial.println("[HandshakeReplay] PROOF received out of order — ignored");
        return;
    }

    uint8_t receivedProof[kHmacLen];
    if (bodyLen != kHmacLen * 2 || !hexDecode(body, bodyLen, receivedProof, kHmacLen)) {
        Serial.println("[HandshakeReplay] Malformed PROOF — resetting");
        resetToIdle();
        return;
    }

    uint8_t expectedProof[kHmacLen];
    hmacTagged(_psk, sizeof(_psk), "PROOF",
               _dongleNonce, kNonceLen, _appNonce, kNonceLen,
               expectedProof);

    if (!constantTimeEquals(receivedProof, expectedProof, kHmacLen)) {
        Serial.println("[HandshakeReplay] PROOF verification failed — ERROR sent");
        sendRaw(reinterpret_cast<const uint8_t*>("ERROR\n"), 6);
        resetToIdle();
        return;
    }

    deriveSessionKey(_psk, sizeof(_psk), _dongleNonce, _appNonce, _sessionKey);
    _txCounter = 0;
    _rxCounter = 0;
    _rxCounterInitialized = false;

    uint8_t okHmac[kHmacLen];
    hmacTagged(_psk, sizeof(_psk), "OK",
               _appNonce, kNonceLen, _dongleNonce, kNonceLen,
               okHmac);

    char hex[kHmacLen * 2 + 1];
    hexEncode(okHmac, kHmacLen, hex);

    char frame[4 + kHmacLen * 2 + 2];
    int n = snprintf(frame, sizeof(frame), "OK %s\n", hex);
    sendRaw(reinterpret_cast<const uint8_t*>(frame), n);

    _state = HandshakeReplayState::ESTABLISHED;
    Serial.println("[HandshakeReplay] PROOF verified — session ESTABLISHED");
}

void SecureHandshakeReplayBleConnectivity::resetToIdle() {
    _state = HandshakeReplayState::IDLE;
    memset(_appNonce, 0, sizeof(_appNonce));
    memset(_dongleNonce, 0, sizeof(_dongleNonce));
    memset(_sessionKey, 0, sizeof(_sessionKey));
    _txCounter = 0;
    _rxCounter = 0;
    _rxCounterInitialized = false;
}

// ── sendSecured ──────────────────────────────────────────────────────────────

void SecureHandshakeReplayBleConnectivity::sendSecured(const uint8_t* plain, size_t len) {
    if (_state != HandshakeReplayState::ESTABLISHED) {
        Serial.println("[HandshakeReplay] sendSecured: no established session — dropped");
        return;
    }
    if (len > kSecureMaxMsgLen) {
        Serial.println("[HandshakeReplay] sendSecured: plaintext too large — dropped");
        return;
    }
    if (_txCounter == UINT64_MAX) {
        // Counter space exhausted — refuse to reuse a (key, counter) pair.
        // A real deployment should force re-handshake well before this point.
        Serial.println("[HandshakeReplay] sendSecured: counter exhausted — session must be re-established");
        return;
    }

    uint64_t counter = _txCounter++;
    uint8_t counterBytes[kCounterLen];
    counterToBytes(counter, counterBytes);

    uint8_t iv[kIvLen];
    esp_fill_random(iv, kIvLen);

    uint8_t aad[kCounterLen + 1];
    memcpy(aad, counterBytes, kCounterLen);
    aad[kCounterLen] = kDirDongleToApp;

    uint8_t cipherBuf[kSecureMaxMsgLen];
    uint8_t tag[kTagLen];

    mbedtls_gcm_context ctx;
    mbedtls_gcm_init(&ctx);
    int ret = mbedtls_gcm_setkey(&ctx, MBEDTLS_CIPHER_ID_AES, _sessionKey, 256);
    if (ret == 0) {
        ret = mbedtls_gcm_crypt_and_tag(&ctx, MBEDTLS_GCM_ENCRYPT, len, iv, kIvLen,
                                         aad, sizeof(aad), plain, cipherBuf, kTagLen, tag);
    }
    mbedtls_gcm_free(&ctx);

    if (ret != 0) {
        Serial.printf("[HandshakeReplay] GCM encrypt failed: -0x%04X\n", (unsigned)(-ret));
        return;
    }

    char counterHex[kCounterHexLen + 1];
    hexEncode(counterBytes, kCounterLen, counterHex);

    char outBuf[kCounterHexLen + kIvHexLen + kSecureMaxMsgLen * 2 + kTagHexLen + 2];
    memcpy(outBuf, counterHex, kCounterHexLen);
    hexEncode(iv, kIvLen, outBuf + kCounterHexLen);
    hexEncode(cipherBuf, len, outBuf + kCounterHexLen + kIvHexLen);
    hexEncode(tag, kTagLen, outBuf + kCounterHexLen + kIvHexLen + len * 2);

    size_t wireLen = kCounterHexLen + kIvHexLen + len * 2 + kTagHexLen;
    outBuf[wireLen]     = '\n';
    outBuf[wireLen + 1] = '\0';

    sendRaw(reinterpret_cast<const uint8_t*>(outBuf), wireLen + 1);
}

// ── Crypto helpers ───────────────────────────────────────────────────────────

void SecureHandshakeReplayBleConnectivity::hmacTagged(const uint8_t* key, size_t keyLen,
                                                        const char* tag,
                                                        const uint8_t* a, size_t aLen,
                                                        const uint8_t* b, size_t bLen,
                                                        uint8_t* out)
{
    const mbedtls_md_info_t* mdInfo = mbedtls_md_info_from_type(MBEDTLS_MD_SHA256);
    mbedtls_md_context_t ctx;
    mbedtls_md_init(&ctx);
    mbedtls_md_setup(&ctx, mdInfo, 1 /* HMAC */);
    mbedtls_md_hmac_starts(&ctx, key, keyLen);
    mbedtls_md_hmac_update(&ctx, reinterpret_cast<const uint8_t*>(tag), strlen(tag));
    mbedtls_md_hmac_update(&ctx, a, aLen);
    mbedtls_md_hmac_update(&ctx, b, bLen);
    mbedtls_md_hmac_finish(&ctx, out);
    mbedtls_md_free(&ctx);
}

void SecureHandshakeReplayBleConnectivity::deriveSessionKey(const uint8_t* psk, size_t pskLen,
                                                              const uint8_t* dongleNonce,
                                                              const uint8_t* appNonce,
                                                              uint8_t* out)
{
    uint8_t salt[kNonceLen * 2];
    memcpy(salt, dongleNonce, kNonceLen);
    memcpy(salt + kNonceLen, appNonce, kNonceLen);

    static const char kInfo[] = "session-key";
    const mbedtls_md_info_t* mdInfo = mbedtls_md_info_from_type(MBEDTLS_MD_SHA256);

    mbedtls_hkdf(mdInfo, salt, sizeof(salt), psk, pskLen,
                 reinterpret_cast<const uint8_t*>(kInfo), strlen(kInfo),
                 out, kKeyLen);
}

bool SecureHandshakeReplayBleConnectivity::constantTimeEquals(const uint8_t* a, const uint8_t* b, size_t len) {
    uint8_t diff = 0;
    for (size_t i = 0; i < len; ++i) {
        diff |= a[i] ^ b[i];
    }
    return diff == 0;
}

void SecureHandshakeReplayBleConnectivity::counterToBytes(uint64_t counter, uint8_t* out8) {
    for (int i = 0; i < 8; ++i) {
        out8[i] = static_cast<uint8_t>(counter >> (8 * (7 - i)));
    }
}

uint64_t SecureHandshakeReplayBleConnectivity::bytesToCounter(const uint8_t* in8) {
    uint64_t counter = 0;
    for (int i = 0; i < 8; ++i) {
        counter = (counter << 8) | in8[i];
    }
    return counter;
}

// ── Hex helpers ──────────────────────────────────────────────────────────────

int SecureHandshakeReplayBleConnectivity::hexCharToNibble(char c) {
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'a' && c <= 'f') return 10 + c - 'a';
    if (c >= 'A' && c <= 'F') return 10 + c - 'A';
    return -1;
}

bool SecureHandshakeReplayBleConnectivity::hexDecode(const char* hex, size_t hexLen,
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

void SecureHandshakeReplayBleConnectivity::hexEncode(const uint8_t* in, size_t inLen, char* out) {
    static const char kHex[] = "0123456789abcdef";
    for (size_t i = 0; i < inLen; ++i) {
        out[i * 2]     = kHex[in[i] >> 4];
        out[i * 2 + 1] = kHex[in[i] & 0x0F];
    }
    out[inLen * 2] = '\0';
}
