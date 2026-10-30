#include <unity.h>

#include <cstdlib>
#include <cstring>
#include <string>
#include <vector>

#include "collector/Events.h"
#include "collector/MarkerBlock.h"
#include "collector/SegmentFormat.h"
#include "crypto/HmacSha256.h"
#include "security/BlockAuth.h"
#include "security/DeviceKeys.h"
#include "security/HandshakeAuth.h"
#include "support/Fakes.h"
#include "support/SyncFakes.h"
#include "support/vectors.h"
#include "sync/SyncEngine.h"
#include "sync/SyncProtocol.h"

using namespace datasync;
using collector::Block;

namespace {

class CaptureSink : public collector::IEventSink {
public:
    void emit(const char* body, size_t len, const uint32_t*) override { lines.emplace_back(body, len); }
    std::vector<std::string> of(const std::string& event) const {
        std::vector<std::string> out;
        for (const auto& l : lines) {
            if (l.compare(0, event.size() + 1, event + " ") == 0) out.push_back(l);
        }
        return out;
    }
    std::vector<std::string> lines;
};
CaptureSink sink;

std::vector<uint8_t> unhex(const char* s) {
    std::vector<uint8_t> out;
    for (size_t i = 0; s[i] && s[i + 1]; i += 2) {
        char b[3] = {s[i], s[i + 1], 0};
        out.push_back(static_cast<uint8_t>(strtoul(b, nullptr, 16)));
    }
    return out;
}

std::vector<uint8_t> cat(std::vector<uint8_t> a, const std::vector<uint8_t>& b) {
    a.insert(a.end(), b.begin(), b.end());
    return a;
}

std::vector<uint8_t> tlv(uint8_t type, const std::vector<uint8_t>& v) {
    uint8_t h[3];
    putTlvHeader(type, static_cast<uint16_t>(v.size()), h);
    return cat(std::vector<uint8_t>(h, h + 3), v);
}

/** Bloco de dados íntegro, com tag preenchida (o motor não confere a tag, só o CRC). */
Block makeBlock(uint32_t seq, uint8_t tagLen = 32, size_t length = 17) {
    Block b = {};
    b.length = length;
    collector::putU32(&b.bytes[collector::header::kSeq], seq);
    collector::putU16(&b.bytes[collector::header::kBootId], 1);
    collector::putU16(&b.bytes[collector::header::kLength], static_cast<uint16_t>(length));
    b.bytes[collector::kHeaderSize] = 0x0C;
    b.tagLength = tagLen;
    for (uint8_t i = 0; i < tagLen; ++i) b.tag[i] = static_cast<uint8_t>(seq + i);
    b.crc = collector::computeCrc(b);
    return b;
}

Block makeBoot(uint32_t seq, uint32_t from, uint32_t to, uint8_t tagLen = 32) {
    Block b = {};
    collector::buildBootMarker(b, seq, 4, 812, from, to);
    b.tagLength = tagLen;
    for (uint8_t i = 0; i < tagLen; ++i) b.tag[i] = static_cast<uint8_t>(seq + i);
    b.crc = collector::computeCrc(b);
    return b;
}

void fill(MemoryBlockStore& store, uint32_t first, uint32_t last, uint8_t tagLen = 32) {
    for (uint32_t s = first; s <= last; ++s) store.blocks.push_back(makeBlock(s, tagLen));
}

uint32_t u32(const std::vector<uint8_t>& m, size_t offset) { return collector::getU32(&m[offset]); }

struct Rig {
    MemoryBlockStore store;
    FakeSyncLink link;
    FakeSyncPersistence nvs;
    FakeClock clock;
    std::vector<uint8_t> devId = unhex(VEC_DEV_ID);
    std::vector<uint8_t> kAuth = unhex(VEC_K_AUTH);
    std::vector<uint8_t> kRec = unhex(VEC_K_REC);
    crypto::PortableHmacSha256 hmacAuth{kAuth.data(), kAuth.size()};
    crypto::PortableHmacSha256 hmacRec{kRec.data(), kRec.size()};
    security::HmacHandshakeAuth hs{hmacAuth, devId.data()};
    security::HmacBlockAuth blockAuth{hmacRec, devId.data()};
    collector::NoAuth noAuth;
    FakeAuth fake64{64};
    SyncEngine* engine = nullptr;
    uint32_t nextCs = 7;

    /** `profile`: 0 = none (tag 0 B), 1 = hmac (32 B), 2 = tag falsa de 64 B (asym). */
    explicit Rig(int profile = 1, uint32_t lastCs = 6) {
        nvs.lastCs = lastCs;
        nvs.link = &link;
        collector::IMessageAuth& loss = profile == 0   ? static_cast<collector::IMessageAuth&>(noAuth)
                                        : profile == 2 ? static_cast<collector::IMessageAuth&>(fake64)
                                                       : static_cast<collector::IMessageAuth&>(blockAuth);
        SyncDeps deps{store, nvs, hs, loss, link, clock, devId.data(), 3, nullptr};
        engine = new SyncEngine(deps);
    }
    ~Rig() { delete engine; }

    void start() {
        engine->begin();
        engine->onConnected(1);
        engine->onSubscribed(true);
    }

    std::vector<uint8_t> challenge(uint32_t cs, uint32_t ack, uint32_t cConf, bool badTag = false) {
        std::vector<uint8_t> nonce(16);
        for (int i = 0; i < 16; ++i) nonce[i] = static_cast<uint8_t>(i);
        uint8_t le[4];
        collector::putU32(le, cs);
        std::vector<uint8_t> p(le, le + 4);
        p = cat(p, nonce);
        collector::putU32(le, ack);
        p.insert(p.end(), le, le + 4);
        collector::putU32(le, cConf);
        p.insert(p.end(), le, le + 4);
        p.push_back(0);
        uint8_t tag[16];
        hs.tag(0x01, p.data(), p.size(), tag);
        if (badTag) tag[0] ^= 1;
        return tlv(0x01, cat(p, std::vector<uint8_t>(tag, tag + 16)));
    }

    std::vector<uint8_t> ackMsg(uint32_t ack, uint32_t cConf, uint8_t flags = 0) {
        uint8_t p[9];
        encodeAckPayload(ack, cConf, flags, p);
        uint8_t tag[16];
        hs.tag(0x04, p, 9, tag);
        return tlv(0x04, cat(std::vector<uint8_t>(p, p + 9), std::vector<uint8_t>(tag, tag + 16)));
    }

    void feed(const std::vector<uint8_t>& m) { engine->onMessage(m.data(), m.size(), 300); }

    /** Handshake completo: desafio + ACK de abertura com `ack`. */
    void openSession(uint32_t ack, uint8_t ackFlags = 0) {
        const uint32_t cs = nextCs++;
        feed(challenge(cs, ack, cs - 1));
        feed(ackMsg(ack, cs, ackFlags));
    }

    void request(uint16_t k) {
        uint8_t le[2];
        collector::putU16(le, k);
        feed(tlv(0x05, std::vector<uint8_t>(le, le + 2)));
    }

    /** Roda o lote até acabar; devolve os `seq` e os tipos dos blocos enviados (TLV 03). */
    std::vector<std::vector<uint8_t>> drain() {
        const size_t before = link.ofType(0x03).size();
        for (int i = 0; i < 1000 && engine->batchActive(); ++i) engine->sendNextBlock();
        auto all = link.ofType(0x03);
        return std::vector<std::vector<uint8_t>>(all.begin() + before, all.end());
    }

    std::vector<uint8_t> lastNotice() const {
        auto n = link.ofType(0x07);
        return n.empty() ? std::vector<uint8_t>() : n.back();
    }
};

}  // namespace

void setUp() {
    sink = CaptureSink();
    collector::setEventSink(&sink);
}
void tearDown() { collector::setEventSink(nullptr); }

// 1. Conexão
void test_subscribe_sends_notice_with_both_handshake_bits() {
    Rig rig;
    fill(rig.store, 65, 130);
    rig.start();
    const auto n = rig.lastNotice();
    TEST_ASSERT_EQUAL_UINT32(16, n.size());
    TEST_ASSERT_EQUAL_UINT32(130, u32(n, 3));
    TEST_ASSERT_EQUAL_HEX8(0x03, n[7]);
    const auto devId = unhex(VEC_DEV_ID);
    TEST_ASSERT_EQUAL_HEX8_ARRAY(devId.data(), &n[8], 8);
}

// 2. Handshake feliz
void test_happy_handshake_saves_cs_before_reply_and_frees_flash() {
    Rig rig;
    fill(rig.store, 65, 130);
    rig.start();
    const size_t beforeChallenge = rig.link.sent.size();
    rig.clock.now = 5000;

    const auto payload = unhex(VEC_HS_DESAFIO_PAYLOAD);
    rig.feed(tlv(0x01, cat(payload, unhex(VEC_HS_DESAFIO_TAG))));  // c_S=7, ACK_seq=100, c_conf=6

    TEST_ASSERT_EQUAL_UINT32(7, rig.nvs.lastCs);
    TEST_ASSERT_EQUAL_UINT32(beforeChallenge, rig.nvs.sentAtSave);  // salvou antes de qualquer resposta
    TEST_ASSERT_EQUAL_UINT32(101, rig.store.oldestSeq());

    const auto responses = rig.link.ofType(0x02);
    TEST_ASSERT_EQUAL_UINT32(1, responses.size());
    const auto& r = responses[0];
    TEST_ASSERT_EQUAL_UINT32(53, r.size());
    TEST_ASSERT_EQUAL_UINT32(7, u32(r, 3));
    TEST_ASSERT_EQUAL_UINT32(101, u32(r, 23));  // TAIL
    TEST_ASSERT_EQUAL_UINT32(130, u32(r, 27));  // HEAD
    TEST_ASSERT_EQUAL_UINT16(3, collector::getU16(&r[31]));
    TEST_ASSERT_EQUAL_UINT32(5000, u32(r, 33));
    TEST_ASSERT_TRUE(rig.hs.verify(0x02, &r[3], 34, &r[37]));
    TEST_ASSERT_EQUAL_UINT32(101, rig.engine->status().cursor);
}

void test_response_matches_spec_vector_when_nothing_is_freed() {
    Rig rig;
    fill(rig.store, 65, 130);
    rig.start();
    rig.clock.now = 123456;
    rig.feed(rig.challenge(7, 60, 6));  // ACK_seq 60 <= ackLocal (64): nada apagado
    const auto r = rig.link.ofType(0x02).at(0);
    const auto expected = cat(unhex(VEC_HS_RESPOSTA_PAYLOAD), unhex(VEC_HS_RESPOSTA_TAG));
    TEST_ASSERT_EQUAL_UINT32(65, rig.store.oldestSeq());
    TEST_ASSERT_EQUAL_HEX8_ARRAY(expected.data(), &r[3], 50);
}

// 3. ACK de abertura
void test_ack_opens_session_and_clears_notice_flags() {
    Rig rig;
    fill(rig.store, 65, 130);
    rig.start();
    rig.feed(rig.challenge(7, 100, 6));
    rig.feed(tlv(0x04, cat(unhex(VEC_HS_ACK_PAYLOAD), unhex(VEC_HS_ACK_TAG))));  // ACK_seq=130, c_conf=7
    TEST_ASSERT_TRUE(rig.engine->status().sessionOpen);
    TEST_ASSERT_EQUAL_HEX8(0x00, rig.lastNotice()[7]);
}

void test_ack_with_anchor_failed_keeps_boot_bit() {
    Rig rig;
    fill(rig.store, 65, 140);
    rig.start();
    rig.openSession(130, 1);
    TEST_ASSERT_TRUE(rig.engine->status().sessionOpen);
    TEST_ASSERT_EQUAL_HEX8(0x02, rig.lastNotice()[7]);
}

// 4. Erros
void test_errors_for_bad_tag_stale_cs_bad_ack_and_no_session() {
    Rig rig;
    fill(rig.store, 65, 130);
    rig.start();
    auto lastError = [&]() { return rig.link.ofType(0x06).back(); };

    rig.feed(rig.challenge(7, 100, 6, true));
    TEST_ASSERT_EQUAL_HEX8(0x01, lastError()[3]);
    TEST_ASSERT_EQUAL_UINT32(0, rig.link.ofType(0x02).size());

    rig.feed(rig.challenge(6, 100, 5));  // c_S <= last_cs
    TEST_ASSERT_EQUAL_HEX8(0x02, lastError()[3]);

    rig.feed(tlv(0x01, std::vector<uint8_t>(46, 0)));  // L inválido
    TEST_ASSERT_EQUAL_HEX8(0x01, lastError()[3]);

    rig.feed(rig.challenge(7, 100, 6));
    rig.feed(rig.ackMsg(131, 7));  // HEAD + 1
    TEST_ASSERT_EQUAL_HEX8(0x03, lastError()[3]);
    TEST_ASSERT_FALSE(rig.engine->status().sessionOpen);
    rig.feed(rig.ackMsg(99, 7));   // abaixo de ackLocal (100)
    TEST_ASSERT_EQUAL_HEX8(0x03, lastError()[3]);
    TEST_ASSERT_FALSE(rig.engine->status().sessionOpen);

    rig.request(10);  // sem sessão
    TEST_ASSERT_EQUAL_HEX8(0x05, lastError()[3]);
}

void test_nvs_failure_sends_no_response() {
    Rig rig;
    fill(rig.store, 65, 130);
    rig.start();
    rig.nvs.fail = true;
    rig.feed(rig.challenge(7, 100, 6));
    TEST_ASSERT_EQUAL_UINT32(0, rig.link.ofType(0x02).size());
    TEST_ASSERT_EQUAL_UINT32(1, sink.of("HS_CHALLENGE").size());
    TEST_ASSERT_TRUE(sink.of("HS_CHALLENGE")[0].find("result=nvs_fail") != std::string::npos);
}

// 5. Lote
void test_batch_requests_send_in_order_with_tag_length() {
    Rig rig;
    fill(rig.store, 65, 140);
    rig.start();
    rig.openSession(130);
    rig.request(5);
    auto blocks = rig.drain();
    TEST_ASSERT_EQUAL_UINT32(5, blocks.size());
    TEST_ASSERT_EQUAL_UINT32(131, u32(blocks[0], 3));
    TEST_ASSERT_EQUAL_UINT32(135, u32(blocks[4], 3));
    TEST_ASSERT_EQUAL_UINT16(17 + 32, collector::getU16(&blocks[0][1]));  // L = tamanho + tag
    TEST_ASSERT_EQUAL_UINT32(136, rig.engine->status().cursor);

    rig.request(100);
    blocks = rig.drain();
    TEST_ASSERT_EQUAL_UINT32(5, blocks.size());
    TEST_ASSERT_EQUAL_UINT32(140, u32(blocks[4], 3));

    const size_t sent = rig.link.sent.size();
    rig.request(10);  // cursor > HEAD
    TEST_ASSERT_FALSE(rig.engine->batchActive());
    TEST_ASSERT_EQUAL_UINT32(sent, rig.link.sent.size());
}

// 6. Sem esperar ACK
void test_cursor_does_not_wait_for_ack_and_ack_does_not_rewind() {
    Rig rig;
    fill(rig.store, 65, 140);
    rig.start();
    rig.openSession(130);
    rig.request(3);
    rig.drain();
    rig.request(3);
    rig.drain();
    TEST_ASSERT_EQUAL_UINT32(137, rig.engine->status().cursor);
    rig.feed(rig.ackMsg(133, 7));
    TEST_ASSERT_EQUAL_UINT32(137, rig.engine->status().cursor);
    TEST_ASSERT_EQUAL_UINT32(133, rig.engine->status().ackLocal);
}

// 7. Rewind
void test_app_error_rewinds_cursor_and_next_request_restarts_at_ack() {
    Rig rig;
    fill(rig.store, 65, 140);
    rig.start();
    rig.openSession(130);
    rig.request(5);
    rig.engine->sendNextBlock();
    rig.engine->sendNextBlock();
    const size_t notices = rig.link.ofType(0x07).size();

    rig.feed(tlv(0x06, {0x04}));
    TEST_ASSERT_FALSE(rig.engine->batchActive());
    TEST_ASSERT_EQUAL_UINT32(131, rig.engine->status().cursor);
    TEST_ASSERT_EQUAL_UINT32(notices + 1, rig.link.ofType(0x07).size());  // reason=resync
    TEST_ASSERT_TRUE(sink.of("SYNC_NOTICE").back().find("reason=resync") != std::string::npos);

    // 0x07 seguido de pedido na mesma rajada: o lote novo começa em ackLocal + 1.
    rig.engine->sendNextBlock();
    rig.feed(tlv(0x06, {0x07}));
    rig.request(2);
    const auto blocks = rig.drain();
    TEST_ASSERT_EQUAL_UINT32(131, u32(blocks.at(0), 3));
}

// 8. CRC / LOSS
void test_corrupt_blocks_become_one_loss_with_valid_tag() {
    Rig rig;
    fill(rig.store, 65, 140);
    rig.start();
    rig.openSession(130);
    rig.store.corrupt(133);
    rig.store.corrupt(134);
    rig.request(10);
    auto blocks = rig.drain();
    TEST_ASSERT_EQUAL_UINT32(131, u32(blocks[0], 3));
    TEST_ASSERT_EQUAL_UINT32(132, u32(blocks[1], 3));
    const auto& loss = blocks[2];
    TEST_ASSERT_EQUAL_UINT32(133, u32(loss, 3));
    TEST_ASSERT_EQUAL_UINT16(23 + 32, collector::getU16(&loss[1]));
    TEST_ASSERT_EQUAL_HEX8(0xFD, loss[3 + 12]);
    TEST_ASSERT_EQUAL_UINT32(133, u32(loss, 3 + 15));
    TEST_ASSERT_EQUAL_UINT32(134, u32(loss, 3 + 19));
    TEST_ASSERT_TRUE(rig.blockAuth.verify(&loss[3], 23, &loss[3 + 23]));
    TEST_ASSERT_EQUAL_UINT32(135, u32(blocks[3], 3));

    // Reenvio após rewind gera o mesmo LOSS.
    rig.feed(tlv(0x06, {0x04}));
    rig.request(10);
    const auto again = rig.drain();
    TEST_ASSERT_EQUAL_HEX8_ARRAY(loss.data(), again[2].data(), loss.size());
}

// 9. Lacuna de boot
void test_boot_gap_jumps_to_boot_in_same_batch() {
    Rig rig;
    fill(rig.store, 65, 130);
    rig.store.blocks.push_back(makeBoot(192, 131, 191));
    rig.start();
    rig.openSession(130);
    rig.request(1);
    const auto blocks = rig.drain();
    TEST_ASSERT_EQUAL_UINT32(1, blocks.size());
    TEST_ASSERT_EQUAL_UINT32(192, u32(blocks[0], 3));
    TEST_ASSERT_EQUAL_HEX8(0xFE, blocks[0][3 + 12]);
    TEST_ASSERT_EQUAL_UINT32(193, rig.engine->status().cursor);
    TEST_ASSERT_TRUE(sink.of("SYNC_CURSOR").back().find("reason=") != std::string::npos);
}

void test_missing_range_without_boot_becomes_loss() {
    Rig rig;
    fill(rig.store, 65, 130);
    rig.store.blocks.push_back(makeBlock(192));
    rig.start();
    rig.openSession(130);
    rig.request(100);
    const auto blocks = rig.drain();
    TEST_ASSERT_EQUAL_UINT32(2, blocks.size());
    TEST_ASSERT_EQUAL_HEX8(0xFD, blocks[0][3 + 12]);
    TEST_ASSERT_EQUAL_UINT32(131, u32(blocks[0], 3 + 15));
    TEST_ASSERT_EQUAL_UINT32(191, u32(blocks[0], 3 + 19));
    TEST_ASSERT_EQUAL_UINT32(192, u32(blocks[1], 3));
}

void test_corrupt_boot_is_not_a_marker() {
    Rig rig;
    fill(rig.store, 65, 130);
    rig.store.blocks.push_back(makeBoot(192, 131, 191));
    rig.store.corrupt(192);
    rig.start();
    rig.openSession(130);
    // O TAIL efetivo não usa o from_seq do BOOT corrompido.
    TEST_ASSERT_EQUAL_UINT32(192, u32(rig.link.ofType(0x02).at(0), 23));
    rig.request(100);
    const auto blocks = rig.drain();
    TEST_ASSERT_EQUAL_UINT32(2, blocks.size());
    TEST_ASSERT_EQUAL_UINT32(131, u32(blocks[0], 3));
    TEST_ASSERT_EQUAL_UINT32(191, u32(blocks[0], 3 + 19));
    TEST_ASSERT_EQUAL_UINT32(192, u32(blocks[1], 3));  // LOSS [192, 192], nunca LOSS com seq 192 antes
    TEST_ASSERT_EQUAL_UINT32(192, u32(blocks[1], 3 + 19));
}

// 10. LOSS longo
void test_long_missing_range_splits_in_runs_of_64() {
    Rig rig;
    fill(rig.store, 65, 130);
    rig.store.blocks.push_back(makeBlock(281));
    rig.start();
    rig.openSession(130);
    rig.request(500);
    const auto blocks = rig.drain();
    TEST_ASSERT_EQUAL_UINT32(4, blocks.size());
    TEST_ASSERT_EQUAL_UINT32(194, u32(blocks[0], 3 + 19));
    TEST_ASSERT_EQUAL_UINT32(195, u32(blocks[1], 3 + 15));
    TEST_ASSERT_EQUAL_UINT32(258, u32(blocks[1], 3 + 19));
    TEST_ASSERT_EQUAL_UINT32(259, u32(blocks[2], 3 + 15));
    TEST_ASSERT_EQUAL_UINT32(280, u32(blocks[2], 3 + 19));
    TEST_ASSERT_EQUAL_UINT32(281, u32(blocks[3], 3));
}

// 11. Flash cheia
void test_overflow_above_ack_closes_session_and_handshake_recovers() {
    Rig rig;
    fill(rig.store, 65, 140);
    rig.start();
    rig.openSession(130);
    TEST_ASSERT_TRUE(rig.engine->status().sessionOpen);

    rig.store.overflow = 135;  // descartou bloco não confirmado
    rig.store.deleteUpTo(135);
    rig.engine->onFlushed(141);
    TEST_ASSERT_FALSE(rig.engine->status().sessionOpen);
    TEST_ASSERT_EQUAL_HEX8(0x01, rig.lastNotice()[7] & 0x01);
    TEST_ASSERT_EQUAL_UINT32(1, sink.of("SYNC_OVERFLOW").size());

    rig.request(5);
    TEST_ASSERT_EQUAL_HEX8(0x05, rig.link.ofType(0x06).back()[3]);

    rig.feed(rig.challenge(rig.nextCs, 130, 7));  // app devolve ACK <= ackLocal
    const auto r = rig.link.ofType(0x02).back();
    TEST_ASSERT_EQUAL_UINT32(136, u32(r, 23));  // novo TAIL
    rig.feed(rig.ackMsg(135, rig.nextCs));
    TEST_ASSERT_TRUE(rig.engine->status().sessionOpen);
    TEST_ASSERT_EQUAL_UINT32(136, rig.engine->status().cursor);
}

// 12. Flash > 80 %
void test_flash_high_notice_with_hysteresis() {
    Rig rig;
    fill(rig.store, 65, 140);
    rig.start();
    rig.openSession(130);
    rig.store.usage = 801;
    rig.engine->tick();
    TEST_ASSERT_EQUAL_HEX8(0x04, rig.lastNotice()[7]);
    const size_t n = rig.link.ofType(0x07).size();
    rig.store.usage = 790;
    rig.engine->tick();
    TEST_ASSERT_EQUAL_UINT32(n, rig.link.ofType(0x07).size());
    rig.store.usage = 779;
    rig.engine->tick();
    TEST_ASSERT_EQUAL_HEX8(0x00, rig.lastNotice()[7]);
}

// 13. Reenvio do aviso sem sessão
void test_notice_repeats_every_15_seconds_without_session() {
    Rig rig;
    fill(rig.store, 65, 130);
    rig.start();
    const size_t n = rig.link.ofType(0x07).size();
    rig.clock.advance(14000);
    rig.engine->tick();
    TEST_ASSERT_EQUAL_UINT32(n, rig.link.ofType(0x07).size());
    rig.clock.advance(1000);
    rig.engine->tick();
    TEST_ASSERT_EQUAL_UINT32(n + 1, rig.link.ofType(0x07).size());
    TEST_ASSERT_TRUE(sink.of("SYNC_NOTICE").back().find("reason=retry") != std::string::npos);
}

// 14. Link caindo no meio
void test_link_failure_does_not_advance_cursor_and_ends_batch() {
    Rig rig;
    fill(rig.store, 65, 140);
    rig.start();
    rig.openSession(130);
    rig.request(5);
    rig.engine->sendNextBlock();
    rig.engine->sendNextBlock();
    rig.link.failOnSend = static_cast<int>(rig.link.attempts);  // o próximo envio falha
    rig.engine->sendNextBlock();
    TEST_ASSERT_FALSE(rig.engine->batchActive());
    TEST_ASSERT_EQUAL_UINT32(133, rig.engine->status().cursor);
}

// 15. Perfis
void test_blocks_and_loss_follow_tag_size_of_each_profile() {
    const int profiles[] = {0, 2};
    for (int profile : profiles) {
        const uint8_t tagLen = profile == 0 ? 0 : 64;
        Rig rig(profile);
        fill(rig.store, 65, 140, tagLen);
        rig.start();
        rig.openSession(130);
        rig.store.corrupt(133);
        rig.request(5);
        const auto blocks = rig.drain();
        TEST_ASSERT_EQUAL_UINT32(5, blocks.size());
        TEST_ASSERT_EQUAL_UINT16(17 + tagLen, collector::getU16(&blocks[0][1]));
        TEST_ASSERT_EQUAL_UINT16(23 + tagLen, collector::getU16(&blocks[2][1]));  // LOSS [133,133]
        TEST_ASSERT_EQUAL_UINT32(133, u32(blocks[2], 3));
    }
}

// 16. TAIL efetivo
void test_effective_tail_counts_the_boot_range() {
    Rig rig;
    fill(rig.store, 65, 130);
    rig.store.blocks.push_back(makeBoot(192, 131, 191));
    rig.start();
    rig.feed(rig.challenge(7, 130, 6));
    TEST_ASSERT_EQUAL_UINT32(131, u32(rig.link.ofType(0x02).at(0), 23));  // não 192

    // Só o BOOT desde o boot: ackLocal inicial = from_seq - 1.
    Rig fresh;
    fresh.store.blocks.push_back(makeBoot(192, 131, 191));
    fresh.engine->begin();
    TEST_ASSERT_EQUAL_UINT32(130, fresh.engine->status().ackLocal);
}

// 17. Eventos
void test_event_sequence_and_microsecond_fields() {
    Rig rig;
    fill(rig.store, 65, 140);
    rig.start();
    rig.feed(rig.challenge(7, 130, 6));
    rig.feed(rig.ackMsg(130, 7));
    rig.request(1);
    rig.drain();

    const auto chal = sink.of("HS_CHALLENGE");
    TEST_ASSERT_EQUAL_UINT32(1, chal.size());
    TEST_ASSERT_TRUE(chal[0].find("c_s=7 last_cs=6 ack=130 c_conf=6 flags=00 diag_min=- len=45") != std::string::npos);
    TEST_ASSERT_TRUE(chal[0].find("result=ok") != std::string::npos);
    TEST_ASSERT_EQUAL_UINT32(1, sink.of("HS_RESPONSE").size());
    TEST_ASSERT_TRUE(sink.of("HS_ACK")[0].find("opened=1") != std::string::npos);
    const auto block = sink.of("SYNC_BLOCK");
    TEST_ASSERT_EQUAL_UINT32(1, block.size());
    TEST_ASSERT_TRUE(block[0].find("seq=131 kind=data len=17 tag_len=32") != std::string::npos);
    TEST_ASSERT_TRUE(block[0].find("tx_us=1100") != std::string::npos);
    TEST_ASSERT_TRUE(block[0].find("crc_ok=1") != std::string::npos);

    // Ordem: HS_CHALLENGE antes do HS_RESPONSE; HS_ACK antes do SYNC_SESSION open=1.
    size_t iChal = 0, iResp = 0, iAck = 0, iSession = 0;
    for (size_t i = 0; i < sink.lines.size(); ++i) {
        const std::string& l = sink.lines[i];
        if (l.rfind("HS_CHALLENGE", 0) == 0) iChal = i;
        if (l.rfind("HS_RESPONSE", 0) == 0) iResp = i;
        if (l.rfind("HS_ACK", 0) == 0) iAck = i;
        if (l.rfind("SYNC_SESSION open=1", 0) == 0) iSession = i;
    }
    TEST_ASSERT_TRUE(iChal < iResp);
    TEST_ASSERT_TRUE(iAck < iSession);
}

int main(int, char**) {
    UNITY_BEGIN();
    RUN_TEST(test_subscribe_sends_notice_with_both_handshake_bits);
    RUN_TEST(test_happy_handshake_saves_cs_before_reply_and_frees_flash);
    RUN_TEST(test_response_matches_spec_vector_when_nothing_is_freed);
    RUN_TEST(test_ack_opens_session_and_clears_notice_flags);
    RUN_TEST(test_ack_with_anchor_failed_keeps_boot_bit);
    RUN_TEST(test_errors_for_bad_tag_stale_cs_bad_ack_and_no_session);
    RUN_TEST(test_nvs_failure_sends_no_response);
    RUN_TEST(test_batch_requests_send_in_order_with_tag_length);
    RUN_TEST(test_cursor_does_not_wait_for_ack_and_ack_does_not_rewind);
    RUN_TEST(test_app_error_rewinds_cursor_and_next_request_restarts_at_ack);
    RUN_TEST(test_corrupt_blocks_become_one_loss_with_valid_tag);
    RUN_TEST(test_boot_gap_jumps_to_boot_in_same_batch);
    RUN_TEST(test_missing_range_without_boot_becomes_loss);
    RUN_TEST(test_corrupt_boot_is_not_a_marker);
    RUN_TEST(test_long_missing_range_splits_in_runs_of_64);
    RUN_TEST(test_overflow_above_ack_closes_session_and_handshake_recovers);
    RUN_TEST(test_flash_high_notice_with_hysteresis);
    RUN_TEST(test_notice_repeats_every_15_seconds_without_session);
    RUN_TEST(test_link_failure_does_not_advance_cursor_and_ends_batch);
    RUN_TEST(test_blocks_and_loss_follow_tag_size_of_each_profile);
    RUN_TEST(test_effective_tail_counts_the_boot_range);
    RUN_TEST(test_event_sequence_and_microsecond_fields);
    return UNITY_END();
}
