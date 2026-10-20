# TCCelta

A custom UDP protocol for BeamNG.drive that sends vehicle telemetry to an OBD-II ECU simulator.

The protocol is based on BeamNG's built-in `outgauge.lua` and sends vehicle data over UDP to `127.0.0.1:4446` at up to 50 updates per second.

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

Each UDP packet contains the following fields:

| Field | Description | OBD-II PID / Unit |
|---|---|---|
| `format` | Protocol identifier (`OBD1`) | — |
| `rpm` | Engine speed | PID `0C`, RPM |
| `engineLoad` | Calculated engine load | PID `04`, 0–1 |
| `watertemp` | Coolant temperature | PID `05`, °C |
| `fuel` | Fuel level | PID `2F`, 0–1 |
| `throttle` | Throttle position | PID `49`, 0–1 |
| `speed` | Vehicle speed | PID `0D`, m/s |
| `turboBoostPsi` | Turbo boost pressure | PSI |
| `oiltemp` | Engine oil temperature | °C |

The data is sent as a binary structure. The receiving application must use the same field order, data types, and memory layout to decode packets correctly.

## Configuration

The default settings are defined in `tccelta.lua`:

- **IP address:** `127.0.0.1`
- **UDP port:** `4446`
- **Maximum update rate:** `50 Hz`

You can modify these values in the Lua file if needed.

## Notes

- The protocol skips sending data until the vehicle's coolant temperature signal is available.
- Some telemetry values may be zero if the vehicle does not provide the corresponding signal.
- Installing directly into the game directory may require reinstalling the file after a game update or file verification.
