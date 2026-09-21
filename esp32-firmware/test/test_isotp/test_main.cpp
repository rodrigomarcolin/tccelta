#include <unity.h>
#include "obd2/real/IsoTpClient.h"
#include "FakeCanBus.h"

void setUp() {}
void tearDown() {}

// ── RX: recepção da resposta do peer ─────────────────────────────────────

// Mode 03, 0 DTCs: "43 00" cabe inteiro em um Single Frame.
void test_rx_sf_clean() {
    FakeCanBus can;
    can.enqueue(0x7E8, {0x02, 0x43, 0x00});

    uint8_t req[1] = {0x03};
    uint8_t out[32];
    int n = IsoTp::request(&can, 0x7DF, 0x7E8, 0x7EF, req, 1, out, sizeof(out));

    TEST_ASSERT_EQUAL_INT(2, n);
    TEST_ASSERT_EQUAL_HEX8(0x43, out[0]);
    TEST_ASSERT_EQUAL_HEX8(0x00, out[1]);
}

// Mode 03, 3 DTCs: 8 bytes de payload (SID+count+3*2) -> First Frame (6
// bytes) + 1 Consecutive Frame (2 bytes restantes).
void test_rx_ff_one_cf() {
    FakeCanBus can;
    can.enqueue(0x7E8, {0x10, 0x08, 0x43, 0x03, 0x03, 0x01, 0x04, 0x20});
    can.enqueue(0x7E8, {0x21, 0x07, 0x00});

    uint8_t req[1] = {0x03};
    uint8_t out[32];
    int n = IsoTp::request(&can, 0x7DF, 0x7E8, 0x7EF, req, 1, out, sizeof(out));

    TEST_ASSERT_EQUAL_INT(8, n);
    uint8_t expected[8] = {0x43, 0x03, 0x03, 0x01, 0x04, 0x20, 0x07, 0x00};
    TEST_ASSERT_EQUAL_UINT8_ARRAY(expected, out, 8);

    // can.sent[0] é a própria requisição (SF, 1 byte, enviada antes de
    // qualquer resposta chegar). O FC vem depois, endereçado ao ID físico de
    // requisição (0x7E0 = 0x7E8 - 8).
    TEST_ASSERT_EQUAL_UINT32(2, can.sent.size());
    TEST_ASSERT_EQUAL_UINT32(0x7E0, can.sent[1].id);
    TEST_ASSERT_EQUAL_HEX8(0x30, can.sent[1].data[0]);  // FC, FS=CTS
}

// Mode 03, 6 DTCs: 14 bytes de payload -> First Frame (6) + 2 Consecutive
// Frames (7 + 1 bytes restantes) — valida sequenciamento (SN 1, 2).
void test_rx_ff_two_cf() {
    FakeCanBus can;
    can.enqueue(0x7E8, {0x10, 0x0E, 0x43, 0x06, 0x03, 0x01, 0x01, 0x71});
    can.enqueue(0x7E8, {0x21, 0x04, 0x20, 0x07, 0x00, 0x08, 0x30, 0x0A});
    can.enqueue(0x7E8, {0x22, 0x00});

    uint8_t req[1] = {0x03};
    uint8_t out[32];
    int n = IsoTp::request(&can, 0x7DF, 0x7E8, 0x7EF, req, 1, out, sizeof(out));

    TEST_ASSERT_EQUAL_INT(14, n);
    uint8_t expected[14] = {0x43, 0x06, 0x03, 0x01, 0x01, 0x71, 0x04, 0x20,
                             0x07, 0x00, 0x08, 0x30, 0x0A, 0x00};
    TEST_ASSERT_EQUAL_UINT8_ARRAY(expected, out, 14);
}

// Resposta negativa (7F SID NRC) em Single Frame.
void test_rx_negative_response() {
    FakeCanBus can;
    can.enqueue(0x7E8, {0x03, 0x7F, 0x03, 0x11});

    uint8_t req[1] = {0x03};
    uint8_t out[32];
    int n = IsoTp::request(&can, 0x7DF, 0x7E8, 0x7EF, req, 1, out, sizeof(out));

    TEST_ASSERT_EQUAL_INT(IsoTp::NEGATIVE_RESPONSE, n);
    TEST_ASSERT_EQUAL_HEX8(0x03, out[0]);  // origService
    TEST_ASSERT_EQUAL_HEX8(0x11, out[1]);  // NRC serviceNotSupported
}

void test_rx_multi_ecu_single_frame() {
    FakeCanBus can;
    can.enqueue(0x7E8, {0x03, 0x41, 0x0C, 0x10});
    can.enqueue(0x7E9, {0x03, 0x41, 0x0C, 0x20});

    uint8_t req[2] = {0x01, 0x0C};
    IsoTp::ResponseSet responses;
    int n = IsoTp::requestAll(&can, 0x7DF, 0x7E8, 0x7EF,
                              req, sizeof(req), responses, 20, 2);

    TEST_ASSERT_EQUAL_INT(2, n);
    TEST_ASSERT_EQUAL_UINT32(2, responses.count);
    TEST_ASSERT_EQUAL_HEX32(0x7E8, responses.items[0].ecuId);
    TEST_ASSERT_EQUAL_HEX32(0x7E9, responses.items[1].ecuId);
    TEST_ASSERT_EQUAL_HEX8(0x10, responses.items[0].data[2]);
    TEST_ASSERT_EQUAL_HEX8(0x20, responses.items[1].data[2]);
}

void test_rx_multi_ecu_interleaved_multiframe() {
    FakeCanBus can;
    can.enqueue(0x7E8, {0x10, 0x08, 0x41, 0x0C, 0x01, 0x02, 0x03, 0x04});
    can.enqueue(0x7E9, {0x10, 0x08, 0x41, 0x0C, 0x11, 0x12, 0x13, 0x14});
    can.enqueue(0x7E8, {0x21, 0x05, 0x06});
    can.enqueue(0x7E9, {0x21, 0x15, 0x16});

    uint8_t req[2] = {0x01, 0x0C};
    IsoTp::ResponseSet responses;
    int n = IsoTp::requestAll(&can, 0x7DF, 0x7E8, 0x7EF,
                              req, sizeof(req), responses, 20, 2);

    TEST_ASSERT_EQUAL_INT(2, n);
    TEST_ASSERT_EQUAL_UINT32(4, can.sent.size());
    TEST_ASSERT_EQUAL_HEX32(0x7E0, can.sent[1].id);
    TEST_ASSERT_EQUAL_HEX32(0x7E1, can.sent[2].id);
    TEST_ASSERT_EQUAL_HEX8(0x06, responses.items[0].data[7]);
    TEST_ASSERT_EQUAL_HEX8(0x16, responses.items[1].data[7]);
}

// Nenhuma resposta chega -> timeout (N_Bs). Único teste que consome tempo
// real (~1s) — os demais são resolvidos sem esperar prazos.
void test_rx_timeout() {
    FakeCanBus can;

    uint8_t req[1] = {0x03};
    uint8_t out[32];
    int n = IsoTp::request(&can, 0x7DF, 0x7E8, 0x7EF, req, 1, out, sizeof(out));

    TEST_ASSERT_EQUAL_INT(IsoTp::TIMEOUT, n);
}

// ── TX: envio de uma requisição segmentada (>7 bytes) ────────────────────
// Nenhum fluxo real do projeto hoje manda requisição >7 bytes (Mode
// 03/07/0A é só o SID); estes testes validam a máquina de estados do lado
// emissor mesmo assim, com uma requisição sintética.

void test_tx_ff_cf_cts() {
    FakeCanBus can;
    can.enqueue(0x7E8, {0x30, 0x00, 0x00});        // FC: CTS, BS=0, STmin=0
    can.enqueue(0x7E8, {0x02, 0x40, 0x00});        // resposta final (SF, SID = req[0]|0x40 = 0x40)

    uint8_t req[10] = {0, 1, 2, 3, 4, 5, 6, 7, 8, 9};
    uint8_t out[16];
    int n = IsoTp::request(&can, 0x7E0, 0x7E8, 0x7E8, req, sizeof(req), out, sizeof(out));

    TEST_ASSERT_EQUAL_INT(2, n);
    TEST_ASSERT_EQUAL_UINT32(2, can.sent.size());  // FF + 1 CF
    TEST_ASSERT_EQUAL_HEX8(0x10, can.sent[0].data[0] & 0xF0);
    TEST_ASSERT_EQUAL_HEX8(0x21, can.sent[1].data[0]);  // CF, seq=1
}

void test_tx_wait_then_cts() {
    FakeCanBus can;
    can.enqueue(0x7E8, {0x31, 0x00, 0x00});  // FC: FS=Wait
    can.enqueue(0x7E8, {0x30, 0x00, 0x00});  // FC: FS=CTS
    can.enqueue(0x7E8, {0x02, 0x40, 0x00});  // resposta final (SID = req[0]|0x40 = 0x40)

    uint8_t req[10] = {0, 1, 2, 3, 4, 5, 6, 7, 8, 9};
    uint8_t out[16];
    int n = IsoTp::request(&can, 0x7E0, 0x7E8, 0x7E8, req, sizeof(req), out, sizeof(out));

    TEST_ASSERT_EQUAL_INT(2, n);
    TEST_ASSERT_EQUAL_UINT32(2, can.sent.size());  // FF + 1 CF (Wait não reenvia FF)
}

void test_tx_overflow() {
    FakeCanBus can;
    can.enqueue(0x7E8, {0x32, 0x00, 0x00});  // FC: FS=Overflow

    uint8_t req[10] = {0, 1, 2, 3, 4, 5, 6, 7, 8, 9};
    uint8_t out[16];
    int n = IsoTp::request(&can, 0x7E0, 0x7E8, 0x7E8, req, sizeof(req), out, sizeof(out));

    TEST_ASSERT_EQUAL_INT(IsoTp::OVERFLOW_ABORT, n);
    TEST_ASSERT_EQUAL_UINT32(1, can.sent.size());  // só o FF, nenhum CF
}

// Block Size = 1: só pode mandar 1 CF por FC recebido.
void test_tx_block_size() {
    FakeCanBus can;
    can.enqueue(0x7E8, {0x30, 0x01, 0x00});  // FC: CTS, BS=1
    can.enqueue(0x7E8, {0x30, 0x00, 0x00});  // FC: CTS, BS=0 (libera o resto)
    can.enqueue(0x7E8, {0x02, 0x40, 0x00});  // resposta final (SID = req[0]|0x40 = 0x40)

    uint8_t req[20];
    for (int i = 0; i < 20; i++) req[i] = (uint8_t)i;
    uint8_t out[16];
    int n = IsoTp::request(&can, 0x7E0, 0x7E8, 0x7E8, req, sizeof(req), out, sizeof(out));

    TEST_ASSERT_EQUAL_INT(2, n);
    TEST_ASSERT_EQUAL_UINT32(3, can.sent.size());  // FF + CF(seq1) + CF(seq2)
    TEST_ASSERT_EQUAL_HEX8(0x21, can.sent[1].data[0]);
    TEST_ASSERT_EQUAL_HEX8(0x22, can.sent[2].data[0]);
}

int main(int argc, char** argv) {
    (void)argc; (void)argv;
    UNITY_BEGIN();
    RUN_TEST(test_rx_sf_clean);
    RUN_TEST(test_rx_ff_one_cf);
    RUN_TEST(test_rx_ff_two_cf);
    RUN_TEST(test_rx_negative_response);
    RUN_TEST(test_rx_multi_ecu_single_frame);
    RUN_TEST(test_rx_multi_ecu_interleaved_multiframe);
    RUN_TEST(test_rx_timeout);
    RUN_TEST(test_tx_ff_cf_cts);
    RUN_TEST(test_tx_wait_then_cts);
    RUN_TEST(test_tx_overflow);
    RUN_TEST(test_tx_block_size);
    return UNITY_END();
}
