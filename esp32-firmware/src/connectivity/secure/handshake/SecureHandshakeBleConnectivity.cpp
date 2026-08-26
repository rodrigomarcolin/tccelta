#include <Arduino.h>
#include "SecureHandshakeBleConnectivity.h"

// ── Constructor ───────────────────────────────────────────────────────────────

SecureHandshakeBleConnectivity::SecureHandshakeBleConnectivity(IConnectivity* inner)
    : SecureBleConnectivity(inner), _state(HandshakeState::IDLE)
{
    Serial.println("[Handshake] Initialised — stub mode (pass-through, no encryption)");
}

// ── onRawFrameReceived ────────────────────────────────────────────────────────

/**
 * TODO: Implement the inbound handshake state machine:
 *
 *   IDLE → parse "HELLO <clientNonce:64hex>"
 *        → reply "CHALLENGE <serverNonce:64hex>"
 *        → _state = CHALLENGE_RECEIVED
 *
 *   CHALLENGE_RECEIVED → parse "PROOF <hmac:64hex>"
 *        → verify HMAC-SHA256(PSK, clientNonce ‖ serverNonce)
 *        → derive session key: HKDF-SHA256(PSK, clientNonce ‖ serverNonce, "session-key")
 *        → reply "OK"
 *        → _state = ESTABLISHED
 *
 *   ESTABLISHED → AES-256-GCM decrypt with _sessionKey + monotonic counter
 *        → on success: deliverPlaintext(plain, plainLen)
 *        → on failure: drop, optionally reset to IDLE
 *
 * Current behaviour (stub): pass frame through as-is with a warning.
 */
void SecureHandshakeBleConnectivity::onRawFrameReceived(const uint8_t* data, size_t len) {
    Serial.println("[Handshake] WARNING: stub — forwarding raw frame without decryption");
    deliverPlaintext(data, len);
}

// ── sendSecured ───────────────────────────────────────────────────────────────

/**
 * TODO: Implement outbound encryption:
 *
 *   ESTABLISHED → AES-256-GCM encrypt plaintext with _sessionKey + counter
 *              → hex-encode IV ‖ ciphertext ‖ tag
 *              → sendRaw(wireFrame, wireLen)
 *
 *   Otherwise   → drop or queue until handshake completes (decide per protocol)
 *
 * Current behaviour (stub): send plaintext as-is with a warning.
 */
void SecureHandshakeBleConnectivity::sendSecured(const uint8_t* plain, size_t len) {
    Serial.println("[Handshake] WARNING: stub — sending plaintext without encryption");
    sendRaw(plain, len);
}
