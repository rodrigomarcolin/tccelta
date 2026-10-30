#include <unity.h>

#include <cstdlib>
#include <cstring>
#include <vector>

#include "sync/Fragmenter.h"
#include "sync/SyncProtocol.h"
#include "support/vectors.h"

using namespace datasync;

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

static std::vector<uint8_t> concat(std::vector<uint8_t> a, const std::vector<uint8_t>& b) {
    a.insert(a.end(), b.begin(), b.end());
    return a;
}

static std::vector<uint8_t> tlv(uint8_t type, const std::vector<uint8_t>& v) {
    uint8_t h[3];
    putTlvHeader(type, static_cast<uint16_t>(v.size()), h);
    return concat(std::vector<uint8_t>(h, h + 3), v);
}

void test_parse_tlv() {
    const uint8_t ok[] = {0x05, 0x02, 0x00, 0x0A, 0x00};
    TlvView v;
    TEST_ASSERT_TRUE(parseTlv(ok, sizeof(ok), v));
    TEST_ASSERT_EQUAL_UINT8(0x05, v.type);
    TEST_ASSERT_EQUAL_UINT16(2, v.len);

    TEST_ASSERT_FALSE(parseTlv(ok, 2, v));            // menor que o cabeçalho
    TEST_ASSERT_FALSE(parseTlv(ok, sizeof(ok) - 1, v));  // L maior que o resto
    const uint8_t longer[] = {0x05, 0x01, 0x00, 0x0A, 0x00};
    TEST_ASSERT_FALSE(parseTlv(longer, sizeof(longer), v));  // L menor que o resto
}

void test_decode_challenge_lengths_and_flags() {
    const auto payload = unhex(VEC_HS_DESAFIO_PAYLOAD);
    const auto tag = unhex(VEC_HS_DESAFIO_TAG);
    auto v = concat(payload, tag);
    Challenge c;
    TEST_ASSERT_TRUE(decodeChallenge(v.data(), v.size(), c));
    TEST_ASSERT_EQUAL_UINT32(7, c.cs);
    TEST_ASSERT_EQUAL_UINT32(100, c.ackSeq);
    TEST_ASSERT_EQUAL_UINT32(6, c.cConf);
    TEST_ASSERT_EQUAL_UINT32(29, c.payloadLen);
    TEST_ASSERT_EQUAL_PTR(v.data() + 29, c.tag);
    TEST_ASSERT_EQUAL_HEX8_ARRAY(tag.data(), c.tag, 16);

    const auto dv = concat(unhex(VEC_HS_DESAFIO_DIAG_PAYLOAD), unhex(VEC_HS_DESAFIO_DIAG_TAG));
    TEST_ASSERT_EQUAL_UINT32(47, dv.size());
    TEST_ASSERT_TRUE(decodeChallenge(dv.data(), dv.size(), c));
    TEST_ASSERT_EQUAL_UINT8(1, c.flags);
    TEST_ASSERT_EQUAL_UINT16(30, c.diagMin);
    TEST_ASSERT_EQUAL_UINT32(31, c.payloadLen);

    // 45 B com flags=1 e 47 B com flags=0 são incoerentes.
    auto bad1 = v;
    bad1[28] = 1;
    TEST_ASSERT_FALSE(decodeChallenge(bad1.data(), bad1.size(), c));
    auto bad2 = dv;
    bad2[28] = 0;
    TEST_ASSERT_FALSE(decodeChallenge(bad2.data(), bad2.size(), c));
    TEST_ASSERT_FALSE(decodeChallenge(v.data(), 46, c));

    // Bits reservados ligados são ignorados (compatibilidade futura).
    auto reserved = v;
    reserved[28] = 0x80;
    TEST_ASSERT_TRUE(decodeChallenge(reserved.data(), reserved.size(), c));
}

void test_encode_response_payload_matches_vector() {
    Response r;
    r.cs = 7;
    for (int i = 0; i < 16; ++i) r.nonce[i] = static_cast<uint8_t>(i);
    r.tailSeq = 65;
    r.headSeq = 130;
    r.bootId = 3;
    r.tMs = 123456;
    uint8_t out[kResponsePayloadLen];
    TEST_ASSERT_EQUAL_UINT32(34, encodeResponsePayload(r, out));
    const auto expected = unhex(VEC_HS_RESPOSTA_PAYLOAD);
    TEST_ASSERT_EQUAL_HEX8_ARRAY(expected.data(), out, 34);
}

void test_ack_roundtrip_and_notice() {
    uint8_t payload[kAckPayloadLen];
    encodeAckPayload(130, 7, 0, payload);
    const auto expected = unhex(VEC_HS_ACK_PAYLOAD);
    TEST_ASSERT_EQUAL_HEX8_ARRAY(expected.data(), payload, 9);

    const auto v = concat(expected, unhex(VEC_HS_ACK_TAG));
    Ack ack;
    TEST_ASSERT_TRUE(decodeAck(v.data(), v.size(), ack));
    TEST_ASSERT_EQUAL_UINT32(130, ack.ackSeq);
    TEST_ASSERT_EQUAL_UINT32(7, ack.cConf);
    TEST_ASSERT_FALSE(decodeAck(v.data(), v.size() - 1, ack));

    const auto devId = unhex(VEC_DEV_ID);
    uint8_t notice[kNoticeLen];
    encodeNotice(130, 0x03, devId.data(), notice);
    const auto exp = unhex("8200000003544343454C544101");
    TEST_ASSERT_EQUAL_HEX8_ARRAY(exp.data(), notice, 13);

    uint16_t k = 0;
    const uint8_t req[] = {0x0A, 0x00};
    TEST_ASSERT_TRUE(decodeBatchRequest(req, 2, k));
    TEST_ASSERT_EQUAL_UINT16(10, k);
    TEST_ASSERT_FALSE(decodeBatchRequest(req, 1, k));
}

void test_wire_examples_from_spec() {
    // Aviso HEAD=130 flags=03: 07 0D00 82000000 03 544343454C544101
    const auto devId = unhex(VEC_DEV_ID);
    uint8_t notice[kNoticeLen];
    encodeNotice(130, 0x03, devId.data(), notice);
    const auto m = tlv(0x07, std::vector<uint8_t>(notice, notice + 13));
    const auto expected = unhex("070D0082000000" "03544343454C544101");
    TEST_ASSERT_EQUAL_UINT32(expected.size(), m.size());
    TEST_ASSERT_EQUAL_HEX8_ARRAY(expected.data(), m.data(), m.size());
}

static std::vector<std::vector<uint8_t>> fragments(const std::vector<uint8_t>& msg, uint16_t mtu) {
    Fragmenter f(msg.data(), msg.size(), mtu);
    std::vector<std::vector<uint8_t>> out;
    uint8_t buf[600];
    size_t n;
    while (f.next(buf, n)) out.emplace_back(buf, buf + n);
    TEST_ASSERT_EQUAL_UINT32(out.size(), f.count());
    return out;
}

void test_fragmenter_spec_example() {
    // Resposta de 53 B com MTU 23 (chunk 19): 3 fragmentos.
    const auto v = concat(unhex(VEC_HS_RESPOSTA_PAYLOAD), unhex(VEC_HS_RESPOSTA_TAG));
    const auto msg = tlv(0x02, v);
    TEST_ASSERT_EQUAL_UINT32(53, msg.size());
    const auto frags = fragments(msg, 23);
    TEST_ASSERT_EQUAL_UINT32(3, frags.size());
    const auto f0 = unhex("80" "02320007000000000102030405060708090A0B");
    const auto f1 = unhex("01" "0C0D0E0F4100000082000000030040E201003A");
    const auto f2 = unhex("02" "93F9DC93EDA9E7DBAD035426EEB061");
    TEST_ASSERT_EQUAL_HEX8_ARRAY(f0.data(), frags[0].data(), f0.size());
    TEST_ASSERT_EQUAL_HEX8_ARRAY(f1.data(), frags[1].data(), f1.size());
    TEST_ASSERT_EQUAL_HEX8_ARRAY(f2.data(), frags[2].data(), f2.size());
}

void test_fragmenter_boundaries_and_counter_wrap() {
    TEST_ASSERT_EQUAL_UINT32(1, fragments(std::vector<uint8_t>(19, 1), 23).size());
    TEST_ASSERT_EQUAL_UINT32(2, fragments(std::vector<uint8_t>(20, 1), 23).size());

    const auto big = fragments(std::vector<uint8_t>(1091, 7), 23);
    TEST_ASSERT_EQUAL_UINT32(58, big.size());
    TEST_ASSERT_EQUAL_HEX8(0x80, big[0][0]);
    TEST_ASSERT_EQUAL_HEX8(0x01, big[1][0]);
    TEST_ASSERT_EQUAL_HEX8(0x39, big[57][0]);

    const auto wrap = fragments(std::vector<uint8_t>(3000, 7), 23);
    TEST_ASSERT_EQUAL_HEX8(0x7F, wrap[127][0]);
    TEST_ASSERT_EQUAL_HEX8(0x00, wrap[128][0]);  // volta a 0 sem o bit 7

    // MTU abaixo de 23 vale 23; bloco de 1059 B com MTU 247 -> 5 fragmentos.
    TEST_ASSERT_EQUAL_UINT32(fragments(std::vector<uint8_t>(100, 1), 23).size(),
                             fragments(std::vector<uint8_t>(100, 1), 10).size());
    const auto hmac = fragments(std::vector<uint8_t>(1059, 1), 247);
    TEST_ASSERT_EQUAL_UINT32(5, hmac.size());
    TEST_ASSERT_EQUAL_UINT32(1 + 87, hmac[4].size());
}

int main(int, char**) {
    UNITY_BEGIN();
    RUN_TEST(test_parse_tlv);
    RUN_TEST(test_decode_challenge_lengths_and_flags);
    RUN_TEST(test_encode_response_payload_matches_vector);
    RUN_TEST(test_ack_roundtrip_and_notice);
    RUN_TEST(test_wire_examples_from_spec);
    RUN_TEST(test_fragmenter_spec_example);
    RUN_TEST(test_fragmenter_boundaries_and_counter_wrap);
    return UNITY_END();
}
