#include "DTCMap_Definition.h"

uint8_t lookupDtcFlags(uint16_t dtcCode)
{
  for (uint8_t i = 0; i < DTC_CATALOG_SIZE; i++)
  {
    DTCCatalogEntry_t entry;
    memcpy_P(&entry, &DTCCatalog[i], sizeof(DTCCatalogEntry_t));
    if (entry.code == dtcCode)
      return entry.flags;
  }

  return 0x00; // Unknown DTC: no priority flag.
}

int freezeFramePidOffset(uint8_t pid)
{
  uint8_t offset = 0;
  for (uint8_t i = 0; i < FREEZE_FRAME_PID_COUNT; i++)
  {
    if (FREEZE_FRAME_PIDS[i] == pid)
      return offset;
    offset += _PID_BYTE_LENGTH[FREEZE_FRAME_PIDS[i]];
  }

  return -1;
}
