# TCCelta — BeamNG Simulated GPS over BLE

A Python bridge that takes the vehicle's position in BeamNG.drive, converts it to real latitude/longitude and publishes it over Bluetooth Low Energy, with the PC acting as a BLE peripheral. The TCCelta app (debug builds only, **Configurações → Posição (GPS) → Fonte: Simulador (BeamNG)**) reads it as its GPS source and forwards it to the dongle with `AT#GPS`, exactly like a real phone fix.

## How It Works

```text
BeamNG.drive (map cid_exp3)
     │
     │ UDP "GPS1" world position, 20 Hz, 127.0.0.1:4447   (tccelta_gps.lua)
     ▼
Python Bridge (tccelta_gps_bridge.py)
     │  world x,y,z → SIRGAS 2000 / UTM 23S → lat, lon, alt   (geo.py)
     │
     │ BLE notify, 16-byte payload, 2 Hz   (payload.py)
     ▼
TCCelta app (GpsReporter)
     │
     │ AT#GPS <latE7> <lonE7> <altMm>
     ▼
OBD-II Dongle
```

The bridge keeps only the newest UDP packet. If no packet arrives for more than 1 s (game paused, vehicle reloading, BeamNG closed), it **stops notifying**, so the app never re-sends an old position.

## Files

| File | Purpose |
|---|---|
| `../protocol/tccelta_gps.lua` | BeamNG UDP protocol: `GPS1` packet with world position and velocity |
| `geo.py` | World → UTM → lat/lon (Krüger series, GRS80, no `pyproj`), rounding helpers |
| `payload.py` | BLE payload v1 encoder/decoder |
| `tccelta_gps_bridge.py` | The bridge (asyncio + WinRT GATT server) |
| `test_geo.py` | Unit tests (stdlib `unittest`) |

## Installation

### 1. Install the BeamNG Lua protocol

Put `tccelta_gps.lua` in the same folder as `tccelta.lua` (see `../protocol/README.md`):

```text
USER_FOLDER/mods/unpacked/tccelta/lua/vehicle/protocols/tccelta_gps.lua
```

or, in the game files (**Steam → Manage → Browse local files**), in `lua/vehicle/protocols/`.

BeamNG loads every protocol other than `outgauge` and `motionSim` as an "others" protocol, so the "others" protocols option (setting `protocols_others_enabled`) must be enabled — it already is if `tccelta.lua` works. Reload the vehicle with `Ctrl + R`.

The packet is 28 bytes, little-endian `<4s6f`:

| Field | Description | Unit |
|---|---|---|
| `format` | Protocol identifier (`GPS1`) | — |
| `posX`, `posY`, `posZ` | World position (`obj:getPositionXYZ()`); +X east, +Y north, Z elevation | m |
| `velX`, `velY`, `velZ` | World velocity (`obj:getVelocityXYZ()`) | m/s |

While the vehicle object is not available the Lua side leaves the packet zeroed (no `GPS1` magic) and the bridge drops it.

### 2. Install Python dependencies

BLE needs Windows 10+ and a Bluetooth adapter that supports the **peripheral role** (the bridge checks `BluetoothAdapter.IsPeripheralRoleSupported` and says so if not). It uses the Python WinRT projection (winrt 3.x) directly:

```bash
py -m pip install winrt-runtime winrt-Windows.Foundation winrt-Windows.Devices.Bluetooth winrt-Windows.Devices.Bluetooth.GenericAttributeProfile winrt-Windows.Storage.Streams
```

These are only needed for BLE and are imported lazily. With `--no-ble` the bridge (and the tests) use the standard library only.

## Usage

Run from this folder:

```bash
py tccelta_gps_bridge.py                 # BeamNG -> BLE, plus console output
py tccelta_gps_bridge.py --no-ble        # BeamNG -> console only
py tccelta_gps_bridge.py --fake          # no BeamNG: 100 m circle around the map centre -> BLE
py tccelta_gps_bridge.py --fake --no-ble # no BeamNG, no BLE: just see the numbers
```

Once per second the bridge prints the current fix:

```text
-23.5603495,-46.7313340  alt= 728.16m  speed=  36.0km/h  seq=7
```

The leading `lat,lon` can be pasted straight into Google Maps to check where the car is.

## Command-Line Options

| Option | Default | Description |
|---|---|---|
| `--hz` | `2` | BLE notify rate |
| `--stale-s` | `1.0` | Stop notifying when no UDP packet arrived for this many seconds |
| `--no-ble` | off | Console only; the `winrt` packages are not imported |
| `--fake` | off | Ignore BeamNG; 100 m radius circle around `-23.56057,-46.72906`, one lap per minute (~37.7 km/h), altitude 750 m |

## BLE Contract

The app depends on these values; change both sides together.

- Advertised name: `TCCelta-SIM`; the service UUID is included in the advertisement.
- Service `8f1d0001-7c2b-4e5a-9b3c-5a4e2d1c0b01`
  - RX `8f1d0002-7c2b-4e5a-9b3c-5a4e2d1c0b01`: write / write without response, ignored (exists only so the app's `BleService.connect(service, rx, tx)` works unchanged)
  - TX `8f1d0003-7c2b-4e5a-9b3c-5a4e2d1c0b01`: notify + read

Payload v1, 16 bytes little-endian `<BBHiii`:

| Offset | Type | Field | Notes |
|---|---|---|---|
| 0 | `u8` | `version` | `1` |
| 1 | `u8` | `flags` | bit 0 = valid fix |
| 2 | `u16` | `seq` | +1 per notification, wraps at 65536 |
| 4 | `i32` | `latE7` | latitude × 1e7, rounded half away from zero (like Dart's `round()`) |
| 8 | `i32` | `lonE7` | longitude × 1e7 |
| 12 | `i32` | `altMm` | altitude in mm |

Golden vector (tested in `test_geo.py` and in the app): lat −23.5603492, lon −46.7313340, alt 728.160 m, seq 1, valid →

```text
01010100dcf9f4f1445d25e4601c0b00
```

Notifications always carry `flags = 1`. When the source goes stale, notifications stop and a read of TX returns the last position with `flags = 0`.

On Windows the advertised local name may be the PC's Bluetooth name instead of `TCCelta-SIM` (the WinRT GATT server advertises the service UUID but not a custom name); scanners should match on the service UUID.

## Coordinate Transform

The map `cid_exp3` (a cut of Cidade Universitária, São Paulo) was generated with the BeamNG Mapping Tools from data in **SIRGAS 2000 / UTM zone 23S (EPSG:31983)** — GRS80 ellipsoid, central meridian −45°, k0 0.9996, false easting 500000 m, false northing 10000000 m. From `levels/cid_exp3/MT_settings.json` and the terrain block:

- native bbox: X 322470.511–324580.511, Y 7392303.171–7394413.171 (2110 m);
- terrain: 2048 px × 1.03 m = 2109.44 m, `TerrainBlock.position = (-1054.72, -1054.72, 715.58)`.

World (x, y, z) → lat/lon/alt:

```text
s   = 2109.44 / 2110                            # world metres per UTM metre
E   = 322470.511 + (x + 1054.72) / s            # +X = east
N   = 7392303.171 + (y + 1054.72 + 1.0) / s     # +Y = north; +1.0 m = measured one-pixel-row bias
lat, lon = UTM23S_inverse(E, N)                 # Krüger series, 3 terms
alt = z                                         # world z is already the real elevation
```

The constants live in `geo.MAPS["cid_exp3"]`. The transform was validated against six OSM nodes with known IDs and against the decal-road nodes (0.00–0.10 m, axes not swapped); `test_geo.py` re-checks the six OSM nodes with a 1.0 m tolerance. A linear interpolation over the lat/lon bbox does **not** work: meridian convergence (~0.7°) causes up to ~13 m of error at the edges. SIRGAS 2000 and WGS84 coincided at SIRGAS's epoch (2000.4) and today differ only by a few decimetres of plate motion, below phone GPS accuracy, so the result is used as GPS lat/lon directly.

## Tests

From the repository root:

```bash
py -m unittest discover -s esp32-firmware/beamng/gps
```

They cover the UTM forward/inverse round trip (< 1 mm), the six OSM nodes (< 1.0 m), Dart-compatible rounding and the payload golden vector.

## Gate G1 — Does the PC work as a BLE peripheral?

Not every Bluetooth adapter/driver accepts the peripheral (GATT server + advertising) role on Windows. Check it before relying on the app integration:

1. On the PC: `py tccelta_gps_bridge.py --fake`. It must print `GATT advertisement status: STARTED`, then `BLE advertising ...` and one `lat,lon` line per second.
2. On the phone, open **nRF Connect** → **Scanner** and scan.
3. The **PC's Bluetooth name** appears (the WinRT GATT server cannot set a custom local name, so `TCCelta-SIM` is not advertised on Windows; the app finds it by service UUID), advertising service `8f1d0001-…-5a4e2d1c0b01`.
4. **Connect**. The service shows RX (`…0002…`, write) and TX (`…0003…`, notify, read).
5. Enable notifications on TX (the triple-arrow icon). Values arrive about twice a second; byte 0 is `01`, byte 1 is `01`, bytes 2–3 (`seq`) increase and the lat/lon bytes change slowly as the fake car drives around the circle.
6. Stop the bridge (`Ctrl + C`); the phone shows the disconnection.

If the bridge fails to start the peripheral, or the device never shows up in the scan, the adapter probably does not support the peripheral role: try a USB Bluetooth dongle (BT ≥ 4.0) before integrating the app.

## Notes

- The UDP destination is `127.0.0.1:4447` (`tccelta.lua`'s engine telemetry stays on `4446`); both bridges can run at the same time.
- Only `cid_exp3` is georeferenced. On other maps the printed coordinates are meaningless.
- `--fake` does not bind the UDP port, so it can run while BeamNG is open without mixing sources.
