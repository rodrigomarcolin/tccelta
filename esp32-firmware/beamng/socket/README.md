# TCCelta — BeamNG to ECUSim Bridge

A Python bridge that connects BeamNG.drive to an Arduino-based ECUSim ECU simulator, allowing an OBD-II dongle to read simulated engine data through the Arduino.

The bridge receives vehicle telemetry from BeamNG over UDP, converts it into OBD-II PID values, and sends those values to the Arduino using the serial protocol expected by `PIDUpdateSerialControl.ino`.

The Arduino maintains the latest engine state and responds to OBD-II requests from the connected dongle.

## How It Works

```text
BeamNG.drive
     │
     │ UDP telemetry
     ▼
Python Bridge
     │
     │ Serial PID updates over USB
     ▼
Arduino ECUSim
     │
     │ OBD-II requests and responses
     ▼
OBD-II Dongle
     │
     ▼
Diagnostic Software
```

1. BeamNG sends vehicle telemetry using the custom Lua protocol.
2. Python decodes the UDP packets and converts the telemetry into OBD-II PID values.
3. Python sends serial commands to update the Arduino's internal engine state.
4. When the OBD-II dongle requests a supported PID, the Arduino responds using its latest state.

## Requirements

- BeamNG.drive
- The custom `tccelta.lua` Lua protocol
- Python 3
- `pyserial` for serial communication
- A compatible Arduino running the ECUSim firmware
- An OBD-II dongle and compatible diagnostic software

## Installation

### 1. Install the BeamNG Lua protocol

Install `tccelta.lua` into BeamNG's vehicle protocols directory.

You can install it as an unpacked mod:

```text
USER_FOLDER/mods/unpacked/tccelta/lua/vehicle/protocols/tccelta.lua
```

Alternatively, open BeamNG.drive in Steam, select **Manage → Browse local files**, and place the file in:

```text
lua/vehicle/protocols/
```

Reload the vehicle with `Ctrl + R` if necessary.

### 2. Install Python dependencies

Install Python 3, then install the serial communication library:

```bash
python -m pip install pyserial
```

`pyserial` is only required when sending data to the Arduino. Without `--port`, the script runs in print-only mode.

### 3. Prepare the Arduino

Upload the compatible ECUSim firmware containing `PIDUpdateSerialControl.ino` to your Arduino.

Connect the Arduino to your computer using USB. Ensure the firmware is running and has initialized the CAN bus shield successfully.

The bridge expects the firmware to accept serial messages at **115200 baud** by default.

### 4. Run the bridge

Save the Python script as `tccelta_bridge.py`.

**Print-only mode** — receive telemetry and display the calculated values without connecting to the Arduino:

```bash
python tccelta_bridge.py
```

**Serial mode** — receive telemetry and send PID updates to an Arduino on COM8:

```bash
python tccelta_bridge.py --port COM8
```

Replace `COM8` with the serial port assigned to your Arduino. On Linux, a port might look like `/dev/ttyACM0` or `/dev/ttyUSB0`.

## Command-Line Options

| Option | Default | Description |
|---|---|---|
| `--port` | None | Arduino serial port; omit for print-only mode |
| `--baud` | `115200` | Serial baud rate |
| `--hz` | `20` | Maximum ECU update frequency |
| `--gap-ms` | `3` | Delay between serial PID messages, in milliseconds |

For example:

```bash
python tccelta_bridge.py --port COM8 --baud 115200 --hz 20 --gap-ms 3
```

Each tick, the bridge sends the PIDs whose value changed or that were not sent in the last two seconds (to recover from missed updates), oldest first. A tick only sends as many messages as fit in its period at the configured `--gap-ms` (about 10 at 20 Hz); the remaining ones go out on the next tick.

## Supported OBD-II PIDs

The bridge feeds **every Service 01 data PID the dongle forwards to the bus in diagnostic mode** (the whitelist in `esp32-firmware/src/elm327/whitelist/WhitelistConfig.h`), not only those read by the collector. Service 02 freeze frames are snapshots of these same values, taken by the ECUSim when a DTC is set.

Not fed on purpose: `00`/`20`/`40`/`60` (supported-PID bitmaps, computed by the firmware) and `01`/`02` (monitor status and freeze-frame DTC, owned by the DTC control channel).

Sources: **BeamNG** = direct signal; **estimated** = physical model from BeamNG signals; **modelled** = synthetic but plausible value (BeamNG has no such signal); **accumulated** = counted by the bridge since it started, which plays the role of the last "codes cleared".

| PID | Parameter | Source |
|---|---|---|
| `04` | Calculated engine load | BeamNG |
| `05` | Engine coolant temperature | BeamNG |
| `06` | Short-term fuel trim, bank 1 | Modelled (±3 % oscillation) |
| `07` | Long-term fuel trim, bank 1 | Modelled (+1.5 %) |
| `0A` | Fuel pressure (gauge) | Modelled (300 kPa running) |
| `0B` | Intake manifold absolute pressure | Estimated (load, boost, baro) |
| `0C` | Engine RPM | BeamNG |
| `0D` | Vehicle speed | BeamNG |
| `0E` | Timing advance | Modelled (rpm, load) |
| `0F` | Intake air temperature | Estimated (ambient + boost heating) |
| `10` | Mass air flow (MAF) | Estimated (speed-density) |
| `11` | Throttle position | BeamNG |
| `14` | O2 sensor B1S1 voltage + trim | Modelled (switching 0.05–0.85 V) |
| `15` | O2 sensor B1S2 voltage | Modelled (0.65 V) |
| `1F` | Run time since engine start | Accumulated |
| `21` | Distance with MIL on | Accumulated (while BeamNG check engine is on) |
| `2C` | Commanded EGR | Modelled (10 % at part load) |
| `2D` | EGR error | Modelled (0 %) |
| `2E` | Commanded evaporative purge | Modelled (15 % when warm) |
| `2F` | Fuel tank level | BeamNG |
| `30` | Warm-ups since codes cleared | Accumulated (coolant crossing 70 °C) |
| `31` | Distance since codes cleared | Accumulated |
| `32` | Evap system vapor pressure | Modelled (50 Pa) |
| `33` | Absolute barometric pressure | BeamNG |
| `3C` | Catalyst temperature B1S1 | Modelled (first-order lag on load/rpm) |
| `42` | Control module voltage | Modelled (14.2 V running, 12.5 V off) |
| `43` | Absolute load value | Estimated (MAP / baro) |
| `44` | Commanded equivalence ratio (λ) | Modelled (1.0, 0.87 at full load) |
| `45` | Relative throttle position | BeamNG |
| `46` | Ambient air temperature | BeamNG |
| `47` | Absolute throttle position B | BeamNG |
| `49` | Accelerator pedal position D | BeamNG |
| `4C` | Commanded throttle actuator | BeamNG |
| `4D` | Time run with MIL on | Accumulated (while BeamNG check engine is on) |
| `4E` | Time since codes cleared | Accumulated |
| `52` | Ethanol fuel % | Modelled (27 %, Brazilian E27) |
| `5A` | Relative accelerator pedal position | BeamNG |
| `5C` | Engine oil temperature | BeamNG |
| `5D` | Fuel injection timing | Modelled (load) |
| `5E` | Engine fuel rate | Estimated (MAF / AFR) |
| `61` | Driver's demand engine torque | BeamNG (pedal) |
| `62` | Actual engine torque | BeamNG (torque / max torque) |
| `63` | Engine reference torque | BeamNG (max torque) |

The estimation and model parameters (displacement, volumetric efficiency, stoichiometric AFR, ethanol content, etc.) are constants at the top of the Python script and can be tuned.

## Serial Protocol

The bridge sends one ASCII message per PID using the format:

```text
PPVVVVVVVV\n
```

- `PP`: PID as two hexadecimal characters.
- `VVVVVVVV`: Four data bytes represented by eight hexadecimal characters.
- `\n`: Newline terminating the command.

For example:

```text
0C2EE00000
```

This message updates PID `0C` (engine RPM) with the four supplied data bytes in the format expected by the ECUSim firmware.

Each message is sent separately, with a configurable delay between messages to reduce the chance of overflowing the Arduino Uno's serial receive buffer.

## Troubleshooting

- **No telemetry:** Verify that BeamNG is running and `tccelta.lua` is installed correctly.
- **Serial port unavailable:** Check the COM port, close other programs using the Arduino, and verify the USB connection.
- **CAN bus initialization failure:** Check the CAN bus shield and Arduino firmware.
- **No OBD-II responses:** Verify the dongle connection, supported PIDs, and ECUSim firmware.
- **Incorrect readings:** Check the serial protocol compatibility and adjust the MAP/MAF estimation parameters if necessary.

## Notes

- The default UDP destination is `127.0.0.1:4446`.
- The Python bridge does not communicate directly with the OBD-II dongle; the Arduino handles OBD-II requests and responses.
- Estimated and modelled PIDs are not direct measurements; they exist so a diagnostic app sees a plausible value for every PID the dongle exposes.
- The UDP packet format changed from `OBD1` (36 bytes) to `OBD2` (64 bytes): update `tccelta.lua` and the bridge together.
- Serial commands must match the format expected by the installed ECUSim firmware.

