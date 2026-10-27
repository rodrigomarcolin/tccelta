#pragma once
#include <cstddef>
#include <cstdint>

#include "collector/BlockFormat.h"

/**
 * Formato de fio do serviço SYNC (spec fase-1/04 §3, §9): TLV `T(1) | L(2 LE) | V`
 * e os layouts das mensagens 0x01-0x07. Código puro, testado no native.
 */
namespace sync {

enum class MsgType : uint8_t {
    Challenge = 0x01,
    Response = 0x02,
    Block = 0x03,
    Ack = 0x04,
    BatchRequest = 0x05,
    Error = 0x06,
    Notice = 0x07,
};

/** Tabela 5 do documento de segurança. */
enum class ErrorCode : uint8_t {
    InvalidTag = 0x01,
    StaleChallenge = 0x02,
    InvalidAckSeq = 0x03,
    Incomplete = 0x04,
    NoSession = 0x05,
    ResendFromBoundary = 0x07,
};

constexpr size_t kTlvHeaderSize = 3;
constexpr size_t kHandshakeTagSize = 16;
constexpr size_t kChallengeLen = 45, kChallengeDiagLen = 47, kResponseLen = 50, kAckLen = 25;
constexpr size_t kBatchRequestLen = 2, kErrorLen = 1, kNoticeLen = 13;
constexpr size_t kChallengePayloadLen = 29, kChallengeDiagPayloadLen = 31;
constexpr size_t kResponsePayloadLen = 34, kAckPayloadLen = 9;
constexpr size_t kMaxBlockValueLen = collector::kBlockSize + collector::kMaxTagSize;  // 1088
constexpr size_t kMaxTxMessage = kTlvHeaderSize + kMaxBlockValueLen;                  // 1091
constexpr size_t kSyncRxMax = 64;

constexpr uint8_t kChallengeFlagDiag = 0x01;
constexpr uint8_t kAckFlagAnchorFailed = 0x01;
constexpr uint8_t kNoticeNoHandshakeConn = 0x01;
constexpr uint8_t kNoticeNoHandshakeBoot = 0x02;
constexpr uint8_t kNoticeFlashHigh = 0x04;

struct TlvView {
    uint8_t type;
    const uint8_t* value;
    uint16_t len;
};

/** Exige `L == len - 3`. */
bool parseTlv(const uint8_t* buf, size_t len, TlvView& out);
size_t putTlvHeader(uint8_t type, uint16_t len, uint8_t out[3]);

struct Challenge {
    uint32_t cs;
    uint8_t nonce[16];
    uint32_t ackSeq;
    uint32_t cConf;
    uint8_t flags;
    uint16_t diagMin;
    size_t payloadLen;    // bytes autenticados (V sem a tag): 29 ou 31
    const uint8_t* tag;   // aponta para dentro de `v`
};
/** Aceita 45 B com flags bit 0 = 0, ou 47 B com bit 0 = 1; bits reservados são ignorados. */
bool decodeChallenge(const uint8_t* v, size_t len, Challenge& out);

struct Response {
    uint32_t cs;
    uint8_t nonce[16];
    uint32_t tailSeq;
    uint32_t headSeq;
    uint16_t bootId;
    uint32_t tMs;
};
/** Os 34 B autenticados da Resposta (sem a tag). */
size_t encodeResponsePayload(const Response& r, uint8_t out[kResponsePayloadLen]);

struct Ack {
    uint32_t ackSeq;
    uint32_t cConf;
    uint8_t flags;
    const uint8_t* tag;
};
bool decodeAck(const uint8_t* v, size_t len, Ack& out);
size_t encodeAckPayload(uint32_t ackSeq, uint32_t cConf, uint8_t flags, uint8_t out[kAckPayloadLen]);

bool decodeBatchRequest(const uint8_t* v, size_t len, uint16_t& k);
/** Aviso 0x07 só com o V (13 B). */
size_t encodeNotice(uint32_t head, uint8_t flags, const uint8_t devId[8], uint8_t out[kNoticeLen]);

}  // namespace sync
