#include <unity.h>
#include <cstdio>
#include <string>
#include <vector>
#include "collector/Events.h"
#include "connectivity/pairing/PairButton.h"
#include "connectivity/pairing/PairingPolicy.h"
#include "support/CaptureSink.h"

using namespace pairing;

static CaptureSink sink;

void setUp() {
    sink = CaptureSink();
    collector::setEventSink(&sink);
}
void tearDown() { collector::setEventSink(nullptr); }

// ── Auxiliares ───────────────────────────────────────────────────────────────

static bool contains(const std::vector<std::string>& lines, const std::string& needle) {
    for (const auto& l : lines) {
        if (l.find(needle) != std::string::npos) return true;
    }
    return false;
}

static bool sawLine(const std::string& needle) { return contains(sink.lines, needle); }

// ── PairButton ───────────────────────────────────────────────────────────────

struct Edges {
    int pressed = 0;
    int released = 0;
    int hold = 0;
};

/** Alimenta o botão de `from` (inclusive) até `to` (exclusive) com passo fixo; aceita virada do relógio. */
static Edges feed(PairButton& b, bool pressed, uint32_t from, uint32_t to, uint32_t step = 20) {
    Edges e;
    const uint32_t n = (to - from) / step;
    for (uint32_t i = 0; i < n; ++i) {
        switch (b.update(pressed, from + i * step)) {
            case PairButton::Edge::Pressed:   ++e.pressed; break;
            case PairButton::Edge::Released:  ++e.released; break;
            case PairButton::Edge::HoldFired: ++e.hold; break;
            case PairButton::Edge::None:      break;
        }
    }
    return e;
}

static int heldOf(const std::string& line) {
    const size_t p = line.find("held=");
    int v = -1;
    if (p != std::string::npos) std::sscanf(line.c_str() + p, "held=%d", &v);
    return v;
}

static void accumulate(Edges& total, const Edges& e) {
    total.pressed += e.pressed;
    total.released += e.released;
    total.hold += e.hold;
}

void test_btn01_released_stays_silent() {
    PairButton b(50, 5000);
    const Edges e = feed(b, false, 0, 1000);
    TEST_ASSERT_EQUAL_INT(0, e.pressed + e.released + e.hold);
    TEST_ASSERT_EQUAL_UINT32(0, sink.lines.size());
}

void test_btn02_short_pulse_is_noise() {
    PairButton b(50, 5000);
    Edges e = feed(b, false, 0, 100, 10);
    accumulate(e, feed(b, true, 100, 130, 10));
    accumulate(e, feed(b, false, 130, 400, 10));
    TEST_ASSERT_EQUAL_INT(0, e.pressed + e.released + e.hold);
    TEST_ASSERT_EQUAL_UINT32(0, sink.lines.size());
}

void test_btn03_stable_press_emits_press() {
    PairButton b(50, 5000);
    feed(b, false, 0, 100);
    const Edges e = feed(b, true, 100, 300);
    TEST_ASSERT_EQUAL_INT(1, e.pressed);
    TEST_ASSERT_TRUE(b.counting());
    TEST_ASSERT_EQUAL_STRING("PAIR_BTN action=press held=0 fired=0", sink.lines.at(0).c_str());
}

void test_btn04_hold_fires_once_at_five_seconds() {
    PairButton b(50, 5000);
    feed(b, false, 0, 100);
    const Edges e = feed(b, true, 100, 5300);
    TEST_ASSERT_EQUAL_INT(1, e.hold);
    const auto holds = sink.of("PAIR_BTN");
    TEST_ASSERT_TRUE(contains(holds, "action=hold"));
    int held = -1;
    for (const auto& l : holds) {
        if (l.find("action=hold") != std::string::npos) held = heldOf(l);
    }
    TEST_ASSERT_TRUE(held >= 5000 && held <= 5020);
}

void test_btn05_release_before_hold_does_not_fire() {
    PairButton b(50, 5000);
    feed(b, false, 0, 100);
    Edges e = feed(b, true, 100, 3100);
    accumulate(e, feed(b, false, 3100, 3400));
    TEST_ASSERT_EQUAL_INT(0, e.hold);
    TEST_ASSERT_EQUAL_INT(1, e.released);
    const std::string last = sink.lines.back();
    TEST_ASSERT_TRUE(last.find("action=release") != std::string::npos);
    TEST_ASSERT_TRUE(last.find("fired=0") != std::string::npos);
    TEST_ASSERT_INT_WITHIN(70, 3000, heldOf(last));
}

void test_btn06_long_press_fires_only_once() {
    PairButton b(50, 5000);
    feed(b, false, 0, 100);
    const Edges e = feed(b, true, 100, 12100);
    TEST_ASSERT_EQUAL_INT(1, e.hold);
}

void test_btn07_release_after_hold_reports_fired() {
    PairButton b(50, 5000);
    feed(b, false, 0, 100);
    feed(b, true, 100, 6100);
    const Edges e = feed(b, false, 6100, 6400);
    TEST_ASSERT_EQUAL_INT(1, e.released);
    TEST_ASSERT_TRUE(sink.lines.back().find("action=release") != std::string::npos);
    TEST_ASSERT_TRUE(sink.lines.back().find("fired=1") != std::string::npos);
}

void test_btn08_boot_with_button_pressed_is_blocked() {
    PairButton b(50, 5000);
    Edges e = feed(b, true, 0, 10000);
    TEST_ASSERT_TRUE(b.blocked());
    TEST_ASSERT_EQUAL_INT(0, e.hold + e.pressed);
    TEST_ASSERT_EQUAL_STRING("PAIR_BTN action=blocked held=0 fired=0", sink.lines.at(0).c_str());
    accumulate(e, feed(b, false, 10000, 10200));
    TEST_ASSERT_FALSE(b.blocked());
    TEST_ASSERT_EQUAL_INT(0, e.released);
    const Edges later = feed(b, true, 10200, 15400);
    TEST_ASSERT_EQUAL_INT(1, later.hold);
}

void test_btn09_release_bounce_counts_once() {
    PairButton b(50, 5000);
    feed(b, false, 0, 100);
    Edges e = feed(b, true, 100, 500);
    // Repique de 40 ms (10 ms alternando) antes de soltar de vez.
    for (uint32_t t = 500; t < 540; t += 10) {
        switch (b.update(((t - 500) / 10) % 2 == 1, t)) {
            case PairButton::Edge::Released: ++e.released; break;
            default: break;
        }
    }
    accumulate(e, feed(b, false, 540, 800, 10));
    TEST_ASSERT_EQUAL_INT(1, e.released);
}

void test_btn10_hold_across_millis_wrap() {
    PairButton b(50, 5000);
    const uint32_t start = 0xFFFFF000u;
    feed(b, false, start, start + 100);
    const Edges e = feed(b, true, start + 100, start + 5400);
    TEST_ASSERT_EQUAL_INT(1, e.hold);
}

void test_btn11_hold_time_comes_from_constructor() {
    PairButton b(50, 2000);
    feed(b, false, 0, 100);
    const Edges e = feed(b, true, 100, 2300);
    TEST_ASSERT_EQUAL_INT(1, e.hold);
    int held = -1;
    for (const auto& l : sink.of("PAIR_BTN")) {
        if (l.find("action=hold") != std::string::npos) held = heldOf(l);
    }
    TEST_ASSERT_TRUE(held >= 2000 && held <= 2020);
}

// ── PairingPolicy ────────────────────────────────────────────────────────────

static EncryptionInfo enc(bool bonded, int key = 16, int status = 0, bool lesc = true, bool persisted = true) {
    EncryptionInfo i;
    i.status = status;
    i.encrypted = true;
    i.bonded = bonded;
    i.keySize = static_cast<uint8_t>(key);
    i.lesc = lesc;
    i.persisted = persisted;
    return i;
}

static bool hasReject(const Actions& a, uint16_t conn, uint8_t hci) {
    for (uint8_t i = 0; i < a.rejectCount; ++i) {
        if (a.reject[i].conn == conn && a.reject[i].hciReason == hci) return true;
    }
    return false;
}

static bool hasDeleteBond(const Actions& a, uint16_t conn) {
    for (uint8_t i = 0; i < a.deleteBondCount; ++i) {
        if (a.deleteBond[i] == conn) return true;
    }
    return false;
}

struct Fx {
    PairingPolicy p{Timings{45000, 5000, 35000}, 3};

    Actions begin(bool hasBond, uint32_t t = 1000) {
        Actions a;
        p.begin(t, hasBond, a);
        return a;
    }
    /** Botão + apagamento ok; a janela abre em `t + 20`. */
    Actions openWindow(uint32_t t) {
        Actions a;
        p.onButtonHold(t, a);
        Actions b;
        p.onBondsDeleted(true, t + 20, b);
        return b;
    }
    Actions up(uint16_t c, uint32_t t) {
        Actions a;
        p.onLinkUp(c, t, a);
        return a;
    }
    Actions encrypt(uint16_t c, const EncryptionInfo& i, uint32_t t) {
        Actions a;
        p.onEncryptionChange(c, i, t, a);
        return a;
    }
    Actions tick(uint32_t t) {
        Actions a;
        p.tick(t, a);
        return a;
    }
    Actions down(uint16_t c, uint32_t t) {
        Actions a;
        p.onLinkDown(c, t, a);
        return a;
    }
};

void test_pol01_boot_without_bond() {
    Fx f;
    const Actions a = f.begin(false);
    TEST_ASSERT_EQUAL(WindowState::Closed, f.p.window());
    TEST_ASSERT_TRUE(a.setBonding);
    TEST_ASSERT_FALSE(a.bondingAllowed);
    TEST_ASSERT_TRUE(a.setAdvertising);
    TEST_ASSERT_EQUAL(AdvMode::Normal, a.adv);
    TEST_ASSERT_FALSE(a.windowOpen);
    TEST_ASSERT_FALSE(a.hasBond);
    TEST_ASSERT_EQUAL_STRING("PAIR_WINDOW state=close reason=boot deadline=- dur=-", sink.lines.at(0).c_str());
}

void test_pol02_boot_with_bond() {
    Fx f;
    const Actions a = f.begin(true);
    TEST_ASSERT_EQUAL(AdvMode::Normal, a.adv);
    TEST_ASSERT_TRUE(a.hasBond);
    TEST_ASSERT_TRUE(f.p.hasBond());
}

void test_pol03_button_hold_drops_everything() {
    Fx f;
    f.begin(true);
    f.up(1, 2000);
    f.encrypt(1, enc(true), 2100);
    TEST_ASSERT_TRUE(f.p.isAuthorized(1));
    Actions a;
    f.p.onButtonHold(30000, a);
    TEST_ASSERT_TRUE(a.advertisingOffFirst);
    TEST_ASSERT_TRUE(a.disconnectAll);
    TEST_ASSERT_TRUE(a.deleteAllBonds);
    TEST_ASSERT_FALSE(f.p.isAuthorized(1));
    TEST_ASSERT_EQUAL(WindowState::Wiping, f.p.window());
    TEST_ASSERT_FALSE(f.p.bondingAllowed());
    TEST_ASSERT_EQUAL(AdvMode::Off, a.adv);
}

void test_pol04_bonds_deleted_opens_window() {
    Fx f;
    f.begin(true);
    Actions h;
    f.p.onButtonHold(30000, h);
    Actions a;
    f.p.onBondsDeleted(true, 30020, a);
    TEST_ASSERT_EQUAL(WindowState::Open, f.p.window());
    TEST_ASSERT_EQUAL_UINT32(75020, f.p.windowDeadline());
    TEST_ASSERT_TRUE(a.setBonding);
    TEST_ASSERT_TRUE(a.bondingAllowed);
    TEST_ASSERT_EQUAL(AdvMode::Pairing, a.adv);
    TEST_ASSERT_TRUE(a.windowOpen);
    TEST_ASSERT_FALSE(a.hasBond);
    TEST_ASSERT_TRUE(sawLine("PAIR_WINDOW state=open reason=button deadline=75020 dur=-"));
}

void test_pol05_old_session_is_deauthorized_on_link_down() {
    Fx f;
    f.begin(true);
    f.up(1, 2000);
    f.encrypt(1, enc(true), 2100);
    f.openWindow(30000);
    const Actions a = f.down(1, 30100);
    TEST_ASSERT_TRUE(a.deauthorize);
    TEST_ASSERT_EQUAL_UINT16(1, a.deauthorizeConn);
}

void test_pol06_window_closes_on_timeout() {
    Fx f;
    f.begin(false);
    f.openWindow(10000);  // abre em 10020
    Actions a = f.tick(10020 + 44999);
    TEST_ASSERT_EQUAL(WindowState::Open, f.p.window());
    TEST_ASSERT_FALSE(a.setBonding);
    a = f.tick(10020 + 45000);
    TEST_ASSERT_EQUAL(WindowState::Closed, f.p.window());
    TEST_ASSERT_TRUE(a.setBonding);
    TEST_ASSERT_FALSE(a.bondingAllowed);
    TEST_ASSERT_EQUAL(AdvMode::Normal, a.adv);
    TEST_ASSERT_TRUE(sawLine("PAIR_WINDOW state=close reason=timeout deadline=- dur=45000"));
}

void test_pol07_first_bond_in_window_authorizes_and_closes() {
    Fx f;
    f.begin(false);
    f.openWindow(10000);
    f.up(7, 12000);
    const Actions a = f.encrypt(7, enc(true), 15000);
    TEST_ASSERT_EQUAL_UINT16(7, a.authorize);
    TEST_ASSERT_TRUE(f.p.isAuthorized(7));
    TEST_ASSERT_EQUAL(WindowState::Closed, f.p.window());
    TEST_ASSERT_TRUE(a.setBonding);
    TEST_ASSERT_FALSE(a.bondingAllowed);
    TEST_ASSERT_EQUAL(AdvMode::Off, a.adv);
    TEST_ASSERT_TRUE(f.p.hasBond());
    TEST_ASSERT_TRUE(sawLine("PAIR_RESULT conn=7 ok=1 reason=ok late=0 rc=0 dur=3000"));
    TEST_ASSERT_TRUE(sawLine("PAIR_WINDOW state=close reason=bonded deadline=- dur="));
    TEST_ASSERT_TRUE(sawLine("BOND_VERIFY conn=7 ok=1 lesc=1 persisted=1"));
    TEST_ASSERT_TRUE(sawLine("LINK_ENC conn=7 up=1 bonded=1 fresh=1 key=16 reason=ok rc=0 dur=3000"));
}

void test_pol08_restore_outside_window() {
    Fx f;
    f.begin(true);
    f.up(3, 2000);
    const Actions a = f.encrypt(3, enc(true), 2188);
    TEST_ASSERT_EQUAL_UINT16(3, a.authorize);
    TEST_ASSERT_EQUAL(AdvMode::Off, a.adv);
    TEST_ASSERT_TRUE(sawLine("LINK_ENC conn=3 up=1 bonded=1 fresh=0 key=16 reason=ok rc=0 dur=188"));
    TEST_ASSERT_EQUAL_UINT32(0, sink.of("PAIR_RESULT").size());
}

void test_pol09_pairing_without_bond_outside_window_is_rejected() {
    Fx f;
    f.begin(false);
    f.up(2, 1000);
    const Actions a = f.encrypt(2, enc(false), 4000);
    TEST_ASSERT_TRUE(hasReject(a, 2, 0x05));
    TEST_ASSERT_TRUE(sawLine("PAIR_RESULT conn=2 ok=0 reason=window_closed"));
    TEST_ASSERT_TRUE(sawLine("CONN_REJECT conn=2 reason=not_bonded hci=05 dur=3000"));
    TEST_ASSERT_FALSE(f.p.isAuthorized(2));
}

void test_pol10_pairing_without_bond_inside_window_keeps_window() {
    Fx f;
    f.begin(false);
    f.openWindow(10000);
    f.up(2, 11000);
    const Actions a = f.encrypt(2, enc(false), 14000);
    TEST_ASSERT_TRUE(hasReject(a, 2, 0x05));
    TEST_ASSERT_TRUE(sawLine("PAIR_RESULT conn=2 ok=0 reason=not_bonded"));
    TEST_ASSERT_EQUAL(WindowState::Open, f.p.window());
}

void test_pol11_link_auth_timeout_outside_window() {
    Fx f;
    f.begin(false);
    f.up(4, 1000);
    Actions a = f.tick(5999);
    TEST_ASSERT_EQUAL_UINT8(0, a.rejectCount);
    a = f.tick(6000);
    TEST_ASSERT_TRUE(hasReject(a, 4, 0x05));
    TEST_ASSERT_TRUE(sawLine("CONN_REJECT conn=4 reason=auth_timeout hci=05 dur=5000"));
}

void test_pol12_window_connection_gets_pairing_timeout() {
    Fx f;
    f.begin(false);
    f.openWindow(10000);  // fecha em 55020
    f.up(4, 50000);
    f.tick(50000 + 4999);
    const Actions early = f.tick(55020);  // janela fechou, mas a conexão segue admitida
    TEST_ASSERT_EQUAL(WindowState::Closed, f.p.window());
    TEST_ASSERT_EQUAL_UINT8(0, early.rejectCount);
    f.tick(50000 + 34999);
    const Actions late = f.tick(50000 + 35000);
    TEST_ASSERT_TRUE(hasReject(late, 4, 0x05));
    TEST_ASSERT_TRUE(sawLine("CONN_REJECT conn=4 reason=auth_timeout hci=05 dur=35000"));
}

void test_pol13_pairing_started_in_window_may_finish_late() {
    Fx f;
    f.begin(false);
    f.openWindow(10000);
    f.up(4, 40000);
    f.tick(10020 + 45000);  // janela fecha
    const size_t windowLines = sink.of("PAIR_WINDOW").size();
    const Actions a = f.encrypt(4, enc(true), 56000);
    TEST_ASSERT_EQUAL_UINT16(4, a.authorize);
    TEST_ASSERT_TRUE(sawLine("PAIR_RESULT conn=4 ok=1 reason=ok late=1"));
    TEST_ASSERT_EQUAL_UINT32(windowLines, sink.of("PAIR_WINDOW").size());
    TEST_ASSERT_TRUE(f.p.hasBond());
}

void test_pol14_only_one_bond_per_window() {
    Fx f;
    f.begin(false);
    f.openWindow(10000);
    f.up(1, 11000);
    f.up(2, 11100);
    const Actions first = f.encrypt(1, enc(true), 14000);
    TEST_ASSERT_EQUAL_UINT16(1, first.authorize);
    const Actions second = f.encrypt(2, enc(true), 14100);
    TEST_ASSERT_EQUAL_UINT16(kNoConn, second.authorize);
    TEST_ASSERT_TRUE(hasReject(second, 2, 0x09));
    TEST_ASSERT_TRUE(sawLine("PAIR_RESULT conn=2 ok=0 reason=busy"));
    TEST_ASSERT_TRUE(f.p.isAuthorized(1));
    TEST_ASSERT_FALSE(f.p.isAuthorized(2));
}

void test_pol15_second_link_while_authorized_is_busy() {
    Fx f;
    f.begin(true);
    f.up(1, 1000);
    f.encrypt(1, enc(true), 1100);
    const Actions a = f.up(2, 2000);
    TEST_ASSERT_TRUE(hasReject(a, 2, 0x09));
    TEST_ASSERT_TRUE(sawLine("CONN_REJECT conn=2 reason=busy hci=09 dur=0"));
}

void test_pol16_pairing_failures_are_classified() {
    Fx f;
    f.begin(false);
    f.openWindow(10000);
    f.up(1, 11000);
    f.up(2, 11000);
    f.up(3, 11000);
    f.encrypt(1, enc(false, 16, 0x508), 12000);
    f.encrypt(2, enc(false, 16, 0x403), 12000);
    f.encrypt(3, enc(false, 16, 13), 12000);
    TEST_ASSERT_TRUE(sawLine("PAIR_RESULT conn=1 ok=0 reason=peer_reject"));
    TEST_ASSERT_TRUE(sawLine("PAIR_RESULT conn=2 ok=0 reason=local_reject"));
    TEST_ASSERT_TRUE(sawLine("PAIR_RESULT conn=3 ok=0 reason=timeout"));
    TEST_ASSERT_EQUAL_UINT32(3, sink.of("CONN_REJECT").size());
    TEST_ASSERT_TRUE(sawLine("LINK_ENC conn=1 up=0"));
    TEST_ASSERT_TRUE(sawLine("reason=enc_fail"));
}

void test_pol17_hci_error_on_encryption_is_not_a_pairing_result() {
    Fx f;
    f.begin(false);
    f.up(1, 1000);
    const Actions a = f.encrypt(1, enc(false, 0, 0x206), 2000);
    TEST_ASSERT_TRUE(hasReject(a, 1, 0x05));
    TEST_ASSERT_TRUE(sawLine("reason=other rc=518"));
    TEST_ASSERT_EQUAL_UINT32(0, sink.of("PAIR_RESULT").size());
    TEST_ASSERT_TRUE(sawLine("CONN_REJECT conn=1 reason=enc_fail"));
}

void test_pol18_short_key_is_rejected() {
    Fx f;
    f.begin(true);
    f.up(1, 1000);
    const Actions a = f.encrypt(1, enc(true, 7), 1200);
    TEST_ASSERT_TRUE(hasReject(a, 1, 0x05));
    TEST_ASSERT_TRUE(hasDeleteBond(a, 1));
    TEST_ASSERT_TRUE(sawLine("CONN_REJECT conn=1 reason=key_size"));
}

void test_pol19_rekey_on_authorized_link_is_idempotent() {
    Fx f;
    f.begin(true);
    f.up(1, 1000);
    f.encrypt(1, enc(true), 1100);
    const Actions a = f.encrypt(1, enc(true), 9000);
    TEST_ASSERT_EQUAL_UINT8(0, a.rejectCount);
    TEST_ASSERT_EQUAL_UINT16(kNoConn, a.authorize);
    TEST_ASSERT_TRUE(f.p.isAuthorized(1));
}

void test_pol20_full_table_turns_advertising_off() {
    Fx f;
    f.begin(false);
    f.up(1, 1000);
    f.up(2, 1000);
    const Actions full = f.up(3, 1000);
    TEST_ASSERT_EQUAL(AdvMode::Off, full.adv);
    TEST_ASSERT_TRUE(full.setAdvertising);
    const Actions freed = f.down(2, 1500);
    TEST_ASSERT_EQUAL(AdvMode::Normal, freed.adv);
    TEST_ASSERT_TRUE(freed.setAdvertising);
}

void test_pol21_unknown_connection_is_ignored() {
    Fx f;
    f.begin(false);
    f.up(1, 1000);
    f.tick(7000);  // conexão 1 passa a Rejecting
    sink.lines.clear();
    const Actions a = f.encrypt(1, enc(false), 7100);  // Rejecting: ignorado
    TEST_ASSERT_EQUAL_UINT8(0, a.rejectCount);
    const Actions b = f.encrypt(9, enc(true), 7100);
    const Actions c = f.down(9, 7100);
    TEST_ASSERT_EQUAL_UINT8(0, b.rejectCount);
    TEST_ASSERT_FALSE(c.deauthorize);
    TEST_ASSERT_EQUAL_UINT32(0, sink.lines.size());
}

void test_pol22_rejection_is_repeated_every_two_seconds() {
    Fx f;
    f.begin(false);
    f.up(1, 1000);
    f.tick(6000);  // recusa
    Actions a = f.tick(7999);
    TEST_ASSERT_EQUAL_UINT8(0, a.rejectCount);
    a = f.tick(8000);
    TEST_ASSERT_TRUE(hasReject(a, 1, 0x05));
}

void test_pol23_failed_wipe_recovers_store() {
    Fx f;
    f.begin(true);
    Actions h;
    f.p.onButtonHold(5000, h);
    Actions a;
    f.p.onBondsDeleted(false, 5100, a);
    TEST_ASSERT_TRUE(a.recoverStore);
    TEST_ASSERT_TRUE(a.setBonding);
    TEST_ASSERT_FALSE(a.bondingAllowed);
    TEST_ASSERT_EQUAL(WindowState::Closed, f.p.window());
    TEST_ASSERT_TRUE(sawLine("PAIR_WINDOW state=close reason=error deadline=- dur=0"));
}

void test_pol24_second_hold_restarts_window() {
    Fx f;
    f.begin(false);
    f.openWindow(10000);
    Actions h;
    f.p.onButtonHold(20000, h);
    TEST_ASSERT_EQUAL(WindowState::Wiping, f.p.window());
    TEST_ASSERT_FALSE(f.p.bondingAllowed());
    TEST_ASSERT_TRUE(h.setBonding);
    Actions a;
    f.p.onBondsDeleted(true, 20020, a);
    TEST_ASSERT_EQUAL_UINT32(65020, f.p.windowDeadline());
}

// Propriedade: sequências pseudoaleatórias nunca violam as invariantes da política.
void test_pol25_random_sequences_keep_invariants() {
    uint32_t seed = 0xC0FFEE;
    auto rnd = [&seed](uint32_t n) {
        seed = seed * 1664525u + 1013904223u;
        return (seed >> 8) % n;
    };
    Fx f;
    f.begin(false, 0);
    uint32_t now = 0;
    bool storeBond = false;     // modelo do bond guardado no store da NimBLE
    int okSinceOpen = 0;        // PAIR_RESULT ok=1 desde o último onBondsDeleted(true)
    size_t seen = 0;
    bool wiping = false;

    for (int step = 0; step < 10000; ++step) {
        now += 1 + rnd(3000);
        const uint16_t conn = static_cast<uint16_t>(1 + rnd(4));
        Actions a;
        switch (rnd(8)) {
            case 0: f.p.onLinkUp(conn, now, a); break;
            case 1:
            case 2: {
                // O modelo do SM: bond novo só se o gate estava aberto; senão só restaura o que existe.
                EncryptionInfo info;
                info.encrypted = true;
                info.keySize = 16;
                info.lesc = true;
                info.persisted = true;
                if (f.p.bondingAllowed()) {
                    info.bonded = true;
                    storeBond = true;
                } else {
                    info.bonded = storeBond;
                }
                f.p.onEncryptionChange(conn, info, now, a);
                break;
            }
            case 3: f.p.onLinkDown(conn, now, a); break;
            case 4: f.p.tick(now, a); break;
            case 5:
                if (rnd(6) == 0) {
                    f.p.onButtonHold(now, a);
                    wiping = true;
                }
                break;
            case 6:
                if (wiping) {
                    f.p.onBondsDeleted(true, now, a);
                    storeBond = false;
                    okSinceOpen = 0;
                    wiping = false;
                }
                break;
            default: f.p.tick(now + 10, a); break;
        }
        // Eventos novos: conta pareamentos concluídos.
        for (; seen < sink.lines.size(); ++seen) {
            if (sink.lines[seen].rfind("PAIR_RESULT ", 0) == 0 && sink.lines[seen].find(" ok=1 ") != std::string::npos)
                ++okSinceOpen;
        }
        TEST_ASSERT_TRUE(!f.p.bondingAllowed() || f.p.window() == WindowState::Open);
        TEST_ASSERT_TRUE(okSinceOpen <= 1);
        int authorized = 0;
        for (uint16_t c = 1; c <= 4; ++c) authorized += f.p.isAuthorized(c) ? 1 : 0;
        TEST_ASSERT_TRUE(authorized <= 1);
        if (authorized == 1) TEST_ASSERT_EQUAL(AdvMode::Off, f.p.advMode());
        if (sink.lines.size() > 4000) {
            sink.lines.clear();
            sink.times.clear();
            seen = 0;
        }
    }
}

void test_pol26_window_closes_across_millis_wrap() {
    Fx f;
    f.begin(false, 0xFFFFFF00u);
    f.openWindow(0xFFFFFF00u);  // abre em 0xFFFFFF14
    const uint32_t openedAt = 0xFFFFFF14u;
    Actions a = f.tick(openedAt + 44999u);
    TEST_ASSERT_EQUAL(WindowState::Open, f.p.window());
    a = f.tick(openedAt + 45000u);
    TEST_ASSERT_EQUAL(WindowState::Closed, f.p.window());
    TEST_ASSERT_TRUE(sawLine("reason=timeout deadline=- dur=45000"));
}

void test_pol27_links_during_wipe_are_busy_and_never_bond() {
    Fx f;
    f.begin(true);
    Actions h;
    f.p.onButtonHold(5000, h);
    const Actions up = f.up(5, 5010);
    TEST_ASSERT_TRUE(hasReject(up, 5, 0x09));
    TEST_ASSERT_TRUE(sawLine("CONN_REJECT conn=5 reason=busy hci=09 dur=0"));
    Actions open;
    f.p.onBondsDeleted(true, 5050, open);
    const Actions a = f.encrypt(5, enc(true), 5100);
    TEST_ASSERT_EQUAL_UINT16(kNoConn, a.authorize);
    TEST_ASSERT_FALSE(f.p.isAuthorized(5));
}

void test_pol27b_pending_link_at_hold_never_becomes_the_new_bond() {
    Fx f;
    f.begin(false);
    f.up(6, 1000);  // Pending quando o botão é segurado
    Actions h;
    f.p.onButtonHold(2000, h);
    Actions open;
    f.p.onBondsDeleted(true, 2020, open);
    const Actions a = f.encrypt(6, enc(true), 2500);
    TEST_ASSERT_EQUAL_UINT16(kNoConn, a.authorize);
    TEST_ASSERT_FALSE(f.p.hasBond());
    TEST_ASSERT_EQUAL(WindowState::Open, f.p.window());
}

void test_pol28_legacy_bond_is_deleted_and_rejected() {
    Fx f;
    f.begin(false);
    f.openWindow(10000);
    f.up(1, 11000);
    const Actions a = f.encrypt(1, enc(true, 16, 0, false), 14000);
    TEST_ASSERT_TRUE(hasReject(a, 1, 0x05));
    TEST_ASSERT_TRUE(hasDeleteBond(a, 1));
    TEST_ASSERT_TRUE(sawLine("PAIR_RESULT conn=1 ok=0 reason=legacy"));
    TEST_ASSERT_TRUE(sawLine("BOND_DELETE reason=policy"));
    TEST_ASSERT_EQUAL(WindowState::Open, f.p.window());
}

void test_pol29_bond_not_persisted_is_rejected_and_window_survives() {
    Fx f;
    f.begin(false);
    f.openWindow(10000);
    f.up(1, 11000);
    const Actions a = f.encrypt(1, enc(true, 16, 0, true, false), 14000);
    TEST_ASSERT_TRUE(hasReject(a, 1, 0x05));
    TEST_ASSERT_TRUE(hasDeleteBond(a, 1));
    TEST_ASSERT_TRUE(sawLine("BOND_VERIFY conn=1 ok=0"));
    TEST_ASSERT_TRUE(sawLine("PAIR_RESULT conn=1 ok=0 reason=not_persisted"));
    TEST_ASSERT_EQUAL(WindowState::Open, f.p.window());
    // A janela não foi consumida: outro celular consegue parear.
    f.up(2, 15000);
    const Actions b = f.encrypt(2, enc(true), 17000);
    TEST_ASSERT_EQUAL_UINT16(2, b.authorize);
}

void test_pol30_pairing_finished_after_rejection_deletes_the_bond() {
    Fx f;
    f.begin(false);
    f.openWindow(10000);
    f.up(1, 11000);
    f.tick(11000 + 35000);  // recusa por prazo
    sink.lines.clear();
    const Actions a = f.encrypt(1, enc(true), 48000);
    TEST_ASSERT_TRUE(hasDeleteBond(a, 1));
    TEST_ASSERT_TRUE(sawLine("BOND_DELETE reason=policy"));
    TEST_ASSERT_EQUAL_UINT16(kNoConn, a.authorize);
    TEST_ASSERT_FALSE(f.p.hasBond());
    // Janela já fechou por tempo? Ainda aberta (10020 + 45000 = 55020): não foi consumida.
    TEST_ASSERT_EQUAL(WindowState::Open, f.p.window());
    f.up(2, 49000);
    TEST_ASSERT_EQUAL_UINT16(2, f.encrypt(2, enc(true), 50000).authorize);
}

void test_pol31_second_bond_is_deleted_without_touching_the_first() {
    Fx f;
    f.begin(false);
    f.openWindow(10000);
    f.up(1, 11000);
    f.up(2, 11100);
    f.encrypt(1, enc(true), 14000);
    const Actions b = f.encrypt(2, enc(true), 14100);
    TEST_ASSERT_TRUE(hasReject(b, 2, 0x09));
    TEST_ASSERT_TRUE(hasDeleteBond(b, 2));
    TEST_ASSERT_FALSE(hasDeleteBond(b, 1));
}

void test_pol32_restore_while_another_link_is_authorized_is_busy_without_deleting() {
    Fx f;
    f.begin(true);
    f.up(1, 1000);
    f.up(2, 1000);
    f.encrypt(1, enc(true), 1100);
    const Actions b = f.encrypt(2, enc(true), 1200);
    TEST_ASSERT_TRUE(hasReject(b, 2, 0x09));
    TEST_ASSERT_FALSE(hasDeleteBond(b, 2));
    TEST_ASSERT_TRUE(f.p.isAuthorized(1));
}

int main() {
    UNITY_BEGIN();
    RUN_TEST(test_btn01_released_stays_silent);
    RUN_TEST(test_btn02_short_pulse_is_noise);
    RUN_TEST(test_btn03_stable_press_emits_press);
    RUN_TEST(test_btn04_hold_fires_once_at_five_seconds);
    RUN_TEST(test_btn05_release_before_hold_does_not_fire);
    RUN_TEST(test_btn06_long_press_fires_only_once);
    RUN_TEST(test_btn07_release_after_hold_reports_fired);
    RUN_TEST(test_btn08_boot_with_button_pressed_is_blocked);
    RUN_TEST(test_btn09_release_bounce_counts_once);
    RUN_TEST(test_btn10_hold_across_millis_wrap);
    RUN_TEST(test_btn11_hold_time_comes_from_constructor);
    RUN_TEST(test_pol01_boot_without_bond);
    RUN_TEST(test_pol02_boot_with_bond);
    RUN_TEST(test_pol03_button_hold_drops_everything);
    RUN_TEST(test_pol04_bonds_deleted_opens_window);
    RUN_TEST(test_pol05_old_session_is_deauthorized_on_link_down);
    RUN_TEST(test_pol06_window_closes_on_timeout);
    RUN_TEST(test_pol07_first_bond_in_window_authorizes_and_closes);
    RUN_TEST(test_pol08_restore_outside_window);
    RUN_TEST(test_pol09_pairing_without_bond_outside_window_is_rejected);
    RUN_TEST(test_pol10_pairing_without_bond_inside_window_keeps_window);
    RUN_TEST(test_pol11_link_auth_timeout_outside_window);
    RUN_TEST(test_pol12_window_connection_gets_pairing_timeout);
    RUN_TEST(test_pol13_pairing_started_in_window_may_finish_late);
    RUN_TEST(test_pol14_only_one_bond_per_window);
    RUN_TEST(test_pol15_second_link_while_authorized_is_busy);
    RUN_TEST(test_pol16_pairing_failures_are_classified);
    RUN_TEST(test_pol17_hci_error_on_encryption_is_not_a_pairing_result);
    RUN_TEST(test_pol18_short_key_is_rejected);
    RUN_TEST(test_pol19_rekey_on_authorized_link_is_idempotent);
    RUN_TEST(test_pol20_full_table_turns_advertising_off);
    RUN_TEST(test_pol21_unknown_connection_is_ignored);
    RUN_TEST(test_pol22_rejection_is_repeated_every_two_seconds);
    RUN_TEST(test_pol23_failed_wipe_recovers_store);
    RUN_TEST(test_pol24_second_hold_restarts_window);
    RUN_TEST(test_pol25_random_sequences_keep_invariants);
    RUN_TEST(test_pol26_window_closes_across_millis_wrap);
    RUN_TEST(test_pol27_links_during_wipe_are_busy_and_never_bond);
    RUN_TEST(test_pol27b_pending_link_at_hold_never_becomes_the_new_bond);
    RUN_TEST(test_pol28_legacy_bond_is_deleted_and_rejected);
    RUN_TEST(test_pol29_bond_not_persisted_is_rejected_and_window_survives);
    RUN_TEST(test_pol30_pairing_finished_after_rejection_deletes_the_bond);
    RUN_TEST(test_pol31_second_bond_is_deleted_without_touching_the_first);
    RUN_TEST(test_pol32_restore_while_another_link_is_authorized_is_busy_without_deleting);
    return UNITY_END();
}
