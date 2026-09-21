#include <Arduino.h>
#include <cstring>
#include <esp_random.h>
#include <mbedtls/gcm.h>
#include <mbedtls/md.h>
#include "SecureHandshakeBleConnectivity.h"
#include "connectivity/secure/HkdfSha256.h"

#ifndef SECURE_PSK_HEX
#error "SECURE_PSK_HEX is not set"
#endif
#define PSK_STR_I(x) #x
#define PSK_STR(x) PSK_STR_I(x)
static const char* kPskHex = PSK_STR(SECURE_PSK_HEX);
static constexpr size_t IV_LEN = 12, TAG_LEN = 16;

SecureHandshakeBleConnectivity::SecureHandshakeBleConnectivity(IConnectivity* inner) : SecureBleConnectivity(inner) {
    size_t n = strlen(kPskHex); const char* p = kPskHex;
    if (n >= 2 && p[0] == '"') { ++p; n -= 2; }
    configASSERT(n == 64); configASSERT(hexDecode(p, n, _psk, sizeof(_psk)));
    Serial.println("[Handshake] Initialized — waiting for HELLO");
}

void SecureHandshakeBleConnectivity::onRawFrameReceived(const uint8_t* data, size_t len) {
    while (len && (data[len-1] == '\r' || data[len-1] == '\n')) --len;
    const char* text = reinterpret_cast<const char*>(data);
    Serial.printf("[Handshake] RX frame len=%u state=%d\n", (unsigned)len, (int)_state);
    if (len >= 6 && strncmp(text, "HELLO ", 6) == 0) { handleHello(text+6, len-6); return; }
    if (len >= 6 && strncmp(text, "PROOF ", 6) == 0) { handleProof(text+6, len-6); return; }
    if (_state != HandshakeState::ESTABLISHED || len < 56 || (len & 1)) {
        Serial.println("[Handshake] RX frame not HELLO/PROOF and not a valid secured frame — dropped");
        return;
    }
    uint8_t iv[IV_LEN], tag[TAG_LEN], cipher[kSecureMaxMsgLen], plain[kSecureMaxMsgLen];
    size_t cipherHex = len - 24 - 32, cipherLen = cipherHex / 2;
    if (cipherLen > sizeof(cipher) || !hexDecode(text, 24, iv, IV_LEN) ||
        !hexDecode(text + len - 32, 32, tag, TAG_LEN) ||
        (cipherLen && !hexDecode(text + 24, cipherHex, cipher, cipherLen))) return;
    mbedtls_gcm_context ctx; mbedtls_gcm_init(&ctx);
    int ret = mbedtls_gcm_setkey(&ctx, MBEDTLS_CIPHER_ID_AES, _sessionKey, 256);
    if (!ret) ret = mbedtls_gcm_auth_decrypt(&ctx, cipherLen, iv, IV_LEN, nullptr, 0, tag, TAG_LEN, cipher, plain);
    mbedtls_gcm_free(&ctx);
    if (!ret) deliverPlaintext(plain, cipherLen);
}

void SecureHandshakeBleConnectivity::handleHello(const char* body, size_t len) {
    Serial.printf("[Handshake] HELLO received, body len=%u (expected 64)\n", (unsigned)len);
    if (len != 64) { Serial.println("[Handshake] HELLO rejected — wrong nonce length (frame likely truncated, check MTU)"); return; }
    resetToIdle();
    if (!hexDecode(body, len, _appNonce, sizeof(_appNonce))) { Serial.println("[Handshake] HELLO rejected — invalid hex in nonce"); return; }
    esp_fill_random(_dongleNonce, sizeof(_dongleNonce));
    char nonce[65], frame[80]; hexEncode(_dongleNonce, 32, nonce);
    int n = snprintf(frame, sizeof(frame), "CHALLENGE %s\n", nonce);
    sendRaw(reinterpret_cast<const uint8_t*>(frame), n); _state = HandshakeState::CHALLENGE_SENT;
    Serial.println("[Handshake] CHALLENGE sent");
}

void SecureHandshakeBleConnectivity::handleProof(const char* body, size_t len) {
    Serial.printf("[Handshake] PROOF received, body len=%u, state=%d (expect CHALLENGE_SENT=%d)\n",
                  (unsigned)len, (int)_state, (int)HandshakeState::CHALLENGE_SENT);
    if (_state != HandshakeState::CHALLENGE_SENT || len != 64) { Serial.println("[Handshake] PROOF rejected — wrong state or length"); return; }
    uint8_t got[32], expected[32], salt[64];
    if (!hexDecode(body, len, got, 32)) { Serial.println("[Handshake] PROOF rejected — invalid hex"); return; }
    memcpy(salt, _appNonce, 32); memcpy(salt+32, _dongleNonce, 32);
    hmac(_psk, 32, salt, sizeof(salt), expected);
    if (!constantTimeEquals(got, expected, 32)) {
        Serial.println("[Handshake] PROOF MISMATCH — sending ERROR (check PSK matches app's SECURE_PSK_HEX)");
        sendRaw((const uint8_t*)"ERROR\n", 6); resetToIdle(); return;
    }
    static const uint8_t info[] = "session-key";
    hkdfSha256(salt, sizeof(salt), _psk, 32, info, sizeof(info)-1, _sessionKey, 32);
    static const uint8_t label[] = "confirm"; hmac(_sessionKey, 32, label, sizeof(label)-1, expected);
    char mac[65], frame[70]; hexEncode(expected, 32, mac);
    int n = snprintf(frame, sizeof(frame), "OK %s\n", mac); sendRaw((const uint8_t*)frame, n);
    _state = HandshakeState::ESTABLISHED;
    Serial.println("[Handshake] PROOF verified — session established, OK sent");
}

void SecureHandshakeBleConnectivity::sendSecured(const uint8_t* plain, size_t len) {
    if (_state != HandshakeState::ESTABLISHED || len > kSecureMaxMsgLen) return;
    uint8_t iv[IV_LEN], cipher[kSecureMaxMsgLen], tag[TAG_LEN]; esp_fill_random(iv, IV_LEN);
    mbedtls_gcm_context ctx; mbedtls_gcm_init(&ctx);
    int ret = mbedtls_gcm_setkey(&ctx, MBEDTLS_CIPHER_ID_AES, _sessionKey, 256);
    if (!ret) ret = mbedtls_gcm_crypt_and_tag(&ctx, MBEDTLS_GCM_ENCRYPT, len, iv, IV_LEN, nullptr, 0, plain, cipher, TAG_LEN, tag);
    mbedtls_gcm_free(&ctx); if (ret) return;
    char out[24 + kSecureMaxMsgLen*2 + 32 + 2]; hexEncode(iv, IV_LEN, out); hexEncode(cipher, len, out+24); hexEncode(tag, TAG_LEN, out+24+len*2);
    size_t n = 24 + len*2 + 32; out[n++] = '\n'; sendRaw((const uint8_t*)out, n);
}

void SecureHandshakeBleConnectivity::resetToIdle() { _state = HandshakeState::IDLE; memset(_appNonce,0,sizeof(_appNonce)); memset(_dongleNonce,0,sizeof(_dongleNonce)); memset(_sessionKey,0,sizeof(_sessionKey)); }
void SecureHandshakeBleConnectivity::hmac(const uint8_t* key, size_t keyLen, const uint8_t* data, size_t len, uint8_t* out) {
    auto* info = mbedtls_md_info_from_type(MBEDTLS_MD_SHA256); mbedtls_md_context_t ctx; mbedtls_md_init(&ctx); mbedtls_md_setup(&ctx, info, 1); mbedtls_md_hmac_starts(&ctx,key,keyLen); mbedtls_md_hmac_update(&ctx,data,len); mbedtls_md_hmac_finish(&ctx,out); mbedtls_md_free(&ctx);
}
bool SecureHandshakeBleConnectivity::constantTimeEquals(const uint8_t* a,const uint8_t* b,size_t n){uint8_t d=0;for(size_t i=0;i<n;++i)d|=a[i]^b[i];return d==0;}
int SecureHandshakeBleConnectivity::hexNibble(char c){if(c>='0'&&c<='9')return c-'0';if(c>='a'&&c<='f')return c-'a'+10;if(c>='A'&&c<='F')return c-'A'+10;return -1;}
bool SecureHandshakeBleConnectivity::hexDecode(const char* h,size_t n,uint8_t* out,size_t outLen){if(n!=outLen*2)return false;for(size_t i=0;i<outLen;++i){int a=hexNibble(h[i*2]),b=hexNibble(h[i*2+1]);if(a<0||b<0)return false;out[i]=(a<<4)|b;}return true;}
void SecureHandshakeBleConnectivity::hexEncode(const uint8_t* in,size_t n,char* out){static const char* h="0123456789abcdef";for(size_t i=0;i<n;++i){out[i*2]=h[in[i]>>4];out[i*2+1]=h[in[i]&15];}out[n*2]=0;}
