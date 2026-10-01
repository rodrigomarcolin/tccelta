#include "Mcp2515Can.h"
#include <cstring>

Mcp2515Can::Mcp2515Can(uint8_t csPin, CAN_SPEED bitrate, CAN_CLOCK clock)
    : _mcp(csPin), _bitrate(bitrate), _clock(clock) {}

bool Mcp2515Can::begin() {
    _mcp.reset();
    if (_mcp.setBitrate(_bitrate, _clock) != MCP2515::ERROR_OK) return false;
    return _mcp.setNormalMode() == MCP2515::ERROR_OK;
}

bool Mcp2515Can::send(const CanFrame& frame) {
    struct can_frame f = {};
    f.can_id  = frame.id;
    f.can_dlc = frame.dlc;
    memcpy(f.data, frame.data, frame.dlc);
    return _mcp.sendMessage(&f) == MCP2515::ERROR_OK;
}

bool Mcp2515Can::receive(CanFrame& frame) {
    struct can_frame f;
    if (_mcp.readMessage(&f) != MCP2515::ERROR_OK) return false;
    frame.id  = f.can_id;
    frame.dlc = f.can_dlc;
    memcpy(frame.data, f.data, f.can_dlc);
    return true;
}
