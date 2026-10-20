#include "collector/DataProtocol.h"
#include <cstring>

namespace collector {

namespace {

constexpr size_t kAckBodySize          = 1 + 4;
constexpr size_t kTimeResponseBodySize = 1 + 2 + 4;

}  // namespace

size_t DataProtocol::seal(uint8_t* out, size_t bodyLen, size_t cap) {
    const size_t total = bodyLen + _auth.tagSize();
    if (total > cap) return 0;
    _auth.sign(out, bodyLen, out + bodyLen);
    return total;
}

int DataProtocol::open(const uint8_t* msg, size_t len) {
    const size_t tag = _auth.tagSize();
    if (len < 1 + tag) return -1;
    const size_t bodyLen = len - tag;
    if (!_auth.verify(msg, bodyLen, msg + bodyLen)) return -1;
    return static_cast<int>(bodyLen);
}

size_t DataProtocol::encodeBatch(uint32_t oldestSeq, const Block& block, uint8_t* out, size_t cap) {
    const size_t bodyLen = 1 + 4 + block.length;
    if (bodyLen > cap) return 0;
    out[0] = static_cast<uint8_t>(MessageType::Batch);
    putU32(&out[1], oldestSeq);
    memcpy(&out[5], block.bytes, block.length);
    return seal(out, bodyLen, cap);
}

size_t DataProtocol::encodeTimeRequest(const TimeExchange& request, uint8_t* out, size_t cap) {
    if (cap < 7) return 0;
    out[0] = static_cast<uint8_t>(MessageType::TimeRequest);
    putU16(&out[1], request.bootId);
    putU32(&out[3], request.t1Ms);
    return seal(out, 7, cap);
}

size_t DataProtocol::encodeTimeReport(const TimeExchange& report, uint8_t* out, size_t cap) {
    if (cap < 11) return 0;
    out[0] = static_cast<uint8_t>(MessageType::TimeReport);
    putU16(&out[1], report.bootId);
    putU32(&out[3], report.t1Ms);
    putU32(&out[7], report.t2Ms);
    return seal(out, 11, cap);
}

bool DataProtocol::verifiedType(const uint8_t* msg, size_t len, MessageType& type) {
    if (open(msg, len) < 0) return false;
    type = static_cast<MessageType>(msg[0]);
    return true;
}

bool DataProtocol::decodeAck(const uint8_t* msg, size_t len, uint32_t& seq) {
    if (open(msg, len) != static_cast<int>(kAckBodySize)) return false;
    if (msg[0] != static_cast<uint8_t>(MessageType::Ack)) return false;
    seq = getU32(&msg[1]);
    return true;
}

bool DataProtocol::decodeTimeResponse(const uint8_t* msg, size_t len, TimeExchange& response) {
    if (open(msg, len) != static_cast<int>(kTimeResponseBodySize)) return false;
    if (msg[0] != static_cast<uint8_t>(MessageType::TimeResponse)) return false;
    response.bootId = getU16(&msg[1]);
    response.t1Ms   = getU32(&msg[3]);
    response.t2Ms   = 0;
    return true;
}

size_t frameMessage(const uint8_t* msg, size_t len, uint8_t* out, size_t cap) {
    if (len > UINT16_MAX || len + kFrameHeaderSize > cap) return 0;
    putU16(out, static_cast<uint16_t>(len));
    memcpy(out + kFrameHeaderSize, msg, len);
    return len + kFrameHeaderSize;
}

}  // namespace collector
