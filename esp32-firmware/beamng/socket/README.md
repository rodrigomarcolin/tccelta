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

Save the Python script as `obd_receiver.py`.

**Print-only mode** — receive telemetry and display the calculated values without connecting to the Arduino:

```bash
python obd_receiver.py
```

**Serial mode** — receive telemetry and send PID updates to an Arduino on COM8:

```bash
python obd_receiver.py --port COM8
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
python obd_receiver.py --port COM8 --baud 115200 --hz 20 --gap-ms 3
```

The bridge sends changed PID values at the configured refresh rate and resends all supported values every two seconds to help recover from missed updates.

## Supported OBD-II PIDs

| PID | Parameter | Source |
|---|---|---|
| `04` | Calculated engine load | BeamNG telemetry |
| `05` | Engine coolant temperature | BeamNG telemetry |
| `0C` | Engine RPM | BeamNG telemetry |
| `0D` | Vehicle speed | BeamNG telemetry |
| `10` | Mass air flow (MAF) | Estimated |
| `2F` | Fuel tank level input | BeamNG telemetry |
| `49` | Accelerator pedal position D | BeamNG telemetry |

MAP and MAF are estimated because BeamNG does not directly provide all the required measurements through this protocol. The estimation parameters can be adjusted in the Python script.

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
- MAP and MAF readings are estimates, not direct measurements.
- Serial commands must match the format expected by the installed ECUSim firmware.

