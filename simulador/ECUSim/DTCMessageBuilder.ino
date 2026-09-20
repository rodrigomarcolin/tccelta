#include "DTCMessageBuilder.h"

int buildDtcListMessage(byte* const returnBuf, uint8_t& returnByteCount, const EcuState_t& ecuState, const uint8_t requestedSid)
{
  const uint16_t* list;
  uint8_t count;

  switch (requestedSid)
  {
  case 0x03:
    list = ecuState.confirmedDtc;
    count = ecuState.confirmedCount;
    break;
  case 0x07:
    list = ecuState.pendingDtc;
    count = ecuState.pendingCount;
    break;
  case 0x0A:
    list = ecuState.permanentDtc;
    count = ecuState.permanentCount;
    break;
  default:
    return buildNegativeResponse(returnBuf, returnByteCount, requestedSid, NRC_SERVICE_NOT_SUPPORTED);
  }

  returnBuf[0] = requestedSid + 0x40;
  returnBuf[1] = count;

  uint8_t byteOffset = 2;
  for (uint8_t i = 0; i < count; i++)
  {
    returnBuf[byteOffset]     = (byte)((list[i] >> 8) & 0xFF);
    returnBuf[byteOffset + 1] = (byte)(list[i] & 0xFF);
    byteOffset += 2;
  }

  returnByteCount = byteOffset;
  return NOERROR;
}

int buildFreezeFrameMessage(byte* const returnBuf, uint8_t& returnByteCount, const EcuState_t& ecuState, const uint8_t requestedPID, const uint8_t requestedFrame)
{
  // Only frame #0 (the frame captured when the DTC was set) is modeled.
  if (requestedFrame != 0 || !ecuState.freezeFrame.valid)
    return buildNegativeResponse(returnBuf, returnByteCount, 0x02, NRC_REQUEST_OUT_OF_RANGE);

  returnBuf[0] = 0x02 + 0x40;
  returnBuf[1] = requestedPID;
  returnBuf[2] = requestedFrame;

  if (requestedPID == 0x02) // PID 02: DTC that caused the freeze frame to be stored
  {
    returnBuf[3] = (byte)((ecuState.freezeFrame.originDtc >> 8) & 0xFF);
    returnBuf[4] = (byte)(ecuState.freezeFrame.originDtc & 0xFF);
    returnByteCount = 5;
    return NOERROR;
  }

  const int snapshotOffset = freezeFramePidOffset(requestedPID);
  const uint8_t pidLen = pgm_read_byte(PIDByteLengthMap + requestedPID);
  if (snapshotOffset < 0 || pidLen == 0)
    return buildNegativeResponse(returnBuf, returnByteCount, 0x02, NRC_REQUEST_OUT_OF_RANGE);

  for (uint8_t i = 0; i < pidLen; i++)
    returnBuf[3 + i] = ecuState.freezeFrame.snapshot[snapshotOffset + i];

  returnByteCount = 3 + pidLen;
  return NOERROR;
}

int buildNegativeResponse(byte* const returnBuf, uint8_t& returnByteCount, const uint8_t requestedSid, const byte nrc)
{
  returnBuf[0] = 0x7F;
  returnBuf[1] = requestedSid;
  returnBuf[2] = nrc;
  returnByteCount = 3;
  return NOERROR;
}
