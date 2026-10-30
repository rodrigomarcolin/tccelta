#include <unity.h>

#include <cstdlib>
#include <set>
#include <vector>

#include "collector/BlockWriter.h"
#include "collector/MarkerBlock.h"
#include "collector/SeqReserve.h"
#include "support/Fakes.h"

using namespace collector;

void setUp() {}
void tearDown() {}

static void expectRecovery(uint32_t rsv, uint32_t legacy, uint32_t newest, uint32_t boot, uint32_t from,
                           uint32_t to, bool migrated) {
    const SeqRecovery r = SeqReserve::recover(rsv, legacy, newest);
    TEST_ASSERT_EQUAL_UINT32(boot, r.bootSeq);
    TEST_ASSERT_EQUAL_UINT32(from, r.fromSeq);
    TEST_ASSERT_EQUAL_UINT32(to, r.toSeq);
    TEST_ASSERT_EQUAL(migrated, r.migrated);
}

void test_recovery_table() {
    expectRecovery(0, 0, 0, 1, 1, 0, true);             // primeiro boot
    expectRecovery(192, 0, 130, 192, 131, 191, false);  // reboot normal
    expectRecovery(131, 0, 130, 131, 131, 130, false);  // reboot sem furo
    expectRecovery(0, 127, 127, 128, 128, 127, true);   // migração v1 com log
    expectRecovery(0, 500, 0, 501, 501, 500, true);     // migração v1 sem log
    expectRecovery(100, 0, 150, 151, 151, 150, false);  // reserva inconsistente
}

// ── BlockWriter com reserva (N = 4) ──────────────────────────────────────

class OrderedPersistence : public ISeqReservePersistence {
public:
    bool saveSeqReserve(uint32_t reserve) override {
        if (fail) return false;
        saved.push_back(reserve);
        events.push_back(-static_cast<int>(reserve));  // negativo = NVS
        return true;
    }
    std::vector<uint32_t> saved;
    std::vector<int> events;
    bool fail = false;
};

class OrderedStore : public MemoryBlockStore {
public:
    explicit OrderedStore(OrderedPersistence& p) : _p(p) {}
    bool append(const Block& b) override {
        const bool ok = MemoryBlockStore::append(b);
        if (ok) _p.events.push_back(static_cast<int>(blockSeq(b)));
        return ok;
    }

private:
    OrderedPersistence& _p;
};

static Block dataBlock() {
    Block b = {};
    b.length = kHeaderSize + kPidRecordSize;
    putU16(&b.bytes[header::kLength], static_cast<uint16_t>(b.length));
    return b;
}

void test_reserve_is_written_before_the_block_that_consumes_it() {
    OrderedPersistence nvs;
    OrderedStore store(nvs);
    FakeAuth auth(0);
    FakeClock clock;
    const SeqRecovery rec = SeqReserve::recover(10, 0, 8);  // R = 10, [9, 9]
    BlockWriter writer(store, nvs, auth, clock, SeqReserve::afterRecovery(rec, 4), 2);

    WriteReport report;
    TEST_ASSERT_TRUE(writer.writeBootMarker(rec.fromSeq, rec.toSeq, report));
    TEST_ASSERT_TRUE(report.reserved);
    for (int i = 0; i < 4; ++i) {
        Block b = dataBlock();
        TEST_ASSERT_TRUE(writer.write(b, report));
    }
    // BOOT 10 grava 14; 11..13 sem NVS; 14 grava 18, sempre antes do append do seq que a consome.
    const std::vector<int> expected = {-14, 10, 11, 12, 13, -18, 14};
    TEST_ASSERT_EQUAL_UINT32(expected.size(), nvs.events.size());
    for (size_t i = 0; i < expected.size(); ++i) TEST_ASSERT_EQUAL_INT(expected[i], nvs.events[i]);
}

void test_reserve_failure_does_not_write_or_consume_seq() {
    OrderedPersistence nvs;
    OrderedStore store(nvs);
    FakeAuth auth(0);
    FakeClock clock;
    BlockWriter writer(store, nvs, auth, clock, SeqReserve(5, 0, 4), 1);
    nvs.fail = true;
    Block b = dataBlock();
    WriteReport report;
    TEST_ASSERT_FALSE(writer.write(b, report));
    TEST_ASSERT_EQUAL_UINT32(0, store.blocks.size());
    TEST_ASSERT_EQUAL_UINT32(5, writer.nextSeq());
    TEST_ASSERT_EQUAL_UINT32(1, writer.failedBlocks());
}

// ── Propriedade: R1, R2 e R3 com quedas de energia simuladas ─────────────

namespace {

// Flash e NVS "físicas": sobrevivem ao writer, que é recriado a cada boot (a RAM se perde).
struct CrashStore : MemoryBlockStore {
    std::vector<Block>* flash = nullptr;
    int crashAfter = -1;  // número de gravações até "cair" no meio de um append
    bool append(const Block& b) override {
        if (crashAfter == 0) {
            crashAfter = -1;
            return false;  // registro parcial descartado pela varredura
        }
        if (crashAfter > 0) --crashAfter;
        flash->push_back(b);
        return true;
    }
};

struct CrashNvs : ISeqReservePersistence {
    uint32_t* target = nullptr;
    bool saveSeqReserve(uint32_t r) override {
        *target = r;
        return true;
    }
};

}  // namespace

void test_property_no_repeated_seq_and_no_undeclared_gap() {
    srand(12345);
    uint32_t nvsReserve = 0;
    std::vector<Block> flash;

    for (int boot = 1; boot <= 400; ++boot) {
        const uint32_t newest = flash.empty() ? 0 : blockSeq(flash.back());
        const SeqRecovery rec = SeqReserve::recover(nvsReserve, 0, newest);
        if (nvsReserve > 0) TEST_ASSERT_TRUE(newest < nvsReserve);  // R1

        CrashStore store;
        store.flash = &flash;
        CrashNvs nvs;
        nvs.target = &nvsReserve;
        FakeAuth auth(0);
        FakeClock clock;
        BlockWriter writer(store, nvs, auth, clock, SeqReserve::afterRecovery(rec, 4), 1);
        WriteReport report;
        writer.writeBootMarker(rec.fromSeq, rec.toSeq, report);
        const int blocks = rand() % 30;
        store.crashAfter = rand() % 3 == 0 ? rand() % (blocks + 1) : -1;
        for (int i = 0; i < blocks; ++i) {
            Block b = dataBlock();
            writer.write(b, report);
        }
    }

    // R2: seq estritamente crescente na flash, sem repetição.
    for (size_t i = 1; i < flash.size(); ++i) TEST_ASSERT_TRUE(blockSeq(flash[i]) > blockSeq(flash[i - 1]));

    // R3: a união dos seq com as faixas dos BOOT cobre 1 até o último.
    std::set<uint32_t> covered;
    for (const Block& b : flash) {
        covered.insert(blockSeq(b));
        const MarkerInfo m = parseMarker(b);
        if (m.kind == MarkerKind::Boot) {
            for (uint32_t s = m.fromSeq; m.toSeq >= m.fromSeq && s <= m.toSeq; ++s) covered.insert(s);
        }
    }
    for (uint32_t s = 1; s <= blockSeq(flash.back()); ++s) TEST_ASSERT_TRUE(covered.count(s) == 1);
}

int main(int, char**) {
    UNITY_BEGIN();
    RUN_TEST(test_recovery_table);
    RUN_TEST(test_reserve_is_written_before_the_block_that_consumes_it);
    RUN_TEST(test_reserve_failure_does_not_write_or_consume_seq);
    RUN_TEST(test_property_no_repeated_seq_and_no_undeclared_gap);
    return UNITY_END();
}
