#include "ECUSim.h"
#include "PIDUpdateSerialControl.h"
#include "DTCUpdateSerialControl.h"
#include "LatencyControl.h"
#include "CANMesasgeHandle.h"

byte PID_Value_Map[PIDMemSize];
MCP_CAN CAN(10); // CAN CS: pin 10

void setup()
{
  Serial.begin(115200);
  Serial.println(F("------------------- Arduino setup start ---------------------------"));
  initializePIDValueMap();
  initializeEcuState(ecmState);
  initializeEcuState(tcmState);
  initializeLatency();
  initializeCAN();
}

void pollSerial()
{
  if (Serial.available() > 0)
  {
    const char first = (char)Serial.peek();
    if (isDtcControlMessage(first))
    {
      if (Serial.available() >= DTC_SERIAL_MSG_LENGTH)
        parseDTCUpdateMessage();
    }
    else if (isLatencyControlMessage(first))
    {
      if (Serial.available() >= LATENCY_SERIAL_MSG_LENGTH)
        parseLatencyUpdateMessage();
    }
    else if (Serial.available() >= SERIAL_MSG_LENGTH)
    {
      parsePIDUPdateMessage();
    }
  }
}

void loop()
{
  pollSerial();
  if (CAN.checkReceive() == CAN_MSGAVAIL)
    handleCANMessage();
}

