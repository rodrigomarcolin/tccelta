#include <unity.h>

#include <cstdlib>
#include <cstring>
#include <vector>

#include "collector/BlockFormat.h"
#include "collector/BlockWriter.h"
#include "collector/MarkerBlock.h"
#include "collector/SegmentFormat.h"
#include "crypto/HmacSha256.h"
#include "security/BlockAuth.h"
#include "security/DeviceKeys.h"
#include "support/Fakes.h"
#include "support/vectors.h"

using namespace collector;

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

static Block blockWithOneRecord() {
    Block block = {};
    block.length = kHeaderSize + kPidRecordSize;
    putU16(&block.bytes[header::kLength], static_cast<uint16_t>(block.length));
    return block;
}

// ── Marcadores (spec 02 §3) ──────────────────────────────────────────────

void test_boot_and_loss_markers_match_vector_bytes() {
    Block boot = {};
    buildBootMarker(boot, 192, 4, 812, 131, 191);
    assertHex(VEC_BLK_BOOT_DATA, boot.bytes, boot.length);
    TEST_ASSERT_EQUAL_UINT32(23, boot.length);

    Block loss = {};
    buildLossMarker(loss, 140, 141);
    assertHex(VEC_BLK_LOSS_DATA, loss.bytes, loss.length);
}

void test_parse_marker() {
    Block boot = {};
    buildBootMarker(boot, 192, 4, 812, 131, 191);
    MarkerInfo info = parseMarker(boot);
    TEST_ASSERT_TRUE(info.kind == MarkerKind::Boot);
    TEST_ASSERT_EQUAL_UINT32(131, info.fromSeq);
    TEST_ASSERT_EQUAL_UINT32(191, info.toSeq);

    Block loss = {};
    buildLossMarker(loss, 7, 9);
    info = parseMarker(loss);
    TEST_ASSERT_TRUE(info.kind == MarkerKind::Loss);
    TEST_ASSERT_EQUAL_UINT32(7, info.fromSeq);

    // Bloco de dados e bloco de 23 B cujo 1º registro é PID não são marcadores.
    Block data = blockWithOneRecord();
    TEST_ASSERT_TRUE(parseMarker(data).kind == MarkerKind::None);
    Block pid23 = boot;
    pid23.bytes[kHeaderSize] = 0x0C;
    TEST_ASSERT_TRUE(parseMarker(pid23).kind == MarkerKind::None);
}

void test_boot_tag_and_crc_vectors() {
    auto key = unhex(VEC_K_REC);
    auto devId = unhex(VEC_DEV_ID);
    crypto::PortableHmacSha256 hmac(key.data(), key.size());
    security::HmacBlockAuth auth(hmac, devId.data());

    Block boot = {};
    buildBootMarker(boot, 192, 4, 812, 131, 191);
    boot.tagLength = 32;
    auth.sign(boot.bytes, boot.length, boot.tag);
    assertHex(VEC_BLK_BOOT_TAG, boot.tag, 32);
    boot.crc = computeCrc(boot);
    TEST_ASSERT_EQUAL_HEX32(0xB294D366, boot.crc);
    TEST_ASSERT_TRUE(crcMatches(boot));

    NoAuth none;
    Block bare = {};
    buildBootMarker(bare, 192, 4, 812, 131, 191);
    bare.tagLength = static_cast<uint8_t>(none.tagSize());
    TEST_ASSERT_EQUAL_HEX32(0x8D81DCB2, computeCrc(bare));
}

// ── Segmento (spec 02 §6, §11) ───────────────────────────────────────────

class MemorySource : public IByteSource {
public:
    explicit MemorySource(const std::vector<uint8_t>& data) : _data(data) {}
    size_t read(uint8_t* out, size_t n) override {
        n = n < remaining() ? n : remaining();
        memcpy(out, _data.data() + _pos, n);
        _pos += n;
        return n;
    }
    bool skip(size_t n) override {
        if (n > remaining()) return false;
        _pos += n;
        return true;
    }
    size_t remaining() const override { return _data.size() - _pos; }

private:
    const std::vector<uint8_t>& _data;
    size_t _pos = 0;
};

static void appendRecord(std::vector<uint8_t>& file, uint32_t seq, uint8_t tagLen, size_t length = 17) {
    Block b = {};
    b.length = length;
    putU32(&b.bytes[header::kSeq], seq);
    putU16(&b.bytes[header::kLength], static_cast<uint16_t>(length));
    for (size_t i = kHeaderSize; i < length; ++i) b.bytes[i] = static_cast<uint8_t>(i);
    b.tagLength = tagLen;
    for (uint8_t i = 0; i < tagLen; ++i) b.tag[i] = static_cast<uint8_t>(0xA0 + i);
    b.crc = computeCrc(b);
    file.insert(file.end(), b.bytes, b.bytes + b.length);
    file.insert(file.end(), b.tag, b.tag + tagLen);
    uint8_t crc[4];
    putU32(crc, b.crc);
    file.insert(file.end(), crc, crc + 4);
}

static std::vector<uint8_t> segment(uint8_t tagLen, uint32_t firstSeq, int count) {
    std::vector<uint8_t> file(kSegmentHeaderSize);
    encodeSegmentHeader(tagLen, file.data());
    for (int i = 0; i < count; ++i) appendRecord(file, firstSeq + i, tagLen);
    return file;
}

void test_segment_header_roundtrip_and_rejections() {
    uint8_t h[kSegmentHeaderSize];
    uint8_t tagLen = 99;
    encodeSegmentHeader(32, h);
    TEST_ASSERT_EQUAL_HEX8(0x54, h[0]);
    TEST_ASSERT_TRUE(decodeSegmentHeader(h, tagLen));
    TEST_ASSERT_EQUAL_UINT8(32, tagLen);

    h[0] = 'X';
    TEST_ASSERT_FALSE(decodeSegmentHeader(h, tagLen));
    encodeSegmentHeader(32, h);
    h[4] = 1;
    TEST_ASSERT_FALSE(decodeSegmentHeader(h, tagLen));
    encodeSegmentHeader(31, h);
    TEST_ASSERT_FALSE(decodeSegmentHeader(h, tagLen));
}

void test_scan_intact_segments_for_every_tag_size() {
    const uint8_t sizes[] = {0, 32, 64};
    for (uint8_t tagLen : sizes) {
        auto file = segment(tagLen, 10, 3);
        MemorySource src(file);
        ScanResult r;
        TEST_ASSERT_TRUE(scanSegment(src, 10, r));
        TEST_ASSERT_EQUAL_UINT16(3, r.count);
        TEST_ASSERT_EQUAL_UINT32(12, r.lastSeq);
        TEST_ASSERT_EQUAL_UINT8(tagLen, r.tagLen);
        TEST_ASSERT_EQUAL_UINT32(file.size(), r.bytes);
        TEST_ASSERT_FALSE(r.trailingGarbage);
    }
}

void test_scan_truncated_tail_stops_with_trailing_garbage() {
    auto file = segment(32, 10, 3);
    file.resize(file.size() - 20);  // corta no meio da tag do 3º registro
    MemorySource src(file);
    ScanResult r;
    TEST_ASSERT_TRUE(scanSegment(src, 10, r));
    TEST_ASSERT_EQUAL_UINT16(2, r.count);
    TEST_ASSERT_TRUE(r.trailingGarbage);
}

void test_scan_stops_on_seq_out_of_order_and_bad_length() {
    auto file = segment(0, 10, 2);
    appendRecord(file, 99, 0);  // seq fora de ordem
    MemorySource src(file);
    ScanResult r;
    TEST_ASSERT_TRUE(scanSegment(src, 10, r));
    TEST_ASSERT_EQUAL_UINT16(2, r.count);
    TEST_ASSERT_TRUE(r.trailingGarbage);

    const size_t bad[] = {11, 1025};
    for (size_t len : bad) {
        auto f = segment(0, 10, 1);
        Block b = {};
        putU32(&b.bytes[header::kSeq], 11);
        putU16(&b.bytes[header::kLength], static_cast<uint16_t>(len));
        f.insert(f.end(), b.bytes, b.bytes + 12);
        f.resize(f.size() + 2000, 0);
        MemorySource s2(f);
        TEST_ASSERT_TRUE(scanSegment(s2, 10, r));
        TEST_ASSERT_EQUAL_UINT16(1, r.count);
    }
}

void test_scan_header_only_and_wrong_first_seq() {
    auto file = segment(32, 10, 0);
    MemorySource src(file);
    ScanResult r;
    TEST_ASSERT_TRUE(scanSegment(src, 10, r));
    TEST_ASSERT_EQUAL_UINT16(0, r.count);

    auto other = segment(32, 10, 2);
    MemorySource src2(other);
    TEST_ASSERT_TRUE(scanSegment(src2, 11, r));  // nome do arquivo não bate com o 1º seq
    TEST_ASSERT_EQUAL_UINT16(0, r.count);
}

void test_read_record_first_middle_last_and_beyond() {
    auto file = segment(32, 10, 4);
    Block out = {};
    const uint32_t seqs[] = {10, 12, 13};
    for (uint32_t seq : seqs) {
        MemorySource src(file);
        TEST_ASSERT_TRUE(readRecord(src, 10, seq, out) == ReadStatus::Ok);
        TEST_ASSERT_EQUAL_UINT32(seq, blockSeq(out));
        TEST_ASSERT_EQUAL_UINT8(32, out.tagLength);
        TEST_ASSERT_TRUE(crcMatches(out));
    }
    MemorySource src(file);
    TEST_ASSERT_TRUE(readRecord(src, 10, 14, out) == ReadStatus::Corrupt);
}

// ── BlockWriter (spec 02 §8) ─────────────────────────────────────────────

struct WriterRig {
    MemoryBlockStore store;
    FakeSeqReservePersistence nvs;
    FakeAuth auth{32};
    FakeClock clock;
    BlockWriter writer;
    explicit WriterRig(uint32_t next = 7, uint32_t reserve = 0, uint32_t step = 4)
        : writer(store, nvs, auth, clock, SeqReserve(next, reserve, step), 3) {}
};

void test_writer_stamps_signs_and_crcs_exact_length() {
    WriterRig rig;
    Block block = blockWithOneRecord();
    WriteReport report;
    TEST_ASSERT_TRUE(rig.writer.write(block, report));
    TEST_ASSERT_EQUAL_UINT32(7, blockSeq(rig.store.blocks[0]));
    TEST_ASSERT_EQUAL_UINT16(3, getU16(&rig.store.blocks[0].bytes[header::kBootId]));
    TEST_ASSERT_EQUAL_UINT32(1, rig.auth.signedLengths.size());
    TEST_ASSERT_EQUAL_UINT32(17, rig.auth.signedLengths[0]);  // `tamanho`, não 1024
    TEST_ASSERT_EQUAL_UINT8(32, rig.store.blocks[0].tagLength);
    TEST_ASSERT_TRUE(crcMatches(rig.store.blocks[0]));
    TEST_ASSERT_EQUAL_UINT32(8, rig.writer.nextSeq());
    TEST_ASSERT_TRUE(report.ok);
    TEST_ASSERT_TRUE(report.reserved);
}

void test_writer_retry_writes_same_bytes_and_double_failure_keeps_seq() {
    WriterRig rig;
    Block retried = blockWithOneRecord();
    WriteReport report;
    rig.store.failAppends = 1;
    TEST_ASSERT_TRUE(rig.writer.write(retried, report));
    TEST_ASSERT_EQUAL_INT(2, report.attempts);
    TEST_ASSERT_EQUAL_UINT32(1, rig.auth.signedLengths.size());  // assina uma vez só
    TEST_ASSERT_EQUAL_INT(1, rig.store.newSegments);             // 2ª tentativa em segmento novo
    TEST_ASSERT_EQUAL_UINT32(8, rig.writer.nextSeq());

    Block dropped = blockWithOneRecord();
    rig.store.failAppends = 2;
    TEST_ASSERT_FALSE(rig.writer.write(dropped, report));
    TEST_ASSERT_EQUAL_UINT32(8, rig.writer.nextSeq());
    TEST_ASSERT_EQUAL_UINT32(1, rig.writer.failedBlocks());

    Block next = blockWithOneRecord();
    TEST_ASSERT_TRUE(rig.writer.write(next, report));
    TEST_ASSERT_EQUAL_UINT32(8, blockSeq(rig.store.blocks.back()));  // sem lacuna
}

void test_writer_boot_pending_blocks_data_until_written() {
    WriterRig rig(10, 0, 4);
    WriteReport report;
    rig.store.failAppends = 2;
    TEST_ASSERT_FALSE(rig.writer.writeBootMarker(5, 9, report));
    TEST_ASSERT_TRUE(rig.writer.bootPending());

    Block data = blockWithOneRecord();
    rig.store.failAppends = 2;
    TEST_ASSERT_FALSE(rig.writer.write(data, report));  // BOOT ainda falha: o dado também
    TEST_ASSERT_EQUAL_UINT32(0, rig.store.blocks.size());

    TEST_ASSERT_TRUE(rig.writer.write(data, report));  // BOOT sai antes do dado
    TEST_ASSERT_FALSE(rig.writer.bootPending());
    TEST_ASSERT_EQUAL_UINT32(2, rig.store.blocks.size());
    TEST_ASSERT_TRUE(parseMarker(rig.store.blocks[0]).kind == MarkerKind::Boot);
    TEST_ASSERT_EQUAL_UINT32(10, blockSeq(rig.store.blocks[0]));
    TEST_ASSERT_EQUAL_UINT32(11, blockSeq(rig.store.blocks[1]));
}

struct SlowAuth : FakeAuth {
    FakeClock* clock = nullptr;
    void sign(const uint8_t* d, size_t l, uint8_t* t) override {
        FakeAuth::sign(d, l, t);
        clock->advanceUs(84);
    }
};

void test_writer_reports_durations_from_clock() {
    MemoryBlockStore store;
    FakeSeqReservePersistence nvs;
    FakeClock clock;
    SlowAuth auth;
    auth.clock = &clock;
    BlockWriter writer(store, nvs, auth, clock, SeqReserve(1, 0, 4), 1);
    Block block = blockWithOneRecord();
    WriteReport report;
    TEST_ASSERT_TRUE(writer.write(block, report));
    TEST_ASSERT_EQUAL_UINT32(84, report.signUs);
}

int main(int, char**) {
    UNITY_BEGIN();
    RUN_TEST(test_boot_and_loss_markers_match_vector_bytes);
    RUN_TEST(test_parse_marker);
    RUN_TEST(test_boot_tag_and_crc_vectors);
    RUN_TEST(test_segment_header_roundtrip_and_rejections);
    RUN_TEST(test_scan_intact_segments_for_every_tag_size);
    RUN_TEST(test_scan_truncated_tail_stops_with_trailing_garbage);
    RUN_TEST(test_scan_stops_on_seq_out_of_order_and_bad_length);
    RUN_TEST(test_scan_header_only_and_wrong_first_seq);
    RUN_TEST(test_read_record_first_middle_last_and_beyond);
    RUN_TEST(test_writer_stamps_signs_and_crcs_exact_length);
    RUN_TEST(test_writer_retry_writes_same_bytes_and_double_failure_keeps_seq);
    RUN_TEST(test_writer_boot_pending_blocks_data_until_written);
    RUN_TEST(test_writer_reports_durations_from_clock);
    return UNITY_END();
}
