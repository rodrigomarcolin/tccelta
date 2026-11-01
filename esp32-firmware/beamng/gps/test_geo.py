"""Tests for geo.py and payload.py (stdlib only).

From the repo root:
  py -m unittest discover -s esp32-firmware/beamng/gps
  py -m unittest esp32-firmware/beamng/gps/test_geo.py
"""
import math
import os
import sys
import unittest

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))

import geo  # noqa: E402
import payload  # noqa: E402

def distance_m(lat1, lon1, lat2, lon2):
    """Local ellipsoidal (GRS80) metric; accurate to << 1 mm/m over a few km."""
    e2 = geo.FLATTENING * (2 - geo.FLATTENING)
    phi = math.radians((lat1 + lat2) / 2)
    w = 1 - e2 * math.sin(phi) ** 2
    meridian_r = geo.A_AXIS * (1 - e2) / w ** 1.5
    normal_r = geo.A_AXIS / math.sqrt(w)
    dn = math.radians(lat2 - lat1) * meridian_r
    de = math.radians(lon2 - lon1) * normal_r * math.cos(phi)
    return math.hypot(dn, de)


# OSM node id, lat, lon, local x, local y (local = terrain pixel space in metres; world = local - 1054.72)
OSM_NODES = [
    (293747336, -23.5619084, -46.7201243, 1968.05, 916.81),
    (8012124527, -23.5603492, -46.7313340, 821.93, 1075.66),
    (392674113, -23.5595222, -46.7379477, 145.98, 1159.58),
    (251803975, -23.5536059, -46.7339740, 543.44, 1818.95),
    (1652258527, -23.5675133, -46.7303219, 934.81, 283.85),
    (1709222089, -23.5578265, -46.7187628, 2101.58, 1370.40),
]
TERRAIN_OFFSET = 1054.72


class UtmRoundTripTest(unittest.TestCase):
    POINTS = [
        (-23.56057, -46.72906),   # map centre
        (-23.5603492, -46.7313340),
        (-23.0, -45.0),           # on the central meridian
        (-20.0, -47.9),           # near the zone's west edge
        (-0.5, -42.1),            # near the equator, east edge
        (-33.5, -44.0),
    ]

    def test_forward_inverse_round_trip_under_1mm(self):
        for lat, lon in self.POINTS:
            with self.subTest(lat=lat, lon=lon):
                e, n = geo.utm_forward(lat, lon)
                lat2, lon2 = geo.utm_inverse(e, n)
                self.assertLess(distance_m(lat, lon, lat2, lon2), 0.001)
                e2, n2 = geo.utm_forward(lat2, lon2)
                self.assertLess(math.hypot(e2 - e, n2 - n), 0.001)

    def test_central_meridian_and_false_origin(self):
        e, n = geo.utm_forward(0.0, -45.0)
        self.assertAlmostEqual(e, 500000.0, places=6)
        self.assertAlmostEqual(n, 10000000.0, places=6)

    def test_scale_on_central_meridian_is_k0(self):
        # 1 km of northing along the central meridian ~ 1 km / k0 on the ground.
        lat1, lon1 = geo.utm_inverse(500000.0, 7392000.0)
        lat2, lon2 = geo.utm_inverse(500000.0, 7393000.0)
        self.assertAlmostEqual(lon1, -45.0, places=12)
        self.assertAlmostEqual(distance_m(lat1, lon1, lat2, lon2), 1000.0 / geo.UTM_K0, delta=0.5)


class WorldToWgs84Test(unittest.TestCase):
    def test_osm_nodes_within_1m(self):
        for node_id, lat, lon, lx, ly in OSM_NODES:
            with self.subTest(node=node_id):
                glat, glon, alt = geo.world_to_wgs84(lx - TERRAIN_OFFSET, ly - TERRAIN_OFFSET, 731.5)
                self.assertLess(distance_m(lat, lon, glat, glon), 1.0)
                self.assertEqual(alt, 731.5)

    def test_world_origin_is_near_map_centre(self):
        lat, lon, _ = geo.world_to_wgs84(0.0, 0.0, 0.0)
        self.assertLess(distance_m(lat, lon, -23.56057, -46.72906), 50.0)

    def test_axes_east_and_north(self):
        lat0, lon0, _ = geo.world_to_wgs84(0.0, 0.0, 0.0)
        lat_e, lon_e, _ = geo.world_to_wgs84(100.0, 0.0, 0.0)
        lat_n, lon_n, _ = geo.world_to_wgs84(0.0, 100.0, 0.0)
        self.assertGreater(lon_e, lon0)
        self.assertGreater(lat_n, lat0)
        self.assertAlmostEqual(distance_m(lat0, lon0, lat_e, lon_e), 100.0, delta=0.2)
        self.assertAlmostEqual(distance_m(lat0, lon0, lat_n, lon_n), 100.0, delta=0.2)


class RoundingTest(unittest.TestCase):
    def test_to_e7(self):
        self.assertEqual(geo.to_e7(-23.5603492), -235603492)
        self.assertEqual(geo.to_e7(-46.7313340), -467313340)
        self.assertEqual(geo.to_e7(-0.00000005), -1)    # -0.5 -> -1 (away from zero, like Dart)
        self.assertEqual(geo.to_e7(0.00000005), 1)
        self.assertEqual(geo.to_e7(-0.00000004), 0)
        self.assertEqual(geo.to_e7(-0.00000016), -2)
        self.assertEqual(geo.to_e7(-0.00000025), -3)    # Python's round() would give -2 (banker's)

    def test_round_half_away_from_zero(self):
        cases = {-2.5: -3, -1.5: -2, -0.5: -1, 0.5: 1, 1.5: 2, 2.5: 3,
                 -0.49999999999999994: 0, 0.49999999999999994: 0, -2.4: -2, 2.6: 3}
        for v, expected in cases.items():
            with self.subTest(v=v):
                self.assertEqual(geo._round_half_away(v), expected)

    def test_to_mm(self):
        self.assertEqual(geo.to_mm(728.16), 728160)
        self.assertEqual(geo.to_mm(-0.0005), -1)
        self.assertEqual(geo.to_mm(0.0004), 0)
        self.assertEqual(geo.to_mm(1e12), 2**31 - 1)


class PayloadTest(unittest.TestCase):
    GOLDEN = "01010100dcf9f4f1445d25e4601c0b00"

    def test_golden_vector(self):
        data = payload.encode(1, -235603492, -467313340, 728160, True)
        self.assertEqual(len(data), 16)
        self.assertEqual(data.hex(), self.GOLDEN)

    def test_golden_vector_from_degrees(self):
        data = payload.encode(1, geo.to_e7(-23.5603492), geo.to_e7(-46.7313340), geo.to_mm(728.160), True)
        self.assertEqual(data.hex(), self.GOLDEN)

    def test_decode_round_trip(self):
        self.assertEqual(payload.decode(bytes.fromhex(self.GOLDEN)),
                         (1, True, -235603492, -467313340, 728160))
        data = payload.encode(65535, 1, -2, 3, False)
        self.assertEqual(payload.decode(data), (65535, False, 1, -2, 3))

    def test_seq_wraps(self):
        self.assertEqual(payload.decode(payload.encode(65536, 0, 0, 0, True))[0], 0)
        self.assertEqual(payload.decode(payload.encode(65537, 0, 0, 0, True))[0], 1)

    def test_decode_rejects_bad_input(self):
        with self.assertRaises(ValueError):
            payload.decode(bytes(15))
        with self.assertRaises(ValueError):
            payload.decode(bytes.fromhex("02" + self.GOLDEN[2:]))


if __name__ == "__main__":
    unittest.main()
