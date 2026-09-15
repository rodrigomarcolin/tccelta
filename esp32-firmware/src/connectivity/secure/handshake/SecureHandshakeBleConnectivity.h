#pragma once
#include "connectivity/secure/SecureBleConnectivity.h"

/**
 * SecureHandshakeBleConnectivity — PSK-authenticated handshake + session key.
 *
 * Protocol (dongle side):
 *
 *   Phase 1 — Handshake (plaintext control frames, ASCII, '\n'-terminated)
 *     App    → Dongle:  HELLO  <appNonce:32B hex>
 *     Dongle → App:      CHALLENGE  <dongleNonce:32B hex>
 *     App    → Dongle:  PROOF  <HMAC-SHA256(PSK, "PROOF" ‖ dongleNonce ‖ appNonce):hex>
 *     Dongle → App:      OK  <HMAC-SHA256(PSK, "OK" ‖ appNonce ‖ dongleNonce):hex>
 *                        (or ERROR on verification failure)
 *
 *   Explicit ASCII tags ("PROOF"/"OK") are mixed into each HMAC input so the
 *   two proofs can never be reflected as one another even though they cover
 *   the same nonce pair in swapped order.
 *
 *   Phase 2 — Established session
 *     Session key = HKDF-SHA256(PSK, salt = dongleNonce ‖ appNonce, info = "session-key")
 *     Messages are AES-256-GCM encrypted with the session key, one fresh
 *     random IV per message (same wire format as SecurePskBleConnectivity).
 *
 * This class intentionally does NOT add replay protection beyond what GCM's
 * authentication tag already gives you (integrity, not freshness) — see the
 * project plan for a follow-up implementation that adds a replay-safe
 * transport on top of this same handshake.
 */

enum class HandshakeState {
    IDLE,            ///< Waiting for HELLO
    CHALLENGE_SENT,  ///< CHALLENGE sent, waiting for PROOF
    ESTABLISHED      ///< Session key derived, AES-256-GCM active
};

class SecureHandshakeBleConnectivity : public SecureBleConnectivity {
public:
    explicit SecureHandshakeBleConnectivity(IConnectivity* inner);

protected:
    /**
     * Inbound state machine:
     *   IDLE            → parse HELLO,  send CHALLENGE  → CHALLENGE_SENT
     *   CHALLENGE_SENT  → parse PROOF,  verify, derive session key,
     *                     send OK/ERROR → ESTABLISHED / IDLE
     *   ESTABLISHED     → AES-256-GCM decrypt with session key,
     *                     deliverPlaintext() on success
     */
    void onRawFrameReceived(const uint8_t* data, size_t len) override;

    /**
     * ESTABLISHED → AES-256-GCM encrypt with session key, sendRaw().
     * Otherwise   → dropped (no session yet to encrypt under).
     */
    void sendSecured(const uint8_t* plain, size_t len) override;

private:
    static constexpr size_t kNonceLen = 32;  ///< bytes
    static constexpr size_t kHmacLen  = 32;  ///< SHA-256 output
    static constexpr size_t kKeyLen   = 32;  ///< AES-256 / HKDF output

    uint8_t _psk[kKeyLen];  ///< Long-term pre-shared key (decoded from SECURE_PSK_HEX)

    HandshakeState _state = HandshakeState::IDLE;

    uint8_t _appNonce[kNonceLen];
    uint8_t _dongleNonce[kNonceLen];
    uint8_t _sessionKey[kKeyLen];

    // ── Handshake sub-steps ──────────────────────────────────────────────────
    void handleHello(const char* body, size_t bodyLen);
    void handlePreProof(const char* body, size_t bodyLen);
    void resetToIdle();

    // ── Crypto helpers ───────────────────────────────────────────────────────
    /** HMAC-SHA256(psk, tag ‖ a ‖ b) → out[kHmacLen] */
    static void hmacTagged(const uint8_t* key, size_t keyLen,
                            const char* tag,
                            const uint8_t* a, size_t aLen,
                            const uint8_t* b, size_t bLen,
                            uint8_t* out);

    /** HKDF-SHA256(psk, salt = dongleNonce‖appNonce, info) → out[kKeyLen] */
    static void deriveSessionKey(const uint8_t* psk, size_t pskLen,
                                  const uint8_t* dongleNonce, const uint8_t* appNonce,
                                  uint8_t* out);

    static bool constantTimeEquals(const uint8_t* a, const uint8_t* b, size_t len);

    // ── Hex codec helpers (same layout as SecurePskBleConnectivity) ─────────
    static bool hexDecode(const char* hex, size_t hexLen, uint8_t* out, size_t outLen);
    static void hexEncode(const uint8_t* in, size_t inLen, char* out);
    static int  hexCharToNibble(char c);
};
