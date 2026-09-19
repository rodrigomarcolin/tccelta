#include "DTCUpdateSerialControl.h"

void flushDtcSerialInbuf();
int  parseHexNibble(char hexChar, byte* ret);
int  parseHexWord(const char* hexChars, uint16_t* ret);
EcuState_t* resolveEcuState(char ecuChar);
uint16_t*   resolveDtcList(EcuState_t& ecu, char listChar, uint8_t** countOut);

// Command letters are never valid hex digits, so a leading byte alone tells the
// DTC control protocol (9 bytes) apart from the PID control protocol (11 bytes).
bool isDtcControlMessage(char firstChar)
{
  return firstChar == 'S' || firstChar == 'R' || firstChar == 'Z' || firstChar == 'L' || firstChar == 'Q';
}

void flushDtcSerialInbuf()
{
  if (DTC_ERROR)
    Serial.println(F("Flush serial port input buffer (DTC channel).."));

  while (Serial.read() != -1)
    Serial.read();
}

int parseHexNibble(char hexChar, byte* ret)
{
  if (hexChar >= '0' && hexChar <= '9')
    *ret = hexChar - '0';
  else if (hexChar >= 'a' && hexChar <= 'f')
    *ret = hexChar - 'a' + 10;
  else if (hexChar >= 'A' && hexChar <= 'F')
    *ret = hexChar - 'A' + 10;
  else
    return -1;

  return 0;
}

int parseHexWord(const char* hexChars, uint16_t* ret)
{
  uint16_t val = 0;
  for (int i = 0; i < 4; i++)
  {
    byte nibble;
    if (parseHexNibble(hexChars[i], &nibble) != 0)
      return -1;
    val = (val << 4) | nibble;
  }

  *ret = val;
  return 0;
}

EcuState_t* resolveEcuState(char ecuChar)
{
  if (ecuChar == 'E')
    return &ecmState;
  if (ecuChar == 'T')
    return &tcmState;

  return nullptr;
}

uint16_t* resolveDtcList(EcuState_t& ecu, char listChar, uint8_t** countOut)
{
  switch (listChar)
  {
  case 'C':
    *countOut = &ecu.confirmedCount;
    return ecu.confirmedDtc;
  case 'P':
    *countOut = &ecu.pendingCount;
    return ecu.pendingDtc;
  case 'M':
    *countOut = &ecu.permanentCount;
    return ecu.permanentDtc;
  default:
    *countOut = nullptr;
    return nullptr;
  }
}

void parseDTCUpdateMessage()
{
  if (DTC_DEBUG)
    Serial.println(F("DTC control message start."));

  char serialBuf[DTC_SERIAL_MSG_LENGTH];
  for (int i = 0; i < DTC_SERIAL_MSG_LENGTH; i++)
    serialBuf[i] = (char)Serial.read();

  if (serialBuf[DTC_SERIAL_MSG_LENGTH - 1] != '\n')
  {
    if (DTC_ERROR)
      Serial.println(F("ERROR: DTC control message is not finished with LF."));

    flushDtcSerialInbuf();
    return;
  }

  const char cmdChar  = serialBuf[0];
  const char ecuChar  = serialBuf[1];
  const char listChar = serialBuf[2];

  EcuState_t* ecu = resolveEcuState(ecuChar);
  if (ecu == nullptr)
  {
    if (DTC_ERROR)
      Serial.println(F("ERROR: Unknown or not-yet-available ECU selector in DTC control message."));

    return;
  }

  if (cmdChar == 'L')
  {
    byte milDigit;
    if (parseHexNibble(serialBuf[3], &milDigit) != 0)
    {
      if (DTC_ERROR)
        Serial.println(F("ERROR: Invalid MIL value in DTC control message."));

      return;
    }

    ecu->mil = (milDigit != 0);

    if (DTC_DEBUG)
    {
      Serial.print(F("MIL set to: "));
      Serial.println(ecu->mil);
    }

    return;
  }

  if (cmdChar == 'Q')
  {
    uint16_t dtcCode;
    if (parseHexWord(&serialBuf[3], &dtcCode) != 0)
    {
      if (DTC_ERROR)
        Serial.println(F("ERROR: Invalid DTC code in DTC control message."));

      return;
    }

    captureFreezeFrame(*ecu, dtcCode);

    if (DTC_DEBUG)
      Serial.println(F("Freeze frame capture forced."));

    return;
  }

  uint8_t* count;
  uint16_t* list = resolveDtcList(*ecu, listChar, &count);

  if (cmdChar == 'Z')
  {
    if (list == nullptr)
    {
      if (DTC_ERROR)
        Serial.println(F("ERROR: Invalid DTC list selector in DTC control message."));

      return;
    }

    dtcListClear(*count);

    if (DTC_DEBUG)
      Serial.println(F("DTC list cleared."));

    return;
  }

  if (list == nullptr)
  {
    if (DTC_ERROR)
      Serial.println(F("ERROR: Invalid DTC list selector in DTC control message."));

    return;
  }

  uint16_t dtcCode;
  if (parseHexWord(&serialBuf[3], &dtcCode) != 0)
  {
    if (DTC_ERROR)
      Serial.println(F("ERROR: Invalid DTC code in DTC control message."));

    return;
  }

  switch (cmdChar)
  {
  case 'S':
    dtcListAdd(list, *count, MAX_DTC_PER_LIST, dtcCode);
    if (listChar == 'C')
      captureFreezeFrame(*ecu, dtcCode);
    break;
  case 'R':
    dtcListRemove(list, *count, dtcCode);
    break;
  default:
    if (DTC_ERROR)
      Serial.println(F("ERROR: Unknown DTC control command."));
    return;
  }

  if (DTC_DEBUG)
  {
    Serial.print(F("DTC list count now: "));
    Serial.println(*count);
  }
}
