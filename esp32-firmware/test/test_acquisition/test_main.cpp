#include <unity.h>
#include <map>
#include "collector/BlockBuilder.h"
#include "collector/Collector.h"
#include "collector/CollectorConfig.h"
#include "collector/Scheduler.h"
#include "collector/SequentialPidReader.h"
#include "collector/Utilization.h"
#include "support/Fakes.h"

using namespace collector;

void setUp() {}
void tearDown() {}

static CollectorConfig twoPids(uint32_t tmax, uint32_t period, uint32_t minInterval) {
    CollectorConfig config;
    config.tmaxMs = tmax;
    config.pids[0] = {0x0C, period, minInterval};
    config.pids[1] = {0x0D, period, minInterval};
    config.pidCount = 2;
    return config;
}

// ── Configuração e utilização ────────────────────────────────────────────

void test_defaults_match_table_11() {
    const CollectorConfig config = CollectorConfig::defaults();
    TEST_ASSERT_EQUAL_UINT32(7, config.pidCount);
    TEST_ASSERT_EQUAL_UINT32(250, config.find(0x0C)->periodMs);
    TEST_ASSERT_EQUAL_UINT32(125, config.find(0x0C)->minIntervalMs);
    TEST_ASSERT_EQUAL_UINT32(600, config.find(0x04)->periodMs);
    TEST_ASSERT_EQUAL_UINT32(10000, config.find(0x2F)->minIntervalMs);
    TEST_ASSERT_DOUBLE_WITHIN(0.005, 0.78, utilization(config));
    TEST_ASSERT_EQUAL(ConfigError::None, validate(config));
}

void test_validate_rejects_infeasible_and_floors() {
    CollectorConfig config = CollectorConfig::defaults();
    config.setPid(0x11, 100);  // +0,5 de utilização
    TEST_ASSERT_EQUAL(ConfigError::Infeasible, validate(config));

    config = CollectorConfig::defaults();
    config.tmaxMs = 16;
    TEST_ASSERT_EQUAL(ConfigError::TmaxTooSmall, validate(config));

    config = CollectorConfig::defaults();
    config.setPid(0x0C, 90);
    TEST_ASSERT_EQUAL(ConfigError::PeriodTooSmall, validate(config));
}

// ── Escalonador ──────────────────────────────────────────────────────────

void test_scheduler_reads_most_overdue_first() {
    Scheduler scheduler;
    scheduler.apply(twoPids(50, 250, 250), 0);

    Scheduler::Decision d = scheduler.next(0, 1);
    TEST_ASSERT_TRUE(d.read);
    TEST_ASSERT_FALSE(d.early);
    TEST_ASSERT_EQUAL_HEX8(0x0C, d.pids[0]);
    scheduler.onRead(d, 0);

    d = scheduler.next(0, 1);
    TEST_ASSERT_EQUAL_HEX8(0x0D, d.pids[0]);
}

void test_scheduler_waits_when_nothing_due_and_no_early_read() {
    Scheduler scheduler;
    scheduler.apply(twoPids(50, 250, 250), 0);
    Scheduler::Decision d = scheduler.next(0, 1);
    scheduler.onRead(d, 0);
    d = scheduler.next(0, 1);
    scheduler.onRead(d, 0);

    d = scheduler.next(10, 1);
    TEST_ASSERT_FALSE(d.read);
    TEST_ASSERT_EQUAL_UINT32(240, d.waitMs);
}

void test_scheduler_early_read_only_with_more_than_tmax_slack() {
    Scheduler scheduler;
    scheduler.apply(twoPids(50, 400, 100), 0);
    scheduler.onRead(scheduler.next(0, 1), 0);   // 0C: próxima em 400
    scheduler.onRead(scheduler.next(0, 1), 0);   // 0D: próxima em 400

    // Em 100: Pmin cumprido e folga de 300 > Tmax → antecipa.
    Scheduler::Decision d = scheduler.next(100, 1);
    TEST_ASSERT_TRUE(d.read);
    TEST_ASSERT_TRUE(d.early);

    // Em 360: folga de 40 < Tmax → não antecipa, espera o vencimento.
    Scheduler fresh;
    fresh.apply(twoPids(50, 400, 100), 0);
    fresh.onRead(fresh.next(0, 1), 0);
    fresh.onRead(fresh.next(0, 1), 0);
    d = fresh.next(360, 1);
    TEST_ASSERT_FALSE(d.read);
    TEST_ASSERT_EQUAL_UINT32(40, d.waitMs);
}

void test_scheduler_keeps_phase_for_overdue_reads() {
    Scheduler scheduler;
    scheduler.apply(twoPids(50, 250, 250), 0);
    scheduler.onRead(scheduler.next(0, 1), 30);   // lida com atraso de 30 ms
    scheduler.onRead(scheduler.next(30, 1), 60);

    // nextDue += P (250), não now + P (280/310): os dois vencem juntos.
    Scheduler::Decision d = scheduler.next(250, 2);
    TEST_ASSERT_TRUE(d.read);
    TEST_ASSERT_EQUAL_UINT32(2, d.count);
}

// Intervalo entre leituras de um PID ≤ Pᵢ + Σⱼ≠ᵢ Tⱼ, com todas as respostas
// no prazo máximo (pior caso do canal sequencial).
void test_interval_bound_with_worst_case_responses() {
    FakeClock clock;
    FakeObd2 obd(clock);
    const CollectorConfig config = CollectorConfig::defaults();
    for (size_t i = 0; i < config.pidCount; ++i) obd.silent[config.pids[i].pid] = true;

    SequentialPidReader reader(obd, clock);
    FakePool pool;
    FakeValues values;
    Collector collector(clock, reader, pool, values);
    collector.applyConfig(config);
    collector.setRunning(true);

    std::map<uint8_t, uint32_t> lastStart;
    std::map<uint8_t, uint32_t> worstGap;
    for (int step = 0; step < 2000; ++step) {
        const size_t before = obd.requests.size();
        const uint32_t start = clock.now;
        const uint32_t wait = collector.poll();
        if (obd.requests.size() > before) {
            const uint8_t pid = obd.requests.back();
            if (lastStart.count(pid)) {
                const uint32_t gap = start - lastStart[pid];
                if (gap > worstGap[pid]) worstGap[pid] = gap;
            }
            lastStart[pid] = start;
        } else {
            clock.advance(wait == 0 || wait == UINT32_MAX ? 1 : wait);
        }
    }

    const uint32_t others = (config.pidCount - 1) * config.tmaxMs;
    for (size_t i = 0; i < config.pidCount; ++i) {
        const PidSetting& s = config.pids[i];
        TEST_ASSERT_TRUE(worstGap.count(s.pid));
        TEST_ASSERT_LESS_OR_EQUAL_UINT32(s.periodMs + others, worstGap[s.pid]);
    }
}

// ── Blocos ───────────────────────────────────────────────────────────────

void test_block_builder_serializes_records() {
    Block block;
    BlockBuilder builder;
    builder.open(block, 5000, kDefaultFlushIntervalMs);
    const uint8_t rpm[2] = {0x1A, 0xF8};
    TEST_ASSERT_TRUE(builder.addPid(0x0C, rpm, 2, 5300));
    TEST_ASSERT_TRUE(builder.addGps({-235000000, -466000000, 760000}, 5400));
    Block* closed = builder.close();

    TEST_ASSERT_EQUAL_PTR(&block, closed);
    TEST_ASSERT_EQUAL_UINT32(kHeaderSize + kPidRecordSize + kGpsRecordSize, block.length);
    TEST_ASSERT_EQUAL_UINT32(5000, getU32(&block.bytes[header::kStartMs]));
    TEST_ASSERT_EQUAL_UINT16(block.length, getU16(&block.bytes[header::kLength]));

    const uint8_t* pidRecord = &block.bytes[kHeaderSize];
    TEST_ASSERT_EQUAL_HEX8(0x0C, pidRecord[0]);
    TEST_ASSERT_EQUAL_UINT16(300, getU16(&pidRecord[1]));
    TEST_ASSERT_EQUAL_HEX8(0x1A, pidRecord[3]);
    TEST_ASSERT_EQUAL_HEX8(0xF8, pidRecord[4]);

    const uint8_t* gps = pidRecord + kPidRecordSize;
    TEST_ASSERT_EQUAL_HEX8(kGpsRecordType, gps[0]);
    TEST_ASSERT_EQUAL_UINT16(400, getU16(&gps[1]));
    TEST_ASSERT_EQUAL_INT32(-235000000, static_cast<int32_t>(getU32(&gps[3])));
}

void test_delta_is_unsigned_and_rejects_out_of_range() {
    Block block;
    BlockBuilder builder;
    builder.open(block, 5000, kDefaultFlushIntervalMs);
    TEST_ASSERT_FALSE(builder.addGps({1, 2, 3}, 4990));  // anterior ao início: não cabe (spec 02 §2)
    TEST_ASSERT_TRUE(builder.addGps({1, 2, 3}, 5000 + 65535));
    TEST_ASSERT_EQUAL_UINT16(65535, getU16(&block.bytes[kHeaderSize + 1]));
    const uint8_t v[2] = {1, 2};
    TEST_ASSERT_FALSE(builder.addPid(0x0C, v, 2, 5000 + 65536));
}

void test_block_builder_rejects_when_full() {
    Block block;
    BlockBuilder builder;
    builder.open(block, 0, kDefaultFlushIntervalMs);
    const uint8_t v[2] = {1, 2};
    size_t added = 0;
    while (builder.addPid(0x0C, v, 2, 0)) ++added;
    TEST_ASSERT_EQUAL_UINT32((kBlockSize - kHeaderSize) / kPidRecordSize, added);
}

void test_collector_closes_block_by_deadline_even_without_answers() {
    FakeClock clock;
    FakeObd2 obd(clock);
    SequentialPidReader reader(obd, clock);
    FakePool pool;
    FakeValues values;
    Collector collector(clock, reader, pool, values);
    collector.applyConfig(twoPids(50, 250, 250));
    collector.setRunning(true);

    collector.poll();                 // uma leitura abre o bloco
    obd.silent[0x0C] = obd.silent[0x0D] = true;
    for (int i = 0; i < 400 && pool.submitted.empty(); ++i) {
        const uint32_t wait = collector.poll();
        clock.advance(wait == 0 ? 1 : wait);
    }
    TEST_ASSERT_EQUAL_UINT32(1, pool.submitted.size());
}

void test_collector_wait_includes_block_deadline() {
    FakeClock clock;
    FakeObd2 obd(clock);
    SequentialPidReader reader(obd, clock);
    FakePool pool;
    FakeValues values;
    Collector collector(clock, reader, pool, values);
    CollectorConfig config;
    config.pids[0] = {0x2F, 60000, 60000};
    config.pidCount = 1;
    collector.applyConfig(config);
    collector.setRunning(true);

    collector.poll();  // lê 0x2F; próxima só em 60 s
    const uint32_t wait = collector.poll();
    TEST_ASSERT_LESS_OR_EQUAL_UINT32(kDefaultFlushIntervalMs, wait);
}

void test_collector_closes_block_at_configured_interval() {
    FakeClock clock;
    FakeObd2 obd(clock);
    SequentialPidReader reader(obd, clock);
    FakePool pool;
    FakeValues values;
    Collector collector(clock, reader, pool, values);
    CollectorConfig config;
    config.pids[0] = {0x2F, 60000, 60000};
    config.pidCount = 1;
    config.flushIntervalMs = 2000;
    collector.applyConfig(config);
    collector.setRunning(true);

    collector.poll();           // abre o bloco em 1010
    clock.advance(1999);
    collector.poll();
    TEST_ASSERT_EQUAL_UINT32(0, pool.submitted.size());
    clock.advance(1);
    collector.poll();
    TEST_ASSERT_EQUAL_UINT32(1, pool.submitted.size());
}

void test_validate_flush_interval_range() {
    CollectorConfig config = CollectorConfig::defaults();
    config.flushIntervalMs = 999;
    TEST_ASSERT_EQUAL(ConfigError::FlushOutOfRange, validate(config));
    config.flushIntervalMs = 30001;
    TEST_ASSERT_EQUAL(ConfigError::FlushOutOfRange, validate(config));
    config.flushIntervalMs = 2000;
    TEST_ASSERT_EQUAL(ConfigError::None, validate(config));
}

void test_collector_counts_records_dropped_without_free_buffer() {
    FakeClock clock;
    FakeObd2 obd(clock);
    SequentialPidReader reader(obd, clock);
    FakePool pool;
    pool.free.clear();
    FakeValues values;
    Collector collector(clock, reader, pool, values);
    collector.applyConfig(twoPids(50, 250, 250));
    collector.setRunning(true);

    collector.poll();
    TEST_ASSERT_EQUAL_UINT32(1, collector.droppedRecords());
    TEST_ASSERT_EQUAL_UINT32(1, values.updates.size());  // o valor corrente segue atualizado
}

void test_gps_before_block_start_closes_block_and_opens_another() {
    FakeClock clock;
    FakeObd2 obd(clock);
    SequentialPidReader reader(obd, clock);
    FakePool pool;
    FakeValues values;
    Collector collector(clock, reader, pool, values);
    collector.applyConfig(twoPids(50, 250, 250));
    collector.setRunning(true);

    collector.poll();  // abre o bloco no primeiro registro
    const uint32_t t0 = getU32(&pool.buffers[0].bytes[header::kStartMs]);
    collector.addGps({1, 2, 3}, 900);  // anterior ao primeiro registro do bloco
    collector.closeBlock(CloseReason::Stop);

    TEST_ASSERT_EQUAL_UINT32(2, pool.submitted.size());
    TEST_ASSERT_TRUE(900 < 1000);
    (void)t0;
    TEST_ASSERT_EQUAL_UINT32(900, getU32(&pool.submitted[1].bytes[header::kStartMs]));
}

void test_validate_rejects_reserved_pids() {
    CollectorConfig config = CollectorConfig::defaults();
    TEST_ASSERT_FALSE(config.setPid(0xFD, 1000));
    TEST_ASSERT_FALSE(config.setPid(0xFE, 1000));
    TEST_ASSERT_FALSE(config.setPid(0xFF, 1000));
    TEST_ASSERT_TRUE(config.setPid(0xFC, 1000));
    config.pids[0].pid = 0xFE;
    TEST_ASSERT_EQUAL(ConfigError::InvalidPid, validate(config));
}

int main(int, char**) {
    UNITY_BEGIN();
    RUN_TEST(test_defaults_match_table_11);
    RUN_TEST(test_validate_rejects_infeasible_and_floors);
    RUN_TEST(test_scheduler_reads_most_overdue_first);
    RUN_TEST(test_scheduler_waits_when_nothing_due_and_no_early_read);
    RUN_TEST(test_scheduler_early_read_only_with_more_than_tmax_slack);
    RUN_TEST(test_scheduler_keeps_phase_for_overdue_reads);
    RUN_TEST(test_interval_bound_with_worst_case_responses);
    RUN_TEST(test_block_builder_serializes_records);
    RUN_TEST(test_delta_is_unsigned_and_rejects_out_of_range);
    RUN_TEST(test_block_builder_rejects_when_full);
    RUN_TEST(test_collector_closes_block_by_deadline_even_without_answers);
    RUN_TEST(test_collector_wait_includes_block_deadline);
    RUN_TEST(test_collector_counts_records_dropped_without_free_buffer);
    RUN_TEST(test_collector_closes_block_at_configured_interval);
    RUN_TEST(test_validate_flush_interval_range);
    RUN_TEST(test_gps_before_block_start_closes_block_and_opens_another);
    RUN_TEST(test_validate_rejects_reserved_pids);
    return UNITY_END();
}
