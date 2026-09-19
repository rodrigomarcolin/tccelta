#include "ECUStateModel.h"
#include "ECUSim.h"

EcuState_t ecmState;
EcuState_t tcmState;

void initializeEcuState(EcuState_t& ecu)
{
  ecu.mil = false;
  ecu.confirmedCount = 0;
  ecu.pendingCount = 0;
  ecu.permanentCount = 0;
  ecu.freezeFrame.valid = false;
  ecu.freezeFrame.originDtc = 0;
}

int dtcListAdd(uint16_t* list, uint8_t& count, uint8_t capacity, uint16_t dtcCode)
{
  for (uint8_t i = 0; i < count; i++)
    if (list[i] == dtcCode)
      return DTC_OK; // Already present, nothing to do.

  if (count >= capacity)
    return DTC_LIST_FULL;

  list[count] = dtcCode;
  count++;
  return DTC_OK;
}

int dtcListRemove(uint16_t* list, uint8_t& count, uint16_t dtcCode)
{
  for (uint8_t i = 0; i < count; i++)
  {
    if (list[i] == dtcCode)
    {
      list[i] = list[count - 1]; // Swap-remove; DTC list order is not significant.
      count--;
      return DTC_OK;
    }
  }
  return DTC_NOT_FOUND;
}

void dtcListClear(uint8_t& count)
{
  count = 0;
}

void captureFreezeFrame(EcuState_t& ecu, uint16_t triggeringDtc)
{
  const bool highPriority = (lookupDtcFlags(triggeringDtc) & DTC_FLAG_MISFIRE_FUEL) != 0;

  // A low-priority DTC never overwrites a freeze frame that is already stored.
  if (!highPriority && ecu.freezeFrame.valid)
    return;

  for (uint8_t i = 0; i < FREEZE_FRAME_PID_COUNT; i++)
  {
    const uint8_t pid = FREEZE_FRAME_PIDS[i];
    const uint8_t len = pgm_read_byte(PIDByteLengthMap + pid);
    const unsigned int addr = pgm_read_word(PIDAddressMap + pid);
    const int snapshotOffset = freezeFramePidOffset(pid);

    for (uint8_t b = 0; b < len; b++)
      ecu.freezeFrame.snapshot[snapshotOffset + b] = PID_Value_Map[addr + b];
  }

  ecu.freezeFrame.valid = true;
  ecu.freezeFrame.originDtc = triggeringDtc;
}
