#pragma once
#include "connectivity/secure/SecureBleConnectivity.h"

/**
 * SecureHandshakeBleConnectivity — Stub for a future PSK-based handshake protocol.
 *
 * Planned protocol (NOT YET IMPLEMENTED — all methods are TODO stubs):
 *
 *   Phase 1 — Handshake
 *     Client → Dongle:  HELLO  <clientNonce:32B>
 *     Dongle → Client:  CHALLENGE  <serverNonce:32B>
 *     Client → Dongle:  PROOF  <HMAC-SHA256(PSK, clientNonce ‖ serverNonce)>
 *     Dongle → Client:  OK  (or ERROR)
 *
 *   Phase 2 — Established session
 *     Session key = HKDF-SHA256(PSK, clientNonce ‖ serverNonce, "session-key")
 *     Messages encrypted with AES-256-GCM + replay-protected monotonic counter.
 *
 * Current behaviour:
 *   Pass-through (no encryption, no auth) with Serial warnings so you can see
 *   it is active.  Replace TODO bodies with real logic when ready.
 */

enum class HandshakeState {
    IDLE,               ///< No session — waiting for HELLO
    HELLO_SENT,         ///< (client-side) HELLO sent, waiting for CHALLENGE
    CHALLENGE_RECEIVED, ///< CHALLENGE received, computing PROOF
    ESTABLISHED,        ///< Session key derived, encryption active
    ERROR               ///< Unrecoverable error — session must be reset
};

class SecureHandshakeBleConnectivity : public SecureBleConnectivity {
public:
    explicit SecureHandshakeBleConnectivity(IConnectivity* inner);

protected:
    /**
     * TODO: Implement the inbound state machine:
     *   - In IDLE:               parse HELLO, send CHALLENGE, → CHALLENGE_RECEIVED
     *   - In CHALLENGE_RECEIVED: parse PROOF, derive session key, → ESTABLISHED
     *   - In ESTABLISHED:        AES-256-GCM decrypt with session key + counter,
     *                            call deliverPlaintext() on success
     *
     * Current stub: prints a warning and passes the raw frame through as-is.
     */
    void onRawFrameReceived(const uint8_t* data, size_t len) override;

    /**
     * TODO: Implement outbound encryption:
     *   - In ESTABLISHED: AES-256-GCM encrypt with session key + counter,
     *                     call sendRaw(cipherFrame, cipherLen)
     *   - Otherwise:      drop or queue until handshake completes
     *
     * Current stub: passes plaintext through as-is.
     */
    void sendSecured(const uint8_t* plain, size_t len) override;

private:
    HandshakeState _state = HandshakeState::IDLE;

    // TODO: session state (nonces, derived key, counter)
    // uint8_t  _clientNonce[32];
    // uint8_t  _serverNonce[32];
    // uint8_t  _sessionKey[32];
    // uint32_t _counter = 0;
};
