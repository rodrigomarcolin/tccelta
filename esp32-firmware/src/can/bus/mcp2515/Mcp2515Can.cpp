// Só compila no env collector_mcp2515: com ESP-IDF o build_src_filter não vale
// (o src/CMakeLists.txt pega tudo) e a biblioteca autowp-mcp2515 só existe nele.
#if defined(USE_MCP2515)
#include "Mcp2515Can.h"
#include <cstring>

Mcp2515Can::Mcp2515Can(CanBitrate bitrate, uint8_t csPin, CAN_CLOCK clock)
    : _mcp(csPin),
      _bitrate(bitrate == CanBitrate::Kbps500 ? CAN_500KBPS : CAN_250KBPS),
      _clock(clock) {}

bool Mcp2515Can::begin() {
    _mcp.reset();
    if (_mcp.setBitrate(_bitrate, _clock) != MCP2515::ERROR_OK) return false;
    return _mcp.setNormalMode() == MCP2515::ERROR_OK;
}

bool Mcp2515Can::send(const CanFrame& frame) {
    struct can_frame f = {};
    f.can_id  = frame.extended ? (frame.id | CAN_EFF_FLAG) : frame.id;
    f.can_dlc = frame.dlc;
    memcpy(f.data, frame.data, frame.dlc);
    return _mcp.sendMessage(&f) == MCP2515::ERROR_OK;
}

bool Mcp2515Can::receive(CanFrame& frame) {
    struct can_frame f;
    if (_mcp.readMessage(&f) != MCP2515::ERROR_OK) return false;
    frame.extended = (f.can_id & CAN_EFF_FLAG) != 0;
    frame.id  = frame.extended ? (f.can_id & CAN_EFF_MASK) : (f.can_id & CAN_SFF_MASK);
    frame.dlc = f.can_dlc;
    memcpy(frame.data, f.data, f.can_dlc);
    return true;
}
#endif  // USE_MCP2515
