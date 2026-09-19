#ifndef DTCMAP_DEFINITION_H
#define DTCMAP_DEFINITION_H

#include <avr/pgmspace.h>
#include "PIDMap_Definition.h"

// DTC catalog entry. flags bit0 = misfire/fuel-system group (freeze frame priority rule).
constexpr uint8_t DTC_FLAG_MISFIRE_FUEL = 0x01;

struct DTCCatalogEntry_t
{
  uint16_t code;
  uint8_t  flags;
};

constexpr DTCCatalogEntry_t _DTC_CATALOG[] =
{
  {0x0301, DTC_FLAG_MISFIRE_FUEL}, // P0301 - Cylinder 1 Misfire Detected
  {0x0171, DTC_FLAG_MISFIRE_FUEL}, // P0171 - System Too Lean (Bank 1)
  {0x0133, 0x00},                  // P0133 - O2 Sensor Circuit Slow Response
  {0x0420, 0x00},                  // P0420 - Catalyst System Efficiency Below Threshold
  {0x0700, 0x00},                  // P0700 - Transmission Control System Malfunction
};

constexpr uint8_t DTC_CATALOG_SIZE = sizeof(_DTC_CATALOG) / sizeof(_DTC_CATALOG[0]);

PROGMEM const DTCCatalogEntry_t DTCCatalog[DTC_CATALOG_SIZE] =
{
  _DTC_CATALOG[0],
  _DTC_CATALOG[1],
  _DTC_CATALOG[2],
  _DTC_CATALOG[3],
  _DTC_CATALOG[4],
};

// Freeze frame content: PIDs captured verbatim from PID_Value_Map, same layout as Mode 01.
constexpr uint8_t FREEZE_FRAME_PIDS[] = {0x04, 0x05, 0x0C, 0x0D, 0x0F, 0x11};
constexpr uint8_t FREEZE_FRAME_PID_COUNT = sizeof(FREEZE_FRAME_PIDS) / sizeof(FREEZE_FRAME_PIDS[0]);

constexpr uint8_t calcFreezeFrameSize(uint8_t index);
constexpr uint8_t calcFreezeFrameSize(uint8_t index)
{
    return (index == FREEZE_FRAME_PID_COUNT) ? 0 : (_PID_BYTE_LENGTH[FREEZE_FRAME_PIDS[index]] + calcFreezeFrameSize(index + 1));
}

constexpr uint8_t FREEZE_FRAME_SNAPSHOT_SIZE = calcFreezeFrameSize(0);

uint8_t lookupDtcFlags(uint16_t dtcCode);
int     freezeFramePidOffset(uint8_t pid); // Byte offset within the snapshot buffer, or -1 if not tracked.

#endif
