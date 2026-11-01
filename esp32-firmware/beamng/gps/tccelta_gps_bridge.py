#!/usr/bin/env python3
"""BeamNG -> BLE simulated GPS bridge.

Receives tccelta_gps.lua UDP packets from BeamNG, converts the vehicle's world
position to lat/lon (geo.py, map cid_exp3) and publishes it as a BLE GATT
peripheral ("TCCelta-SIM") that the app reads as its simulated GPS source.

UDP packet in : "<4s6f" = b"GPS1", posX, posY, posZ (m), velX, velY, velZ (m/s)   (28 bytes)
BLE out       : TX notify, 16-byte payload v1 "<BBHiii" (see payload.py), at --hz.
                No UDP packet for more than --stale-s -> notifications stop, so the
                app never re-sends an old position.

Usage:
  py tccelta_gps_bridge.py                 # BeamNG -> BLE (+ console)
  py tccelta_gps_bridge.py --no-ble        # BeamNG -> console only
  py tccelta_gps_bridge.py --fake          # no BeamNG: 100 m circle around the map centre -> BLE
  py tccelta_gps_bridge.py --fake --no-ble
BLE needs:  Windows 10+, an adapter with the BLE peripheral role and the winrt packages
            (see README.md); none of it is needed with --no-ble
"""
import argparse
import asyncio
import math
import struct
import sys
import time

import geo
import payload

HOST, PORT = "127.0.0.1", 4447
FMT = "<4s6f"
SIZE = struct.calcsize(FMT)  # 28
MAGIC = b"GPS1"
MAP_NAME = "cid_exp3"

# --- BLE contract (the app's SimulatedGpsDatasource depends on these) ---
BLE_NAME = "TCCelta-SIM"
SERVICE_UUID = "8f1d0001-7c2b-4e5a-9b3c-5a4e2d1c0b01"
RX_UUID = "8f1d0002-7c2b-4e5a-9b3c-5a4e2d1c0b01"  # write, ignored
TX_UUID = "8f1d0003-7c2b-4e5a-9b3c-5a4e2d1c0b01"  # notify + read, payload.py v1

# --- --fake: one lap per minute on a 100 m circle around the centre of cid_exp3 ---
FAKE_CENTER = (-23.56057, -46.72906)
FAKE_RADIUS_M = 100.0
FAKE_LAP_S = 60.0
FAKE_ALT_M = 750.0
FAKE_HZ = 20.0  # same rate as tccelta_gps.lua


class Fix:
    __slots__ = ("lat", "lon", "alt_m", "speed_ms", "t")

    def __init__(self, lat, lon, alt_m, speed_ms, t):
        self.lat, self.lon, self.alt_m, self.speed_ms, self.t = lat, lon, alt_m, speed_ms, t


class State:
    def __init__(self):
        self.latest = None  # newest Fix, overwritten by every valid packet
        self.seq = 0        # last seq sent (u16)
        self.notified = 0
        self.bad_lengths = set()

    def fresh_fix(self, stale_s):
        fix = self.latest
        if fix is not None and time.monotonic() - fix.t <= stale_s:
            return fix
        return None


class GpsUdpProtocol(asyncio.DatagramProtocol):
    """Keeps only the newest valid GPS1 packet, already converted to lat/lon."""

    def __init__(self, state):
        self.state = state

    def datagram_received(self, data, addr):
        if len(data) != SIZE:
            if len(data) not in self.state.bad_lengths:
                self.state.bad_lengths.add(len(data))
                print(f"bad length {len(data)} (expected {SIZE}) - struct packing differs?")
            return
        fmt, px, py, pz, vx, vy, vz = struct.unpack(FMT, data)
        if fmt != MAGIC:  # zeroed packet: the Lua guard skipped filling it
            return
        if not all(math.isfinite(v) for v in (px, py, pz, vx, vy, vz)):
            return
        lat, lon, alt = geo.world_to_wgs84(px, py, pz, MAP_NAME)
        speed = math.sqrt(vx * vx + vy * vy + vz * vz)
        self.state.latest = Fix(lat, lon, alt, speed, time.monotonic())


async def fake_source(state):
    e0, n0 = geo.utm_forward(*FAKE_CENTER)
    zone = geo.MAPS[MAP_NAME]["zone"]
    south = geo.MAPS[MAP_NAME]["south"]
    speed = 2 * math.pi * FAKE_RADIUS_M / FAKE_LAP_S
    t0 = time.monotonic()
    while True:
        now = time.monotonic()
        ang = 2 * math.pi * (now - t0) / FAKE_LAP_S
        lat, lon = geo.utm_inverse(e0 + FAKE_RADIUS_M * math.cos(ang),
                                   n0 + FAKE_RADIUS_M * math.sin(ang), zone, south)
        state.latest = Fix(lat, lon, FAKE_ALT_M, speed, now)
        await asyncio.sleep(1.0 / FAKE_HZ)


class BlePeripheral:
    """GATT server (Windows WinRT): service SERVICE_UUID with RX (write, ignored) and TX (notify + read).

    WinRT event handlers fire on thread-pool threads, so they only touch plain attributes
    or hand work to the event loop with run_coroutine_threadsafe.
    """

    def __init__(self):
        self.provider = None
        self.tx = None
        self.rx = None
        self.loop = None
        self.value = bytes(payload.encode(0, 0, 0, 0, False))  # read before any fix: invalid
        self._tokens = []  # (object, remove_method_name, token)
        self._notify_task = None
        self._advertising = False
        self._adv_error = None

    async def start(self):
        # lazy: --no-ble and the tests run without the winrt packages installed
        from uuid import UUID

        from winrt.windows.devices.bluetooth import BluetoothAdapter, BluetoothError
        from winrt.windows.devices.bluetooth.genericattributeprofile import (
            GattCharacteristicProperties,
            GattLocalCharacteristicParameters,
            GattProtectionLevel,
            GattServiceProvider,
            GattServiceProviderAdvertisingParameters,
        )

        self.loop = asyncio.get_running_loop()

        adapter = await BluetoothAdapter.get_default_async()
        if adapter is None:
            raise RuntimeError("no Bluetooth adapter found (is Bluetooth turned on?)")
        if not adapter.is_peripheral_role_supported:
            raise RuntimeError("this Bluetooth adapter does not support the BLE peripheral role "
                               "(try a USB Bluetooth >= 4.0 dongle)")

        result = await GattServiceProvider.create_async(UUID(SERVICE_UUID))
        if result.error != BluetoothError.SUCCESS:
            raise RuntimeError(f"GattServiceProvider.create_async failed: {result.error.name}")
        self.provider = result.service_provider

        # TX: read + notify. Reads are answered by _on_read with the newest value.
        tx_params = GattLocalCharacteristicParameters()
        tx_params.characteristic_properties = (
            GattCharacteristicProperties.READ | GattCharacteristicProperties.NOTIFY)
        tx_params.read_protection_level = GattProtectionLevel.PLAIN
        tx_params.user_description = "TCCelta simulated GPS"
        res = await self.provider.service.create_characteristic_async(UUID(TX_UUID), tx_params)
        if res.error != BluetoothError.SUCCESS:
            raise RuntimeError(f"creating TX characteristic failed: {res.error.name}")
        self.tx = res.characteristic
        self._subscribe(self.tx, "read_requested", self._on_read)

        # RX: write / write without response, ignored.
        rx_params = GattLocalCharacteristicParameters()
        rx_params.characteristic_properties = (
            GattCharacteristicProperties.WRITE | GattCharacteristicProperties.WRITE_WITHOUT_RESPONSE)
        rx_params.write_protection_level = GattProtectionLevel.PLAIN
        res = await self.provider.service.create_characteristic_async(UUID(RX_UUID), rx_params)
        if res.error != BluetoothError.SUCCESS:
            raise RuntimeError(f"creating RX characteristic failed: {res.error.name}")
        self.rx = res.characteristic
        self._subscribe(self.rx, "write_requested", self._on_write)

        self._subscribe(self.provider, "advertisement_status_changed", self._on_adv_status)

        adv = GattServiceProviderAdvertisingParameters()
        adv.is_connectable = True
        adv.is_discoverable = True
        self.provider.start_advertising_with_parameters(adv)

        # Windows reports a transient ABORTED right after the call; wait for the real outcome.
        status = None
        for _ in range(40):  # up to 4 s
            status = self.provider.advertisement_status.name
            if status.startswith("STARTED"):
                break
            await asyncio.sleep(0.1)
        print(f"GATT advertisement status: {status}")
        if not status.startswith("STARTED"):
            raise RuntimeError(f"advertising did not start (status {status}, "
                               f"last event error {self._adv_error})")
        if status == "STARTED_WITHOUT_ALL_ADVERTISEMENT_DATA":
            print("warning: advertising started without all advertisement data "
                  "(match on the service UUID, not on the name)")
        self._advertising = True

    def _subscribe(self, obj, event, handler):
        token = getattr(obj, "add_" + event)(handler)
        self._tokens.append((obj, "remove_" + event, token))

    # --- WinRT callbacks (thread-pool threads): keep them minimal ---

    def _on_adv_status(self, sender, args):
        self._adv_error = args.error.name
        if self._advertising:  # before start() finishes, the transient ABORTED is just noise
            print(f"GATT advertisement status changed: {args.status.name} (error {args.error.name})")

    def _on_read(self, sender, args):
        deferral = args.get_deferral()
        loop = self.loop
        if loop is None or loop.is_closed():
            deferral.complete()
            return
        asyncio.run_coroutine_threadsafe(self._respond_read(args, deferral), loop)

    async def _respond_read(self, args, deferral):
        try:
            request = await args.get_request_async()
            if request is not None:
                request.respond_with_value(self._buffer(self.value))
        except Exception as e:
            print(f"read request failed: {type(e).__name__}: {e}")
        finally:
            deferral.complete()

    def _on_write(self, sender, args):
        deferral = args.get_deferral()
        loop = self.loop
        if loop is None or loop.is_closed():
            deferral.complete()
            return
        asyncio.run_coroutine_threadsafe(self._respond_write(args, deferral), loop)

    async def _respond_write(self, args, deferral):
        try:
            from winrt.windows.devices.bluetooth.genericattributeprofile import GattWriteOption

            request = await args.get_request_async()
            if request is not None and request.option == GattWriteOption.WRITE_WITH_RESPONSE:
                request.respond()  # RX is ignored; just acknowledge
        except Exception as e:
            print(f"write request failed: {type(e).__name__}: {e}")
        finally:
            deferral.complete()

    @staticmethod
    def _buffer(data):
        from winrt.windows.storage.streams import DataWriter

        writer = DataWriter()
        writer.write_bytes(bytes(data))
        return writer.detach_buffer()

    def publish(self, data, notify):
        self.value = bytes(data)
        if not notify or self.tx is None or len(self.tx.subscribed_clients) == 0:
            return
        if self._notify_task is not None and not self._notify_task.done():
            return  # previous notification still in flight: the next tick carries newer data
        self._notify_task = asyncio.ensure_future(self._notify(self.value))

    async def _notify(self, value):
        try:
            await self.tx.notify_value_async(self._buffer(value))
        except Exception as e:
            print(f"notify failed: {type(e).__name__}: {e}")

    async def stop(self):
        if self._notify_task is not None:
            self._notify_task.cancel()
            self._notify_task = None
        for obj, remove, token in self._tokens:
            try:
                getattr(obj, remove)(token)
            except Exception:
                pass
        self._tokens = []
        self._advertising = False
        if self.provider is not None:
            try:
                self.provider.stop_advertising()
            except Exception:
                pass
        self.provider = self.tx = self.rx = None


def build_payload(state, fix, valid):
    return payload.encode(state.seq, geo.to_e7(fix.lat), geo.to_e7(fix.lon), geo.to_mm(fix.alt_m), valid)


async def notifier(state, ble, hz, stale_s):
    period = 1.0 / hz
    was_fresh = False
    next_tick = time.monotonic()
    while True:
        fix = state.fresh_fix(stale_s)
        if fix is not None:
            state.seq = (state.seq + 1) & 0xFFFF
            if ble:
                ble.publish(build_payload(state, fix, True), notify=True)
            state.notified += 1
        elif was_fresh and ble and state.latest is not None:
            # just went stale: a read now returns the last position marked invalid; no notify
            ble.publish(build_payload(state, state.latest, False), notify=False)
        was_fresh = fix is not None
        next_tick += period
        await asyncio.sleep(max(0.0, next_tick - time.monotonic()))


async def printer(state, stale_s, source):
    while True:
        await asyncio.sleep(1.0)
        fix = state.fresh_fix(stale_s)
        if fix is not None:
            # "lat,lon" first: paste it straight into Google Maps
            print(f"{fix.lat:.7f},{fix.lon:.7f}  alt={fix.alt_m:7.2f}m  "
                  f"speed={fix.speed_ms * 3.6:6.1f}km/h  seq={state.seq}")
        elif state.latest is None:
            print(f"waiting for {source}...")
        else:
            age = time.monotonic() - state.latest.t
            print(f"no packet for {age:4.1f}s (> {stale_s:g}s) - not notifying")


async def run(args):
    state = State()
    transport = None
    ble = None
    tasks = []
    try:
        if args.fake:
            source = "fake source"
            tasks.append(asyncio.create_task(fake_source(state)))
            print(f"fake source: {FAKE_RADIUS_M:g} m circle around {FAKE_CENTER[0]},{FAKE_CENTER[1]}, "
                  f"1 lap / {FAKE_LAP_S:g} s, alt {FAKE_ALT_M:g} m")
        else:
            source = f"{SIZE}-byte GPS1 packets on {HOST}:{PORT}"
            loop = asyncio.get_running_loop()
            try:
                transport, _ = await loop.create_datagram_endpoint(
                    lambda: GpsUdpProtocol(state), local_addr=(HOST, PORT))
            except OSError as e:
                print(f"cannot bind UDP {HOST}:{PORT}: {e}")
                return 1
            print(f"listening on {HOST}:{PORT}, expecting {SIZE}-byte GPS1 packets (map {MAP_NAME})")

        if not args.no_ble:
            ble = BlePeripheral()
            try:
                await ble.start()
            except ImportError as e:
                print(f"winrt packages are not available ({e}): see README.md (or run with --no-ble)")
                return 1
            except Exception as e:  # adapter missing / no peripheral role support
                print(f"BLE peripheral failed to start: {type(e).__name__}: {e}")
                return 1
            print(f"BLE advertising service {SERVICE_UUID} (Windows advertises the PC's Bluetooth name, not '{BLE_NAME}')")
            print(f"  TX {TX_UUID} (notify+read) at {args.hz:g} Hz, RX {RX_UUID} (write, ignored)")
        else:
            print("--no-ble: console only")

        tasks.append(asyncio.create_task(notifier(state, ble, args.hz, args.stale_s)))
        tasks.append(asyncio.create_task(printer(state, args.stale_s, source)))
        done, _ = await asyncio.wait(tasks, timeout=args.duration, return_when=asyncio.FIRST_EXCEPTION)
        for t in done:
            t.result()  # re-raise a crashed task
        return 0
    finally:
        for t in tasks:
            t.cancel()
        await asyncio.gather(*tasks, return_exceptions=True)
        if transport is not None:
            transport.close()
        if ble is not None:
            await ble.stop()


def main():
    ap = argparse.ArgumentParser(description="BeamNG (tccelta_gps.lua) -> BLE simulated GPS for the TCCelta app")
    ap.add_argument("--hz", type=float, default=2.0, help="BLE notify rate (default 2)")
    ap.add_argument("--stale-s", type=float, default=1.0,
                    help="stop notifying when no UDP packet arrived for this long (default 1.0)")
    ap.add_argument("--no-ble", action="store_true", help="console only, no BLE (winrt not needed)")
    ap.add_argument("--fake", action="store_true",
                    help="no BeamNG: drive a 100 m circle around the map centre, 1 lap/min")
    ap.add_argument("--duration", type=float, default=None, help=argparse.SUPPRESS)  # exit after S seconds
    args = ap.parse_args()
    if args.hz <= 0:
        ap.error("--hz must be > 0")

    try:
        code = asyncio.run(run(args))
    except KeyboardInterrupt:
        print("stopped")
        code = 0
    sys.exit(code)


if __name__ == "__main__":
    main()
