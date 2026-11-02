#include "LatencyControl.h"
#include <math.h>

// latência_ms = baseline + max(0, offset), saturada em 65535.
//   F: offset = 0
//   U: offset ~ Uniforme[A, B]
//   N: offset ~ Normal(média A, desvio B)
//   E: offset ~ Exponencial(média A)
struct LatencyCfg_t
{
  char     mode;   // 'F', 'U', 'N' ou 'E'
  uint16_t base;
  uint16_t a;
  uint16_t b;
};

struct LatencyOverride_t
{
  byte         pid;
  LatencyCfg_t cfg;
};

constexpr uint8_t LATENCY_MAX_OVERRIDES = 16;
constexpr byte    LATENCY_PID_DEFAULT   = 0xFF;

static LatencyCfg_t      latencyDefault;
static LatencyOverride_t latencyOverrides[LATENCY_MAX_OVERRIDES];
static uint8_t           latencyOverrideCount = 0;

int  parseHexWord(const char* hexChars, uint16_t* ret);
int  parseOneByteHex(char *hexChars, byte *ret);
static void flushLatencySerialInbuf();
static const LatencyCfg_t& latencyCfgForPid(byte pid);
static uint16_t sampleLatencyMs(const LatencyCfg_t& cfg);
static float uniform01();

bool isLatencyControlMessage(char firstChar)
{
  return firstChar == 'Y';
}

void initializeLatency()
{
  latencyDefault = { 'F', 0, 0, 0 };
  latencyOverrideCount = 0;
  randomSeed((unsigned long)analogRead(A0) ^ micros());
}

static void flushLatencySerialInbuf()
{
  if (LATENCY_ERROR)
    Serial.println(F("Flush serial port input buffer (latency channel).."));

  while (Serial.available() > 0)
    Serial.read();
}

// Uniforme em (0, 1]; nunca 0, para log() ser seguro.
static float uniform01()
{
  return (float)random(1, 10001) / 10000.0f;
}

static const LatencyCfg_t& latencyCfgForPid(byte pid)
{
  for (uint8_t i = 0; i < latencyOverrideCount; i++)
    if (latencyOverrides[i].pid == pid)
      return latencyOverrides[i].cfg;

  return latencyDefault;
}

static uint16_t sampleLatencyMs(const LatencyCfg_t& cfg)
{
  float offset = 0.0f;
  switch (cfg.mode)
  {
  case 'U':
    offset = (float)random((long)cfg.a, (long)cfg.b + 1);
    break;
  case 'N':
    offset = (float)cfg.a + (float)cfg.b * sqrt(-2.0f * log(uniform01())) * cos(2.0f * PI * uniform01());
    break;
  case 'E':
    offset = -(float)cfg.a * log(uniform01());
    break;
  default:
    break;
  }

  if (offset < 0.0f)
    offset = 0.0f;

  float total = (float)cfg.base + offset;
  if (total > 65535.0f)
    total = 65535.0f;

  return (uint16_t)(total + 0.5f);
}

uint16_t computeResponseLatencyMs(const byte* request, uint8_t length)
{
  uint16_t latency = 0;

  if (request[0] == 0x01 && length >= 2)
  {
    for (uint8_t i = 1; i < length; i++)
    {
      uint16_t l = sampleLatencyMs(latencyCfgForPid(request[i]));
      if (l > latency)
        latency = l;
    }
  }
  else
  {
    latency = sampleLatencyMs(latencyDefault);
  }

  if (LATENCY_DEBUG)
  {
    Serial.print(F("Latency (ms): "));
    Serial.println(latency);
  }
  return latency;
}

// Formato (17 bytes): Y M PP BBBB AAAA CCCC \n
void parseLatencyUpdateMessage()
{
  char buf[LATENCY_SERIAL_MSG_LENGTH];
  for (int i = 0; i < LATENCY_SERIAL_MSG_LENGTH; i++)
    buf[i] = (char)Serial.read();

  if (buf[LATENCY_SERIAL_MSG_LENGTH - 1] != '\n')
  {
    if (LATENCY_ERROR)
      Serial.println(F("ERROR: Latency message is not finished with LF."));
    flushLatencySerialInbuf();
    return;
  }

  const char mode = buf[1];
  byte pid;
  uint16_t base, a, b;
  if (parseOneByteHex(&buf[2], &pid) != 0 ||
      parseHexWord(&buf[4], &base) != 0 ||
      parseHexWord(&buf[8], &a) != 0 ||
      parseHexWord(&buf[12], &b) != 0)
  {
    if (LATENCY_ERROR)
      Serial.println(F("ERROR: Latency message parse is failed."));
    flushLatencySerialInbuf();
    return;
  }

  if (mode == 'C')
  {
    if (pid == LATENCY_PID_DEFAULT)
    {
      latencyDefault = { 'F', 0, 0, 0 };
      latencyOverrideCount = 0;
    }
    else
    {
      for (uint8_t i = 0; i < latencyOverrideCount; i++)
      {
        if (latencyOverrides[i].pid == pid)
        {
          latencyOverrides[i] = latencyOverrides[--latencyOverrideCount];
          break;
        }
      }
    }
    if (LATENCY_DEBUG)
      Serial.println(F("Latency cleared."));
    return;
  }

  if (mode != 'F' && mode != 'U' && mode != 'N' && mode != 'E')
  {
    if (LATENCY_ERROR)
      Serial.println(F("ERROR: Latency mode must be F, U, N, E or C."));
    return;
  }
  if (mode == 'U' && a > b)
  {
    if (LATENCY_ERROR)
      Serial.println(F("ERROR: Latency uniform needs min <= max."));
    return;
  }

  const LatencyCfg_t cfg = { mode, base, a, b };

  if (pid == LATENCY_PID_DEFAULT)
  {
    latencyDefault = cfg;
  }
  else
  {
    uint8_t slot = latencyOverrideCount;
    for (uint8_t i = 0; i < latencyOverrideCount; i++)
      if (latencyOverrides[i].pid == pid)
        slot = i;

    if (slot == LATENCY_MAX_OVERRIDES)
    {
      if (LATENCY_ERROR)
        Serial.println(F("ERROR: Latency override table is full."));
      return;
    }
    if (slot == latencyOverrideCount)
      latencyOverrideCount++;
    latencyOverrides[slot].pid = pid;
    latencyOverrides[slot].cfg = cfg;
  }

  if (LATENCY_DEBUG)
    Serial.println(F("Latency set Ok"));
}
