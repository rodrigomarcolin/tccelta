#pragma once
#include "connectivity/secure/SecureBleConnectivity.h"

/**
 * SecureHandshakeReplayBleConnectivity — PSK handshake (same as
 * SecureHandshakeBleConnectivity) + a strictly-increasing per-direction
 * counter bound into AES-256-GCM as associated data, so a captured
 * ciphertext frame can never be replayed — not even within the same
 * session.
 *
 * Handshake (identical to SecureHandshakeBleConnectivity):
 *   App    → Dongle:  HELLO  <appNonce:32B hex>
 *   Dongle → App:      CHALLENGE  <dongleNonce:32B hex>
 *   App    → Dongle:  PROOF  <HMAC-SHA256(PSK, "PROOF" ‖ dongleNonce ‖ appNonce):hex>
 *   Dongle → App:      OK  <HMAC-SHA256(PSK, "OK" ‖ appNonce ‖ dongleNonce):hex>
 *   Session key = HKDF-SHA256(PSK, salt = dongleNonce ‖ appNonce, info = "session-key")
 *
 * Established session (differs from SecureHandshakeBleConnectivity):
 *   Each direction keeps its own 64-bit counter starting at 0:
 *     _txCounter — this device's next outbound counter value
 *     _rxCounter — last counter value accepted from the peer
 *
 *   Wire frame:
 *     <16 hex — counter, big-endian> <24 hex — IV> <cipher hex> <32 hex — tag> '\n'
 *
 *   AAD passed to AES-256-GCM = counter (8 bytes, big-endian) ‖ direction byte
 *   (0 = app→dongle, 1 = dongle→app), so tampering with the counter breaks
 *   the authentication tag rather than merely desyncing state, and a frame
 *   replayed across directions (reflection) also fails.
 *
 *   A frame is accepted only if its counter is strictly greater than the
 *   last accepted counter for that direction; `_rxCounter` is advanced only
 *   after successful GCM authentication. BLE GATT delivers writes/notifications
 *   in order on a single link, so this never rejects a legitimately-ordered
 *   frame — see the design notes in README/docs for why a counter was chosen
 *   over a synchronized clock.
 *
 *   The session (key + both counters) is scoped to the live handshake; a new
 *   HELLO or a BLE disconnect invalidates it, so counters never need to
 *   persist across reconnects.
 */

enum class HandshakeReplayState {
    IDLE,            ///< Waiting for HELLO
    CHALLENGE_SENT,  ///< CHALLENGE sent, waiting for PROOF
    ESTABLISHED      ///< Session key derived, counters active, AES-256-GCM active
};

class SecureHandshakeReplayBleConnectivity : public SecureBleConnectivity {
public:
    explicit SecureHandshakeReplayBleConnectivity(IConnectivity* inner);

protected:
    void onRawFrameReceived(const uint8_t* data, size_t len) override;
    void sendSecured(const uint8_t* plain, size_t len) override;

private:
    static constexpr size_t kNonceLen = 32;  ///< bytes
    static constexpr size_t kHmacLen  = 32;  ///< SHA-256 output
    static constexpr size_t kKeyLen   = 32;  ///< AES-256 / HKDF output
    static constexpr size_t kCounterLen = 8; ///< bytes (uint64_t, big-endian)

    static constexpr uint8_t kDirAppToDongle = 0;
    static constexpr uint8_t kDirDongleToApp = 1;

    uint8_t _psk[kKeyLen];  ///< Long-term pre-shared key (decoded from SECURE_PSK_HEX)

    HandshakeReplayState _state = HandshakeReplayState::IDLE;

    uint8_t _appNonce[kNonceLen];
    uint8_t _dongleNonce[kNonceLen];
    uint8_t _sessionKey[kKeyLen];

    uint64_t _txCounter = 0;  ///< next counter value this device will send
    uint64_t _rxCounter = 0;  ///< last counter value accepted from the peer (0 = none yet)
    bool     _rxCounterInitialized = false;

    // ── Handshake sub-steps ──────────────────────────────────────────────────
    void handleHello(const char* body, size_t bodyLen);
    void handlePreProof(const char* body, size_t bodyLen);
    void resetToIdle();

    // ── Crypto helpers ───────────────────────────────────────────────────────
    static void hmacTagged(const uint8_t* key, size_t keyLen,
                            const char* tag,
                            const uint8_t* a, size_t aLen,
                            const uint8_t* b, size_t bLen,
                            uint8_t* out);

    static void deriveSessionKey(const uint8_t* psk, size_t pskLen,
                                  const uint8_t* dongleNonce, const uint8_t* appNonce,
                                  uint8_t* out);

    static bool constantTimeEquals(const uint8_t* a, const uint8_t* b, size_t len);

    static void counterToBytes(uint64_t counter, uint8_t* out8);
    static uint64_t bytesToCounter(const uint8_t* in8);

    // ── Hex codec helpers ─────────────────────────────────────────────────────
    static bool hexDecode(const char* hex, size_t hexLen, uint8_t* out, size_t outLen);
    static void hexEncode(const uint8_t* in, size_t inLen, char* out);
    static int  hexCharToNibble(char c);
};
