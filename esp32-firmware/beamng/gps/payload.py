"""BLE GPS payload v1 (PC -> app), 16 bytes little-endian "<BBHiii":
version u8 (=1), flags u8 (bit0 = valid fix), seq u16, latE7 i32, lonE7 i32, altMm i32.
Golden vector: encode(1, -235603492, -467313340, 728160, True).hex()
               == "01010100dcf9f4f1445d25e4601c0b00"
"""
import struct

VERSION = 1
FLAG_VALID = 0x01
FMT = "<BBHiii"
SIZE = struct.calcsize(FMT)  # 16


def encode(seq, lat_e7, lon_e7, alt_mm, valid):
    flags = FLAG_VALID if valid else 0
    return struct.pack(FMT, VERSION, flags, seq & 0xFFFF, lat_e7, lon_e7, alt_mm)


def decode(data):
    """bytes -> (seq, valid, lat_e7, lon_e7, alt_mm); ValueError on wrong size/version."""
    if len(data) != SIZE:
        raise ValueError(f"payload must be {SIZE} bytes, got {len(data)}")
    version, flags, seq, lat_e7, lon_e7, alt_mm = struct.unpack(FMT, data)
    if version != VERSION:
        raise ValueError(f"unsupported payload version {version}")
    return seq, bool(flags & FLAG_VALID), lat_e7, lon_e7, alt_mm
