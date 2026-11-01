# TCCelta

A custom UDP protocol for BeamNG.drive that sends vehicle telemetry to an OBD-II ECU simulator.

The protocol is based on BeamNG's built-in `outgauge.lua` and sends vehicle data over UDP to `127.0.0.1:4446` at up to 50 updates per second.

This folder holds two independent protocol files:

| File | Magic | Destination | Rate | Consumer |
|---|---|---|---|---|
| `tccelta.lua` | `OBD1` | `127.0.0.1:4446` | 50 Hz | `../socket/tccelta_bridge.py` (engine telemetry → ECUSim) |
| `tccelta_gps.lua` | `GPS1` | `127.0.0.1:4447` | 20 Hz | `../gps/tccelta_gps_bridge.py` (vehicle position → BLE → app) |

`tccelta_gps.lua` sends a 28-byte packet (`<4s6f`): `format` (`GPS1`), `posX`, `posY`, `posZ` (world position, m) and `velX`, `velY`, `velZ` (world velocity, m/s), taken from `obj:getPositionXYZ()` / `obj:getVelocityXYZ()`. Install it the same way as `tccelta.lua`, in the same `protocols` folder; both are loaded as "others" protocols, so the game's "others" protocols option (setting `protocols_others_enabled`) must be enabled, as it already is for `tccelta.lua`. See `../gps/README.md` for the bridge and the coordinate conversion.

## Installation

Choose one of the following methods.

### Method 1: Install as a mod

1. Create the following folder structure in your BeamNG user folder:

   ```text
   mods/unpacked/tccelta/lua/vehicle/protocols/
   ```

2. Place `tccelta.lua` inside the `protocols` folder.

   The final path should be:

   ```text
   mods/unpacked/tccelta/lua/vehicle/protocols/tccelta.lua
   ```

### Method 2: Install directly into the game files

1. Open Steam and go to your Library.
2. Right-click **BeamNG.drive** and select **Manage → Browse local files**.
3. Navigate to:

   ```text
   lua/vehicle/protocols/
   ```

4. Place `tccelta.lua` in that folder.

## Usage

1. Start BeamNG.drive.
2. Load a vehicle.
3. Reload the vehicle using **Ctrl + R** if it was already loaded.
4. Make sure your OBD-II ECU simulator is listening for UDP packets on port `4446`.

The protocol sends data to `127.0.0.1` (localhost), so the simulator must run on the same computer unless you modify the destination address.

## Telemetry Data

Each UDP packet (64 bytes, little-endian `<4s15f`) contains the following fields. Only raw vehicle signals are sent; the bridge derives every Service 01 PID available in the dongle's diagnostic mode from them (see `../socket/README.md`).

| Field | Description | Unit | Feeds PIDs |
|---|---|---|---|
| `format` | Protocol identifier (`OBD2`) | — | — |
| `rpm` | Engine speed | RPM | `0C`, MAF/MAP/timing estimates |
| `engineLoad` | Calculated engine load | 0–1 | `04`, `0B`, `43`, `44` |
| `watertemp` | Coolant temperature | °C | `05`, `30`, `2E` |
| `oiltemp` | Engine oil temperature | °C | `5C` |
| `fuel` | Fuel level | 0–1 | `2F` |
| `throttle` | Throttle body position (after ESC / limiter) | 0–1 | `11`, `45`, `47`, `4C` |
| `throttleInput` | Driver pedal | 0–1 | `49`, `5A`, `61` |
| `speed` | Vehicle speed | m/s | `0D`, `31`, `21` |
| `turboBoostPsi` | Turbo boost pressure | PSI | `0B`, `0F` |
| `engineRunning` | Engine running flag | 0/1 | `1F` and all modelled PIDs |
| `checkEngine` | BeamNG's check engine light | 0/1 | `21`, `4D` |
| `engineTorque` | Current engine output torque | N·m | `62` |
| `maxTorque` | Peak torque of the engine | N·m | `62`, `63` |
| `ambientTemp` | Ambient air temperature | °C | `46`, `0F` |
| `ambientPressure` | Ambient pressure | kPa | `33`, `0B` |

The data is sent as a binary structure. The receiving application must use the same field order, data types, and memory layout to decode packets correctly.

## Configuration

The default settings are defined in `tccelta.lua`:

- **IP address:** `127.0.0.1`
- **UDP port:** `4446`
- **Maximum update rate:** `50 Hz`

You can modify these values in the Lua file if needed.

## Notes

- The protocol skips sending data until the vehicle's coolant temperature signal is available.
- Some telemetry values may be zero if the vehicle does not provide the corresponding signal (e.g. `engineTorque`/`maxTorque` on vehicles without a combustion engine). Ambient temperature and pressure fall back to 25 °C / 101.325 kPa if the environment cannot be queried.
- Installing directly into the game directory may require reinstalling the file after a game update or file verification.
