#include <unity.h>

#include <cstdlib>
#include <cstring>
#include <string>
#include <vector>

#include "crypto/ConstantTime.h"
#include "crypto/Crc32.h"
#include "crypto/HmacSha256.h"
#include "crypto/Sha256.h"
#include "security/BlockAuth.h"
#include "security/DeviceKeys.h"
#include "security/HandshakeAuth.h"
#include "security/SecurityProfile.h"
#include "support/vectors.h"

void setUp() {}
void tearDown() {}

static std::vector<uint8_t> unhex(const char* s) {
    std::vector<uint8_t> out;
    for (size_t i = 0; s[i] && s[i + 1]; i += 2) {
        char b[3] = {s[i], s[i + 1], 0};
        out.push_back(static_cast<uint8_t>(strtoul(b, nullptr, 16)));
    }
    return out;
}

static void assertHex(const char* expectedHex, const uint8_t* actual, size_t len) {
    std::vector<uint8_t> e = unhex(expectedHex);
    TEST_ASSERT_EQUAL_UINT32(e.size(), len);
    TEST_ASSERT_EQUAL_HEX8_ARRAY(e.data(), actual, len);
}

static void sha(const std::vector<uint8_t>& in, uint8_t out[32]) {
    crypto::Sha256 s;
    s.update(in.data(), in.size());
    s.finish(out);
}

// ── SHA-256 ──────────────────────────────────────────────────────────────

void test_sha256_known_vectors() {
    uint8_t out[32];
    sha(unhex(VEC_SHA256_EMPTY_IN), out);
    assertHex(VEC_SHA256_EMPTY_OUT, out, 32);
    sha(unhex(VEC_SHA256_ABC_IN), out);
    assertHex(VEC_SHA256_ABC_OUT, out, 32);
}

void test_sha256_million_a() {
    crypto::Sha256 s;
    std::vector<uint8_t> chunk(1000, 'a');
    for (int i = 0; i < 1000; i++) s.update(chunk.data(), chunk.size());
    uint8_t out[32];
    s.finish(out);
    assertHex("CDC76E5C9914FB9281A1C7E284D73E67F1809A48A497200E046D39CCC7112CD0", out, 32);
}

void test_sha256_chunked_equals_single() {
    std::vector<uint8_t> data(300);
    for (size_t i = 0; i < data.size(); i++) data[i] = static_cast<uint8_t>(i * 7 + 3);
    uint8_t whole[32];
    sha(data, whole);
    for (size_t step : {1u, 63u, 64u, 65u}) {
        crypto::Sha256 s;
        for (size_t i = 0; i < data.size(); i += step) {
            s.update(data.data() + i, i + step <= data.size() ? step : data.size() - i);
        }
        uint8_t out[32];
        s.finish(out);
        TEST_ASSERT_EQUAL_HEX8_ARRAY(whole, out, 32);
    }
}

// ── HMAC-SHA256 ──────────────────────────────────────────────────────────

static void hmacOnce(const std::vector<uint8_t>& key, const std::vector<uint8_t>& data, uint8_t out[32]) {
    crypto::PortableHmacSha256 h(key.data(), key.size());
    h.update(data.data(), data.size());
    h.finish(out);
}

void test_hmac_rfc4231() {
    uint8_t out[32];
    hmacOnce(unhex(VEC_HMAC_CASE1_KEY), unhex(VEC_HMAC_CASE1_DATA), out);
    assertHex(VEC_HMAC_CASE1_MAC, out, 32);
    hmacOnce(unhex(VEC_HMAC_CASE2_KEY), unhex(VEC_HMAC_CASE2_DATA), out);
    assertHex(VEC_HMAC_CASE2_MAC, out, 32);
}

void test_hmac_rfc4231_case6_long_key() {
    std::vector<uint8_t> key(131, 0xaa);
    std::string msg = "Test Using Larger Than Block-Size Key - Hash Key First";
    uint8_t out[32];
    hmacOnce(key, std::vector<uint8_t>(msg.begin(), msg.end()), out);
    assertHex("60E431591EE0B67F0D8A26AACBF5B77F8E0BC6213728C5140546040F0EE37F54", out, 32);
}

void test_hmac_begin_reuse_equals_new_instance() {
    std::vector<uint8_t> key = unhex(VEC_K_AUTH);
    std::vector<uint8_t> data = unhex("0102030405");
    uint8_t first[32], second[32], fresh[32];
    crypto::PortableHmacSha256 h(key.data(), key.size());
    h.update(data.data(), data.size());
    h.finish(first);
    h.begin();
    h.update(data.data(), data.size());
    h.finish(second);
    hmacOnce(key, data, fresh);
    TEST_ASSERT_EQUAL_HEX8_ARRAY(first, second, 32);
    TEST_ASSERT_EQUAL_HEX8_ARRAY(fresh, second, 32);
}

// ── CRC32 e tempo constante ──────────────────────────────────────────────

void test_crc32() {
    std::vector<uint8_t> in = unhex(VEC_CRC32_CHECK_IN);
    TEST_ASSERT_EQUAL_HEX32(0xCBF43926u, crypto::crc32(in.data(), in.size()));
    uint32_t chained = crypto::crc32(in.data() + 4, in.size() - 4, crypto::crc32(in.data(), 4));
    TEST_ASSERT_EQUAL_HEX32(0xCBF43926u, chained);
}

void test_ct_equal() {
    uint8_t a[4] = {1, 2, 3, 4}, b[4] = {1, 2, 3, 4};
    TEST_ASSERT_TRUE(crypto::ctEqual(a, b, 4));
    b[0] ^= 1;
    TEST_ASSERT_FALSE(crypto::ctEqual(a, b, 4));
    b[0] ^= 1;
    b[3] ^= 0x80;
    TEST_ASSERT_FALSE(crypto::ctEqual(a, b, 4));
}

// ── Autenticação com as chaves fixas (§12) ───────────────────────────────

static void checkHandshake(const char* payloadHex, const char* tagHex, uint8_t type) {
    std::vector<uint8_t> key = unhex(VEC_K_AUTH), dev = unhex(VEC_DEV_ID), p = unhex(payloadHex);
    crypto::PortableHmacSha256 h(key.data(), key.size());
    security::HmacHandshakeAuth auth(h, dev.data());
    uint8_t tag[16];
    auth.tag(type, p.data(), p.size(), tag);
    assertHex(tagHex, tag, 16);
    TEST_ASSERT_TRUE(auth.verify(type, p.data(), p.size(), tag));
    tag[7] ^= 0x01;
    TEST_ASSERT_FALSE(auth.verify(type, p.data(), p.size(), tag));
}

void test_handshake_vectors() {
    checkHandshake(VEC_HS_DESAFIO_PAYLOAD, VEC_HS_DESAFIO_TAG, VEC_HS_DESAFIO_TYPE);
    checkHandshake(VEC_HS_DESAFIO_DIAG_PAYLOAD, VEC_HS_DESAFIO_DIAG_TAG, VEC_HS_DESAFIO_DIAG_TYPE);
    checkHandshake(VEC_HS_RESPOSTA_PAYLOAD, VEC_HS_RESPOSTA_TAG, VEC_HS_RESPOSTA_TYPE);
    checkHandshake(VEC_HS_ACK_PAYLOAD, VEC_HS_ACK_TAG, VEC_HS_ACK_TYPE);
}

static void checkBlock(const char* dataHex, const char* tagHex, const char* ecdsaHashHex) {
    std::vector<uint8_t> key = unhex(VEC_K_REC), dev = unhex(VEC_DEV_ID), d = unhex(dataHex);
    crypto::PortableHmacSha256 h(key.data(), key.size());
    security::HmacBlockAuth auth(h, dev.data());
    TEST_ASSERT_EQUAL_UINT32(32, auth.tagSize());
    uint8_t tag[32];
    auth.sign(d.data(), d.size(), tag);
    assertHex(tagHex, tag, 32);
    TEST_ASSERT_TRUE(auth.verify(d.data(), d.size(), tag));
    tag[31] ^= 0x01;
    TEST_ASSERT_FALSE(auth.verify(d.data(), d.size(), tag));

    // Entrada do ECDSA: SHA-256("MALCONG" || 03 || DevId || dados)
    std::vector<uint8_t> in(security::kDomainLabel, security::kDomainLabel + 7);
    in.push_back(0x03);
    in.insert(in.end(), dev.begin(), dev.end());
    in.insert(in.end(), d.begin(), d.end());
    uint8_t digest[32];
    sha(in, digest);
    assertHex(ecdsaHashHex, digest, 32);
}

void test_block_vectors() {
    checkBlock(VEC_BLK_BOOT_DATA, VEC_BLK_BOOT_TAG, VEC_BLK_BOOT_ECDSA_HASH);
    checkBlock(VEC_BLK_LOSS_DATA, VEC_BLK_LOSS_TAG, VEC_BLK_LOSS_ECDSA_HASH);
}

void test_no_auth_and_no_handshake_auth() {
    collector::NoAuth none;
    TEST_ASSERT_EQUAL_UINT32(0, none.tagSize());
    security::NoHandshakeAuth hs;
    uint8_t tag[16];
    memset(tag, 0xAA, sizeof(tag));
    hs.tag(0x01, nullptr, 0, tag);
    for (uint8_t b : tag) TEST_ASSERT_EQUAL_UINT8(0, b);
    uint8_t junk[16] = {1};
    TEST_ASSERT_TRUE(hs.verify(0x02, nullptr, 0, junk));
}

void test_profile_tag_sizes() {
    TEST_ASSERT_EQUAL_UINT32(16, security::kHandshakeTagSize);
    TEST_ASSERT_TRUE(security::kBlockTagSize == 0 || security::kBlockTagSize == 32 ||
                     security::kBlockTagSize == 64);
}

int main(int, char**) {
    UNITY_BEGIN();
    RUN_TEST(test_sha256_known_vectors);
    RUN_TEST(test_sha256_million_a);
    RUN_TEST(test_sha256_chunked_equals_single);
    RUN_TEST(test_hmac_rfc4231);
    RUN_TEST(test_hmac_rfc4231_case6_long_key);
    RUN_TEST(test_hmac_begin_reuse_equals_new_instance);
    RUN_TEST(test_crc32);
    RUN_TEST(test_ct_equal);
    RUN_TEST(test_handshake_vectors);
    RUN_TEST(test_block_vectors);
    RUN_TEST(test_no_auth_and_no_handshake_auth);
    RUN_TEST(test_profile_tag_sizes);
    return UNITY_END();
}
