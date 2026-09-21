#pragma once
#include "connectivity/secure/SecureBleConnectivity.h"

// PSK-authenticated nonce handshake with a fresh per-session AES-256-GCM key.
// This mode intentionally has no message counter; use handshake_replay for it.
enum class HandshakeState { IDLE, CHALLENGE_SENT, ESTABLISHED };

class SecureHandshakeBleConnectivity : public SecureBleConnectivity {
public:
    explicit SecureHandshakeBleConnectivity(IConnectivity* inner);
protected:
    void onRawFrameReceived(const uint8_t* data, size_t len) override;
    void sendSecured(const uint8_t* plain, size_t len) override;
private:
    static constexpr size_t kNonceLen = 32;
    static constexpr size_t kKeyLen = 32;
    uint8_t _psk[kKeyLen] = {}, _appNonce[kNonceLen] = {}, _dongleNonce[kNonceLen] = {}, _sessionKey[kKeyLen] = {};
    HandshakeState _state = HandshakeState::IDLE;
    void handleHello(const char* body, size_t len);
    void handleProof(const char* body, size_t len);
    void resetToIdle();
    static void hmac(const uint8_t* key, size_t keyLen, const uint8_t* data, size_t dataLen, uint8_t* out);
    static bool constantTimeEquals(const uint8_t* a, const uint8_t* b, size_t len);
    static bool hexDecode(const char* hex, size_t hexLen, uint8_t* out, size_t outLen);
    static void hexEncode(const uint8_t* in, size_t len, char* out);
    static int hexNibble(char c);
};
