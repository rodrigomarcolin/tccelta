#include <unity.h>
#include "collector/AtHashParser.h"
#include "collector/BlockWriter.h"
#include "collector/DataProtocol.h"
#include "collector/GpsGate.h"
#include "collector/SeqCounter.h"
#include "collector/TimeSync.h"
#include "collector/TmaxMeter.h"
#include "collector/UploadCursor.h"
#include "support/Fakes.h"

using namespace collector;

void setUp() {}
void tearDown() {}

static Block blockWithOneRecord() {
    Block block = {};
    block.length = kHeaderSize + kPidRecordSize;
    return block;
}

// ── seq e gravação (Flush) ───────────────────────────────────────────────

void test_seq_recovery_uses_last_written_plus_one() {
    TEST_ASSERT_EQUAL_UINT32(1, SeqCounter::recover(0, 0).peek());
    TEST_ASSERT_EQUAL_UINT32(42, SeqCounter::recover(41, 40).peek());
    // Corte de energia entre o append e a NVS: o log está à frente.
    TEST_ASSERT_EQUAL_UINT32(42, SeqCounter::recover(40, 41).peek());
}

void test_writer_stamps_header_and_persists_after_append() {
    MemoryBlockStore store;
    FakeSeqPersistence nvs;
    BlockWriter writer(store, nvs, SeqCounter(7), 3);

    Block block = blockWithOneRecord();
    TEST_ASSERT_TRUE(writer.write(block));
    TEST_ASSERT_EQUAL_UINT32(7, blockSeq(store.blocks[0]));
    TEST_ASSERT_EQUAL_UINT16(3, getU16(&store.blocks[0].bytes[header::kBootId]));
    TEST_ASSERT_EQUAL_UINT32(1, nvs.saved.size());
    TEST_ASSERT_EQUAL_UINT32(7, nvs.saved[0]);
    TEST_ASSERT_EQUAL_UINT32(8, writer.nextSeq());
}

void test_writer_retries_once_then_drops_without_consuming_seq() {
    MemoryBlockStore store;
    FakeSeqPersistence nvs;
    BlockWriter writer(store, nvs, SeqCounter(7), 3);

    Block retried = blockWithOneRecord();
    store.failAppends = 1;
    TEST_ASSERT_TRUE(writer.write(retried));
    TEST_ASSERT_EQUAL_UINT32(8, writer.nextSeq());

    Block dropped = blockWithOneRecord();
    store.failAppends = 2;
    TEST_ASSERT_FALSE(writer.write(dropped));
    TEST_ASSERT_EQUAL_UINT32(8, writer.nextSeq());     // seq não consumido
    TEST_ASSERT_EQUAL_UINT32(1, writer.failedBlocks());

    Block next = blockWithOneRecord();
    TEST_ASSERT_TRUE(writer.write(next));
    TEST_ASSERT_EQUAL_UINT32(8, blockSeq(store.blocks.back()));  // sem lacuna
}

// ── Cursor de upload ─────────────────────────────────────────────────────

void test_cursor_respects_window_and_ack() {
    UploadCursor cursor(2, 5000);
    cursor.resync(1);
    TEST_ASSERT_TRUE(cursor.canSend(10));
    TEST_ASSERT_EQUAL_UINT32(1, cursor.takeNext(0));
    TEST_ASSERT_EQUAL_UINT32(2, cursor.takeNext(0));
    TEST_ASSERT_FALSE(cursor.canSend(10));  // janela cheia

    TEST_ASSERT_TRUE(cursor.onAck(1, 100));
    TEST_ASSERT_TRUE(cursor.canSend(10));
    TEST_ASSERT_FALSE(cursor.onAck(1, 100));  // ACK repetido é ignorado
}

void test_cursor_skips_to_oldest_when_blocks_were_deleted() {
    UploadCursor cursor;
    cursor.resync(1);
    cursor.takeNext(0);
    cursor.onAck(1, 0);
    cursor.resync(50);  // partição encheu: blocos 2..49 apagados
    TEST_ASSERT_EQUAL_UINT32(49, cursor.ackSeq());
    TEST_ASSERT_EQUAL_UINT32(50, cursor.takeNext(0));
}

void test_cursor_times_out_and_rewinds() {
    UploadCursor cursor(8, 5000);
    cursor.resync(1);
    cursor.takeNext(0);
    cursor.takeNext(0);
    TEST_ASSERT_FALSE(cursor.timedOut(4999));
    TEST_ASSERT_TRUE(cursor.timedOut(5000));
    cursor.rewind();
    TEST_ASSERT_EQUAL_UINT32(0, cursor.inFlight());
    TEST_ASSERT_EQUAL_UINT32(1, cursor.takeNext(5000));
}

void test_cursor_accepts_ack_beyond_what_was_sent() {
    UploadCursor cursor;
    cursor.resync(1);
    cursor.takeNext(0);
    TEST_ASSERT_TRUE(cursor.onAck(30, 0));  // servidor já tinha até 30 antes do reboot
    TEST_ASSERT_EQUAL_UINT32(31, cursor.takeNext(0));
}

// ── Protocolo e hora ─────────────────────────────────────────────────────

void test_batch_roundtrip_through_frame_decoder() {
    NoAuth auth;
    DataProtocol protocol(auth);
    Block block = blockWithOneRecord();
    stampHeader(block, 9, 1);

    uint8_t msg[kMaxMessageSize];
    const size_t len = protocol.encodeBatch(5, block, msg, sizeof(msg));
    uint8_t frame[kMaxMessageSize + kFrameHeaderSize];
    const size_t framed = frameMessage(msg, len, frame, sizeof(frame));

    FrameDecoder<kMaxMessageSize> decoder;
    size_t received = 0;
    for (size_t i = 0; i < framed; i += 7) {  // fragmentos de 7 bytes
        const size_t chunk = framed - i < 7 ? framed - i : 7;
        decoder.push(frame + i, chunk, [&](const uint8_t* m, size_t n) {
            received = n;
            TEST_ASSERT_EQUAL_HEX8(static_cast<uint8_t>(MessageType::Batch), m[0]);
            TEST_ASSERT_EQUAL_UINT32(5, getU32(&m[1]));
            TEST_ASSERT_EQUAL_UINT32(9, getU32(&m[5]));
        });
    }
    TEST_ASSERT_EQUAL_UINT32(len, received);
}

class RejectAll : public IMessageAuth {
public:
    size_t tagSize() const override { return 4; }
    void sign(const uint8_t*, size_t, uint8_t* tag) override { tag[0] = tag[1] = tag[2] = tag[3] = 0; }
    bool verify(const uint8_t*, size_t, const uint8_t*) override { return false; }
};

void test_ack_is_ignored_when_not_authentic() {
    RejectAll auth;
    DataProtocol protocol(auth);
    const uint8_t ack[] = {0x02, 0x05, 0x00, 0x00, 0x00, 0xDE, 0xAD, 0xBE, 0xEF};
    uint32_t seq = 0;
    TEST_ASSERT_FALSE(protocol.decodeAck(ack, sizeof(ack), seq));
}

void test_time_sync_accepts_only_matching_response() {
    TimeSync sync;
    const TimeExchange request = sync.start(3, 10000);
    TEST_ASSERT_FALSE(sync.accept({3, 9999, 0}, 10300));  // não é deste pedido
    TEST_ASSERT_TRUE(sync.accept({3, request.t1Ms, 0}, 10400));
    TEST_ASSERT_EQUAL_UINT32(10000, sync.report().t1Ms);
    TEST_ASSERT_EQUAL_UINT32(10400, sync.report().t2Ms);
    TEST_ASSERT_FALSE(sync.accept({3, request.t1Ms, 0}, 10500));  // já concluído
}

// ── Tmax e AT# ───────────────────────────────────────────────────────────

void test_tmax_meter_p99_and_suggestion() {
    CollectorConfig config;
    config.pids[0] = {0x0C, 250, 125};
    config.pidCount = 1;
    TmaxMeter meter;
    meter.begin(config);
    for (uint32_t i = 1; i <= kTmaxSamplesPerPid; ++i) {
        meter.record(i != 100, i);  // latências 1..99 e um timeout
    }
    const TmaxReport report = meter.report();
    TEST_ASSERT_TRUE(report.state == TmaxReport::State::Done);
    TEST_ASSERT_EQUAL_UINT32(99, report.pids[0].p99Ms);   // posto ⌈0,99·99⌉ = 99
    TEST_ASSERT_EQUAL_UINT32(1, report.pids[0].timeouts);
    TEST_ASSERT_EQUAL_UINT32(132, report.suggestedTmaxMs);  // ⌈1,3·99⌉ = 129 → 132
}

void test_at_hash_parser() {
    AtHashCommand c = parseAtHash("AT#PID 0C 250");
    TEST_ASSERT_TRUE(c.kind == AtHashCommand::Kind::SetPid);
    TEST_ASSERT_EQUAL_HEX8(0x0C, c.pid);
    TEST_ASSERT_EQUAL_UINT32(250, c.periodMs);

    c = parseAtHash("AT#GPS -235505200 -466333100 760000");
    TEST_ASSERT_TRUE(c.kind == AtHashCommand::Kind::Gps);
    TEST_ASSERT_EQUAL_INT32(-235505200, c.latE7);
    TEST_ASSERT_EQUAL_INT32(760000, c.altMm);

    TEST_ASSERT_TRUE(parseAtHash("AT#TMAX?").kind == AtHashCommand::Kind::TmaxQuery);
    TEST_ASSERT_TRUE(parseAtHash("AT#TMAX").kind == AtHashCommand::Kind::Tmax);
    TEST_ASSERT_TRUE(parseAtHash("AT#DIAG 1").flag);

    TEST_ASSERT_TRUE(parseAtHash("AT#DIAG 2").kind == AtHashCommand::Kind::Invalid);
    TEST_ASSERT_TRUE(parseAtHash("AT#PID 0C").kind == AtHashCommand::Kind::Invalid);
    TEST_ASSERT_TRUE(parseAtHash("AT#PID 0C 250 9").kind == AtHashCommand::Kind::Invalid);
    TEST_ASSERT_TRUE(parseAtHash("AT#GPS 950000000 0 0").kind == AtHashCommand::Kind::Invalid);
    TEST_ASSERT_TRUE(parseAtHash("AT#LISTX").kind == AtHashCommand::Kind::Invalid);
}

void test_ack_roundtrip() {
    NoAuth auth;
    DataProtocol protocol(auth);
    uint8_t msg[16];
    const size_t len = protocol.encodeAck(1234, msg, sizeof(msg));
    uint32_t seq = 0;
    TEST_ASSERT_TRUE(protocol.decodeAck(msg, len, seq));
    TEST_ASSERT_EQUAL_UINT32(1234, seq);
}

void test_at_ack_parsing() {
    AtHashCommand c = parseAtHash("AT#ACK 42");
    TEST_ASSERT_TRUE(c.kind == AtHashCommand::Kind::Ack);
    TEST_ASSERT_EQUAL_UINT32(42, c.seq);
    TEST_ASSERT_TRUE(parseAtHash("AT#ACK").kind == AtHashCommand::Kind::Invalid);
    TEST_ASSERT_TRUE(parseAtHash("AT#ACK 0").kind == AtHashCommand::Kind::Invalid);
}

void test_gps_gate_rejects_diag_mode() {
    GpsGate gate;
    TEST_ASSERT_TRUE(gate.check(true, 1000) == GpsGate::Verdict::Diag);
    // Rejeitar no diagnóstico não conta como posição aceita.
    TEST_ASSERT_TRUE(gate.check(false, 1100) == GpsGate::Verdict::Accept);
}

void test_gps_gate_limits_to_one_per_second_with_jitter() {
    GpsGate gate;
    TEST_ASSERT_TRUE(gate.check(false, 1000) == GpsGate::Verdict::Accept);
    TEST_ASSERT_TRUE(gate.check(false, 1500) == GpsGate::Verdict::Rate);
    TEST_ASSERT_TRUE(gate.check(false, 1950) == GpsGate::Verdict::Accept);   // 950 ms: dentro da tolerância
    TEST_ASSERT_TRUE(gate.check(false, 2800) == GpsGate::Verdict::Rate);     // 850 ms após a última aceita
    TEST_ASSERT_TRUE(gate.check(false, 2950) == GpsGate::Verdict::Accept);
}

int main(int, char**) {
    UNITY_BEGIN();
    RUN_TEST(test_seq_recovery_uses_last_written_plus_one);
    RUN_TEST(test_writer_stamps_header_and_persists_after_append);
    RUN_TEST(test_writer_retries_once_then_drops_without_consuming_seq);
    RUN_TEST(test_cursor_respects_window_and_ack);
    RUN_TEST(test_cursor_skips_to_oldest_when_blocks_were_deleted);
    RUN_TEST(test_cursor_times_out_and_rewinds);
    RUN_TEST(test_cursor_accepts_ack_beyond_what_was_sent);
    RUN_TEST(test_batch_roundtrip_through_frame_decoder);
    RUN_TEST(test_ack_is_ignored_when_not_authentic);
    RUN_TEST(test_time_sync_accepts_only_matching_response);
    RUN_TEST(test_tmax_meter_p99_and_suggestion);
    RUN_TEST(test_at_hash_parser);
    RUN_TEST(test_ack_roundtrip);
    RUN_TEST(test_at_ack_parsing);
    RUN_TEST(test_gps_gate_rejects_diag_mode);
    RUN_TEST(test_gps_gate_limits_to_one_per_second_with_jitter);
    return UNITY_END();
}
