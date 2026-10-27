#include "sync/SyncProtocol.h"

#include <cstring>

namespace sync {

using collector::getU16;
using collector::getU32;
using collector::putU16;
using collector::putU32;

bool parseTlv(const uint8_t* buf, size_t len, TlvView& out) {
    if (len < kTlvHeaderSize) return false;
    const uint16_t l = getU16(&buf[1]);
    if (l != len - kTlvHeaderSize) return false;
    out.type = buf[0];
    out.len = l;
    out.value = buf + kTlvHeaderSize;
    return true;
}

size_t putTlvHeader(uint8_t type, uint16_t len, uint8_t out[3]) {
    out[0] = type;
    putU16(&out[1], len);
    return kTlvHeaderSize;
}

bool decodeChallenge(const uint8_t* v, size_t len, Challenge& out) {
    if (len != kChallengeLen && len != kChallengeDiagLen) return false;
    const uint8_t flags = v[28];
    const bool diag = (flags & kChallengeFlagDiag) != 0;
    if (diag != (len == kChallengeDiagLen)) return false;
    out.cs = getU32(&v[0]);
    memcpy(out.nonce, &v[4], 16);
    out.ackSeq = getU32(&v[20]);
    out.cConf = getU32(&v[24]);
    out.flags = flags;
    out.diagMin = diag ? getU16(&v[29]) : 0;
    out.payloadLen = len - kHandshakeTagSize;
    out.tag = v + out.payloadLen;
    return true;
}

size_t encodeResponsePayload(const Response& r, uint8_t out[kResponsePayloadLen]) {
    putU32(&out[0], r.cs);
    memcpy(&out[4], r.nonce, 16);
    putU32(&out[20], r.tailSeq);
    putU32(&out[24], r.headSeq);
    putU16(&out[28], r.bootId);
    putU32(&out[30], r.tMs);
    return kResponsePayloadLen;
}

bool decodeAck(const uint8_t* v, size_t len, Ack& out) {
    if (len != kAckLen) return false;
    out.ackSeq = getU32(&v[0]);
    out.cConf = getU32(&v[4]);
    out.flags = v[8];
    out.tag = &v[kAckPayloadLen];
    return true;
}

size_t encodeAckPayload(uint32_t ackSeq, uint32_t cConf, uint8_t flags, uint8_t out[kAckPayloadLen]) {
    putU32(&out[0], ackSeq);
    putU32(&out[4], cConf);
    out[8] = flags;
    return kAckPayloadLen;
}

bool decodeBatchRequest(const uint8_t* v, size_t len, uint16_t& k) {
    if (len != kBatchRequestLen) return false;
    k = getU16(v);
    return true;
}

size_t encodeNotice(uint32_t head, uint8_t flags, const uint8_t devId[8], uint8_t out[kNoticeLen]) {
    putU32(&out[0], head);
    out[4] = flags;
    memcpy(&out[5], devId, 8);
    return kNoticeLen;
}

}  // namespace sync
