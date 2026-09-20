#include "OBDDispatcher.h"
#include "PIDMessageBuilder.h"
#include "DTCMessageBuilder.h"

int dispatchMode01(EcuState_t& ecuState, const byte* pidList, const uint8_t pidCount, byte* const returnBuf, uint8_t& returnByteCount);

int dispatchOBDRequest(EcuState_t& ecuState, const byte* requestBuf, const uint8_t requestLength, byte* const returnBuf, uint8_t& returnByteCount)
{
  const uint8_t sid = requestBuf[0];

  switch (sid)
  {
  case 0x01: // Show current data: needs SID + at least 1 PID
    if (requestLength < 2)
      return buildNegativeResponse(returnBuf, returnByteCount, sid, NRC_INVALID_FORMAT);
    return dispatchMode01(ecuState, requestBuf + 1, requestLength - 1, returnBuf, returnByteCount);

  case 0x02: // Show freeze frame data: needs SID + PID + frame number
    if (requestLength != 3)
      return buildNegativeResponse(returnBuf, returnByteCount, sid, NRC_INVALID_FORMAT);
    return buildFreezeFrameMessage(returnBuf, returnByteCount, ecuState, requestBuf[1], requestBuf[2]);

  case 0x03: // Show stored DTCs
  case 0x07: // Show pending DTCs
  case 0x0A: // Show permanent DTCs
    if (requestLength != 1)
      return buildNegativeResponse(returnBuf, returnByteCount, sid, NRC_INVALID_FORMAT);
    return buildDtcListMessage(returnBuf, returnByteCount, ecuState, sid);

  default: // 04+ and anything else: out of scope
    return buildNegativeResponse(returnBuf, returnByteCount, sid, NRC_SERVICE_NOT_SUPPORTED);
  }
}

int dispatchMode01(EcuState_t& ecuState, const byte* pidList, const uint8_t pidCount, byte* const returnBuf, uint8_t& returnByteCount)
{
  const uint8_t returnServiceMode = 0x01 + 0x40;
  return buildPIDValueMessage(returnBuf, returnByteCount, pidList, pidCount, returnServiceMode, ecuState);
}
