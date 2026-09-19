#ifndef ECUSIM_H
#define ECUSIM_H

#include "mcp_can.h"
#include "mcp_can_dfs.h"
#include <SPI.h>
#include <avr/pgmspace.h>
#include "PIDMap_Definition.h"
#include "ECUStateModel.h"
#include <iso-tp.h>

// Simulated ECUs' CAN IDs
constexpr unsigned long ECM_CAN_ID = 0x7E0;
constexpr unsigned long ECM_CAN_RESPONSE_ID = ECM_CAN_ID + 0x008;
constexpr unsigned long TCM_CAN_ID = 0x7E1;
constexpr unsigned long TCM_CAN_RESPONSE_ID = TCM_CAN_ID + 0x008;

// OBD-II functional broadcast request ID (every ECU listens to this one)
constexpr unsigned long OBD_BROADCAST_CAN_ID = 0x7DF;

// ECU response(reply wait in ms)
constexpr int ECU_WAIT = 0; 

// Debug message serial out switch
constexpr bool PIDSET_DEBUG = false;
constexpr bool PIDSET_ERROR = true;
constexpr bool CANMSG_DEBUG = false;
constexpr bool CANMSG_TIME_MEAS = false;
constexpr bool CANMSG_FREERAM_MEAS = false;
constexpr bool CANMSG_ERROR = true;
constexpr bool CANMSG_FATAL = true;
constexpr bool DTC_DEBUG = false;
constexpr bool DTC_ERROR = true;

extern byte PID_Value_Map[];
extern MCP_CAN CAN;

constexpr int SERIAL_MSG_LENGTH = 11;
constexpr int DTC_SERIAL_MSG_LENGTH = 9;
constexpr int CAN_PAYLOAD_LENGTH = 8;

#endif