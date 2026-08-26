#pragma once
#include "connectivity/secure/SecureBleConnectivity.h"

/**
 * SecurePskBleConnectivity — AES-256-GCM with a static pre-shared key.
 *
 * Security model (intentionally stripped-down for demo / first iteration):
 *   - Key:  32 raw bytes decoded at construction from the build-time macro
 *           SECURE_PSK_HEX (64 hex characters), defined in platformio.ini or
 *           an environment-local secrets.ini that you MUST NOT commit to VCS.
 *   - IV:   12 bytes of CSPRNG randomness (esp_fill_random) per message.
 *   - Tag:  16-byte GCM authentication tag (AES-GCM standard).
 *
 * Wire format (ASCII hex, '\n'-terminated — matches what NimBLE sends as a
 * UTF-8 characteristic value):
 *
 *   INBOUND  (app → dongle):
 *     <24 hex chars — IV> <variable hex chars — ciphertext> <32 hex chars — tag> '\n'
 *
 *   OUTBOUND (dongle → app):
 *     <24 hex chars — IV> <variable hex chars — ciphertext> <32 hex chars — tag> '\n'
 *
 *   Total minimum length: 24 + 0 + 32 = 56 hex chars (empty plaintext forbidden
 *   in practice, but handled gracefully).
 *
 * No handshake. No session-key derivation. Replay protection is left to the
 * application layer in this version.
 *
 * Crypto library: mbedTLS (bundled with ESP-IDF / Arduino-ESP32 — no extra
 * lib_deps required).
 */
class SecurePskBleConnectivity : public SecureBleConnectivity {
public:
    /**
     * @param inner  The plain BleConnectivity instance to wrap.
     *
     * Decodes SECURE_PSK_HEX at construction time.
     * Panics (configASSERT) if the macro is missing, not 64 hex chars, or
     * contains non-hex characters.
     */
    explicit SecurePskBleConnectivity(IConnectivity* inner);

protected:
    /**
     * Decrypts an AES-256-GCM frame arriving from the wire.
     *
     * Expected wire layout (hex-encoded, optional trailing '\n' stripped):
     *   bytes [ 0.. 23] → IV  (12 raw bytes)
     *   bytes [24..end-32] → ciphertext
     *   bytes [end-32..end] → GCM tag (16 raw bytes)
     *
     * On authentication success: calls deliverPlaintext(plain, plainLen).
     * On any error (too short, bad hex, GCM tag mismatch): frame is silently
     * dropped and a Serial warning is printed.
     */
    void onRawFrameReceived(const uint8_t* data, size_t len) override;

    /**
     * Encrypts plaintext with AES-256-GCM and pushes the hex-encoded frame to
     * the inner transport via sendRaw().
     *
     * A fresh 12-byte IV is generated for every call (esp_fill_random).
     */
    void sendSecured(const uint8_t* plain, size_t len) override;

private:
    uint8_t _key[32];  ///< 256-bit AES key decoded from SECURE_PSK_HEX

    // ── Hex codec helpers ────────────────────────────────────────────────────
    static bool     hexDecode(const char* hex, size_t hexLen,
                               uint8_t* out, size_t outLen);
    static void     hexEncode(const uint8_t* in, size_t inLen,
                               char* out);          // out must hold inLen*2+1 bytes
    static int      hexCharToNibble(char c);
};
