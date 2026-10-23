#include <unity.h>
#include <string>
#include <vector>
#include "collector/Collector.h"
#include "collector/Events.h"
#include "collector/SequentialPidReader.h"
#include "support/Fakes.h"

using namespace collector;

/** Guarda as linhas como o ESP32 as imprimiria, sem o prefixo "EV n t". */
class CaptureSink : public IEventSink {
public:
    void emit(const char* body, size_t len, const uint32_t* tMs) override {
        lines.emplace_back(body, len);
        times.push_back(tMs ? static_cast<int64_t>(*tMs) : -1);
    }

    /** Linhas que começam com `event`. */
    std::vector<std::string> of(const std::string& event) const {
        std::vector<std::string> out;
        for (const auto& line : lines) {
            if (line.compare(0, event.size() + 1, event + " ") == 0) out.push_back(line);
        }
        return out;
    }

    std::vector<std::string> lines;
    std::vector<int64_t> times;
};

static CaptureSink sink;

void setUp() {
    sink = CaptureSink();
    setEventSink(&sink);
}
void tearDown() { setEventSink(nullptr); }

void test_event_formats_each_value_type() {
    const uint8_t val[2] = {0x1A, 0xF8};
    const uint8_t pids[2] = {0x0C, 0x0D};
    const char txt[] = "AT#PID \"0C\"\r";
    Event("X").num("a", -5).flag("b", true).hex8("c", 0x0C).hex("d", val, 2)
        .hexList("e", pids, 2).token("f", "deadline").absent("g").text("h", txt, sizeof(txt) - 1);
    TEST_ASSERT_EQUAL_STRING("X a=-5 b=1 c=0C d=1AF8 e=0C,0D f=deadline g=- h=\"AT#PID \\\"0C\\\"\\r\"",
                             sink.lines.at(0).c_str());
    TEST_ASSERT_EQUAL_INT64(-1, sink.times.at(0));
}

void test_event_explicit_time_is_forwarded() {
    Event("Y").at(1234);
    TEST_ASSERT_EQUAL_INT64(1234, sink.times.at(0));
}

void test_event_that_does_not_fit_is_not_emitted() {
    char small[16];
    Event("BIG", small, sizeof(small)).num("value", 123456789);
    TEST_ASSERT_EQUAL_UINT32(0, sink.lines.size());
}

void test_collector_emits_config_decide_read_and_record() {
    FakeClock clock;  // começa em 1000
    FakeObd2 obd(clock);
    SequentialPidReader reader(obd, clock);
    FakePool pool;
    FakeValues values;
    Collector collector(clock, reader, pool, values);

    CollectorConfig config;
    config.tmaxMs = 50;
    config.pids[0] = {0x0C, 250, 125};
    config.pidCount = 1;
    collector.applyConfig(config, Collector::ConfigReason::Boot);
    collector.setRunning(true);
    collector.poll();

    TEST_ASSERT_EQUAL_STRING("CONFIG reason=boot proto=6 tmax=50 flush=10000 u=200 pids=0C", sink.of("CONFIG").at(0).c_str());
    TEST_ASSERT_EQUAL_STRING("CVT pid=0C p=250 pmin=125 due=1000 last=875 val=-", sink.of("CVT").at(0).c_str());
    TEST_ASSERT_EQUAL_STRING("DECIDE rule=due pids=0C late=0 slack=-", sink.of("DECIDE").at(0).c_str());
    TEST_ASSERT_EQUAL_STRING("READ pid=0C ok=1 val=0CAB start=1000 dur=10 early=0 next_due=1250",
                             sink.of("READ").at(0).c_str());
    TEST_ASSERT_EQUAL_STRING("BLK_OPEN buf=1 t0=1010", sink.of("BLK_OPEN").at(0).c_str());
    TEST_ASSERT_EQUAL_STRING("REC kind=pid pid=0C dt=0 val=0CAB lat=- lon=- alt=- len=17",
                             sink.of("REC").at(0).c_str());
}

void test_collector_logs_block_close_reason_and_age() {
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
    collector.poll();               // um registro abre o bloco em 1010
    clock.advance(kDefaultFlushIntervalMs);
    collector.poll();               // passou o prazo

    TEST_ASSERT_EQUAL_STRING("BLK_CLOSE buf=1 reason=deadline len=17 recs=1 age=10000",
                             sink.of("BLK_CLOSE").at(0).c_str());
}

void test_early_decision_reports_slack() {
    FakeClock clock;
    FakeObd2 obd(clock);
    SequentialPidReader reader(obd, clock);
    FakePool pool;
    FakeValues values;
    Collector collector(clock, reader, pool, values);

    CollectorConfig config;
    config.tmaxMs = 50;
    config.pids[0] = {0x0C, 400, 100};
    config.pidCount = 1;
    collector.applyConfig(config);
    collector.setRunning(true);
    collector.poll();          // lido em 1010; vence em 1400
    clock.now = 1110;          // Pmin cumprido, folga de 290 > Tmax
    collector.poll();

    TEST_ASSERT_EQUAL_STRING("DECIDE rule=early pids=0C late=- slack=290", sink.of("DECIDE").at(1).c_str());
}

int main(int, char**) {
    UNITY_BEGIN();
    RUN_TEST(test_event_formats_each_value_type);
    RUN_TEST(test_event_explicit_time_is_forwarded);
    RUN_TEST(test_event_that_does_not_fit_is_not_emitted);
    RUN_TEST(test_collector_emits_config_decide_read_and_record);
    RUN_TEST(test_collector_logs_block_close_reason_and_age);
    RUN_TEST(test_early_decision_reports_slack);
    return UNITY_END();
}
