#include "CANMesasgeHandle.h"
#include "PIDMessageBuilder.h"
#include "OBDDispatcher.h"
#include "AVRFreeRAM.h"

IsoTp isotp(&CAN, 0);

constexpr int RETURN_MSGBUILD_BUF_LENGTH = 128;

struct EcuConfig_t
{
  unsigned long requestId;
  unsigned long responseId;
  EcuState_t*   state;
};

constexpr uint8_t ECU_COUNT = 2;
EcuConfig_t ECU_CONFIGS[ECU_COUNT] =
{
  { ECM_CAN_ID, ECM_CAN_RESPONSE_ID, &ecmState },
  { TCM_CAN_ID, TCM_CAN_RESPONSE_ID, &tcmState },
};

void respondToObdRequest(const EcuConfig_t& ecuConfig, const byte* receivedCANBuf, unsigned long canMsgHandleStartTime);

void initializeCAN()
{
  bool initSucess = false;
  while (!initSucess)
  {
    if (CAN_OK == CAN.begin(MCP_ANY, CAN_250KBPS, MCP_8MHZ)) // init can bus : baudrate = 250k
    {
      Serial.println(F("CAN BUS Shield init ok!"));
      initSucess = true;
      CAN.setMode(MCP_NORMAL);   
    }
    else
    {
      Serial.println(F("CAN BUS Shield init fail"));
      Serial.println(F("Init CAN BUS Shield again"));
      delay(100);
      initSucess = false;
    }
  }
}

void handleCANMessage()
{
  unsigned long canMsgHandleStartTime;
  if (CANMSG_TIME_MEAS)
    canMsgHandleStartTime = micros();

  if (CANMSG_DEBUG)
    Serial.println(F("CAN message handle start."));

  byte receivedCANBuf[CAN_PAYLOAD_LENGTH];
  unsigned long canId;
  unsigned char len;
  uint8_t recvResult;

  recvResult = CAN.readMsgBuf(&canId, &len, receivedCANBuf);

  if (len > CAN_PAYLOAD_LENGTH)
  {
    if (CANMSG_FATAL)
      Serial.println(F("FATAL: CAN read message length exceed CAN_PAYLOAD_LENGTH."));

    return;
  }

  if(CANMSG_ERROR)
  {
    if(recvResult != CAN_OK) {
      Serial.print(F("ERROR: CAN message receive is failed. Return of readMsgBuf : "));
      Serial.println(recvResult);
    }
  }

  if (CANMSG_DEBUG)
  {
    Serial.print(F("MCP read result code: "));
    Serial.println(recvResult);
    Serial.print(F("Msg from canId: "));
    Serial.print(canId, HEX);
    Serial.print(F(" Msg length: "));
    Serial.print(len);
    Serial.print(F(" Msg content: "));
    for(int i = 0; i < CAN_PAYLOAD_LENGTH; i++)
    {
      Serial.print(receivedCANBuf[i], HEX);
      if (i == CAN_PAYLOAD_LENGTH - 1)
        Serial.println();
      else
        Serial.print(",");
    }
  }

  // Wait time
  if(ECU_WAIT > 0)
    delay(ECU_WAIT);

  // Match the request against every simulated ECU (0x7DF is a functional broadcast: all ECUs answer).
  bool matchedAnyEcu = false;
  for (uint8_t e = 0; e < ECU_COUNT; e++)
  {
    if (canId != OBD_BROADCAST_CAN_ID && canId != ECU_CONFIGS[e].requestId)
      continue;

    matchedAnyEcu = true;
    respondToObdRequest(ECU_CONFIGS[e], receivedCANBuf, canMsgHandleStartTime);
  }

  if (!matchedAnyEcu)
  {
    if (CANMSG_DEBUG)
      Serial.println(F("CAN ID do not match with any simulated ECU."));

    return;
  }

  if(CANMSG_FREERAM_MEAS)
    display_freeram();
}

void respondToObdRequest(const EcuConfig_t& ecuConfig, const byte* receivedCANBuf, unsigned long canMsgHandleStartTime)
{
  // Get query message length (SID + data bytes).
  const uint8_t queryMessageLength = receivedCANBuf[0];

  if(queryMessageLength < 1 || queryMessageLength > 7)
  {
    if (CANMSG_ERROR)
      Serial.println(F("ERROR: CAN query message length needs to be between 1 and 7 (SID + up to 6 data bytes)."));

    return;
  }

  if (CANMSG_DEBUG)
  {
    Serial.print(F("OBD request (SID + data): "));
    for(uint8_t i = 0; i < queryMessageLength; i++)
    {
      Serial.print(receivedCANBuf[i + 1], HEX);
      if(i == queryMessageLength - 1)
        Serial.println();
      else
        Serial.print(F(","));
    }
  }

  // Build up CAN return message
  byte returnMessageBuf[RETURN_MSGBUILD_BUF_LENGTH];
  uint8_t returnByteCount;

  int dispatchResult = dispatchOBDRequest(*ecuConfig.state, &receivedCANBuf[1], queryMessageLength, returnMessageBuf, returnByteCount);
  if (dispatchResult == PID_NOT_AVAILABLE)
  {
    if (CANMSG_ERROR)
      Serial.println(F("ERROR: CAN query PID is not supported."));
    return;
  }

  // Send CAN return message.
  struct Message_t txMsg;
  uint8_t sendResult;
  txMsg.len = returnByteCount;
  txMsg.rx_id = ecuConfig.requestId;
  txMsg.tx_id = ecuConfig.responseId;
  txMsg.Buffer = returnMessageBuf;
  sendResult = isotp.send(&txMsg);

  if(CANMSG_ERROR)
  {
    if(sendResult != CAN_OK) {
      Serial.print(F("ERROR: CAN message send is failed. Return of sendMsg : "));
      Serial.println(sendResult);
    }
  }

  if(CANMSG_TIME_MEAS)
  {
    Serial.print(F("CAN message handle time (micros): "));
    Serial.println(micros() - canMsgHandleStartTime);
  }
  if (CANMSG_DEBUG)
  {
    Serial.print(F("Return. Value (with padding): "));
    for (int i = 0; i < returnByteCount; i++)
    {
      Serial.print(returnMessageBuf[i], HEX);
      if (i == returnByteCount - 1)
        Serial.println();
      else
        Serial.print(",");
    }

    Serial.print(F("MCP send result code :"));
    Serial.println(sendResult);
  }
}
