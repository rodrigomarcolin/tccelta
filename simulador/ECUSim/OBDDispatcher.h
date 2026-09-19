#ifndef OBDDISPATCHER_H
#define OBDDISPATCHER_H

#include "ECUStateModel.h"

// requestBuf[0] is the service ID (SID); requestLength counts the SID itself.
int dispatchOBDRequest(EcuState_t& ecuState, const byte* requestBuf, const uint8_t requestLength, byte* const returnBuf, uint8_t& returnByteCount);

#endif
