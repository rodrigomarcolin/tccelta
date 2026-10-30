#include <unity.h>
#include "collector/AtHashParser.h"
#include "collector/GpsGate.h"
#include "collector/TmaxMeter.h"
#include "support/Fakes.h"

using namespace collector;

void setUp() {}
void tearDown() {}

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

void test_at_ack_parsing() {
    AtHashCommand c = parseAtHash("AT#ACK 42");
    TEST_ASSERT_TRUE(c.kind == AtHashCommand::Kind::Ack);
    TEST_ASSERT_EQUAL_UINT32(42, c.seq);
    TEST_ASSERT_TRUE(parseAtHash("AT#ACK").kind == AtHashCommand::Kind::Invalid);
    TEST_ASSERT_TRUE(parseAtHash("AT#ACK 0").kind == AtHashCommand::Kind::Invalid);

    const AtHashCommand flush = parseAtHash("AT#FLUSH 2000");
    TEST_ASSERT_TRUE(flush.kind == AtHashCommand::Kind::Flush);
    TEST_ASSERT_EQUAL_UINT32(2000, flush.periodMs);
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
    RUN_TEST(test_tmax_meter_p99_and_suggestion);
    RUN_TEST(test_at_hash_parser);
    RUN_TEST(test_at_ack_parsing);
    RUN_TEST(test_gps_gate_rejects_diag_mode);
    RUN_TEST(test_gps_gate_limits_to_one_per_second_with_jitter);
    return UNITY_END();
}
