-- tccelta.lua : custom BeamNG UDP protocol for an OBD-II ECU simulator
-- Structure copied from the shipped lua/vehicle/protocols/outgauge.lua
-- Install at: USER_FOLDER/mods/unpacked/tccelta/lua/vehicle/protocols/tccelta.lua
-- Then reload the vehicle with Ctrl+R.

local M = {}

local function init() end
local function reset() end
local function getAddress()        return "127.0.0.1" end
local function getPort()           return 4446 end
local function getMaxUpdateRate()  return 50 end

local function isPhysicsStepUsed()
  return false -- graphics step, same as outgauge
end

local function getStructDefinition()
  return [[
    char  format[4];      // fixed value "OBD1", lets the receiver reject other protocols' packets
    float rpm;            // RPM                               -> PID 0C
    float engineLoad;     // 0 to 1                            -> PID 04
    float watertemp;      // C                                 -> PID 05
    float fuel;           // 0 to 1                            -> PID 2F
    float throttle;       // 0 to 1                            -> PID 49
    float speed;          // M/S                               -> PID 0D
    float turboBoostPsi;  // PSI, 0 if no forced induction     -> input for MAP (0B) / MAF (10) estimate
    float oiltemp;        // C
  ]]
end

local function fillStruct(o, dtSim)
  local ev = electrics.values
  if not ev.watertemp then
    -- vehicle not completely initialized, skip sending package (same guard as outgauge.lua)
    return
  end

  o.format = "OBD1"
  o.rpm = ev.rpm or 0
  o.engineLoad = ev.engineLoad or 0
  o.watertemp = ev.watertemp or 0
  o.fuel = ev.fuel or 0
  o.throttle = ev.throttle or 0
  o.speed = ev.wheelspeed or ev.airspeed or 0
  o.turboBoostPsi = ev.turboBoost or 0
  o.oiltemp = ev.oiltemp or 0
end

M.init = init
M.reset = reset
M.getAddress = getAddress
M.getPort = getPort
M.getMaxUpdateRate = getMaxUpdateRate
M.getStructDefinition = getStructDefinition
M.fillStruct = fillStruct
M.isPhysicsStepUsed = isPhysicsStepUsed

return M
