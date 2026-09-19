#ifndef ECUSTATEMODEL_H
#define ECUSTATEMODEL_H

#include "DTCMap_Definition.h"

constexpr uint8_t MAX_DTC_PER_LIST = 8;

constexpr int DTC_OK = 0;
constexpr int DTC_LIST_FULL = -1;
constexpr int DTC_NOT_FOUND = -2;

struct FreezeFrame_t
{
  bool     valid;
  uint16_t originDtc;
  byte     snapshot[FREEZE_FRAME_SNAPSHOT_SIZE];
};

struct EcuState_t
{
  bool          mil;
  uint16_t      confirmedDtc[MAX_DTC_PER_LIST];
  uint8_t       confirmedCount;
  uint16_t      pendingDtc[MAX_DTC_PER_LIST];
  uint8_t       pendingCount;
  uint16_t      permanentDtc[MAX_DTC_PER_LIST];
  uint8_t       permanentCount;
  FreezeFrame_t freezeFrame;
};

extern EcuState_t ecmState;
extern EcuState_t tcmState;

void initializeEcuState(EcuState_t& ecu);
int  dtcListAdd(uint16_t* list, uint8_t& count, uint8_t capacity, uint16_t dtcCode);
int  dtcListRemove(uint16_t* list, uint8_t& count, uint16_t dtcCode);
void dtcListClear(uint8_t& count);
void captureFreezeFrame(EcuState_t& ecu, uint16_t triggeringDtc);

#endif
