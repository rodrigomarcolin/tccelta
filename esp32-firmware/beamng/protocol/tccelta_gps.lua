-- tccelta_gps.lua : custom BeamNG UDP protocol with the vehicle's world position
-- Same structure as tccelta.lua (copied from the shipped lua/vehicle/protocols/outgauge.lua).
-- Position/velocity come from obj:getPositionXYZ() / obj:getVelocityXYZ(), the same calls the
-- shipped protocols.lua uses for motionSim's "world position of the vehicle".
-- Install next to tccelta.lua: USER_FOLDER/mods/unpacked/tccelta/lua/vehicle/protocols/tccelta_gps.lua
-- Needs the "others" protocols option enabled (protocols_others_enabled). Then reload the vehicle with Ctrl+R.
-- Receiver: esp32-firmware/beamng/gps/tccelta_gps_bridge.py

local M = {}

local function init() end
local function reset() end
local function getAddress()        return "127.0.0.1" end
local function getPort()           return 4447 end
local function getMaxUpdateRate()  return 20 end

local function isPhysicsStepUsed()
  return false -- graphics step, same as outgauge
end

local function getStructDefinition()
  return [[
    char  format[4];          // fixed value "GPS1", lets the receiver reject other protocols' packets
    float posX, posY, posZ;   // world position of the vehicle, m (+X east, +Y north, Z = elevation)
    float velX, velY, velZ;   // world velocity of the vehicle, m/s
  ]]
end

local function fillStruct(o, dtSim)
  if not obj then
    -- vehicle object not available yet: leave the zeroed packet (no "GPS1" magic), the receiver drops it
    return
  end

  o.format = "GPS1"
  o.posX, o.posY, o.posZ = obj:getPositionXYZ()
  o.velX, o.velY, o.velZ = obj:getVelocityXYZ()
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
