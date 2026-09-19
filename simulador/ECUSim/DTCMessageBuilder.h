#ifndef DTCMESSAGEBUILDER_H
#define DTCMESSAGEBUILDER_H

#include "ECUStateModel.h"
#include "PIDMessageBuilder.h"

constexpr byte NRC_SERVICE_NOT_SUPPORTED = 0x11;
constexpr byte NRC_INVALID_FORMAT = 0x12;
constexpr byte NRC_REQUEST_OUT_OF_RANGE = 0x31;

int buildDtcListMessage(byte* const returnBuf, uint8_t& returnByteCount, const EcuState_t& ecuState, const uint8_t requestedSid);
int buildFreezeFrameMessage(byte* const returnBuf, uint8_t& returnByteCount, const EcuState_t& ecuState, const uint8_t requestedPID, const uint8_t requestedFrame);
int buildNegativeResponse(byte* const returnBuf, uint8_t& returnByteCount, const uint8_t requestedSid, const byte nrc);

#endif
