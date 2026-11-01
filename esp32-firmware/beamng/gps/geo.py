"""BeamNG world coordinates -> WGS84/SIRGAS 2000 lat/lon, without pyproj.

UTM uses the Krüger series (3 terms, n^3) on the GRS80 ellipsoid, which is the
ellipsoid of SIRGAS 2000 (EPSG:4674/31983). SIRGAS 2000 and WGS84 differ only by a
few decimetres of plate motion since 2000.4, so the result is used as GPS lat/lon directly.

The world -> UTM transform of each map is a scale + offset taken from the map's
BeamNG Mapping Tools export (MT_settings.json + TerrainBlock):
    s = terrain_size_m / native_size_m                     (world metres per UTM metre)
    E = native_x0 + (x - terrain_x0) / s                   (+X = east)
    N = native_y0 + (y - terrain_y0 + y_bias_m) / s        (+Y = north)
    alt = z                                                (world z is already elevation)
"""
import math

# --- GRS80 ---
A_AXIS = 6378137.0
FLATTENING = 1 / 298.257222101

UTM_K0 = 0.9996
UTM_FALSE_EASTING = 500000.0
UTM_FALSE_NORTHING_SOUTH = 10000000.0

_N = FLATTENING / (2 - FLATTENING)
_N2, _N3 = _N * _N, _N * _N * _N
_RECT_A = A_AXIS / (1 + _N) * (1 + _N2 / 4)  # rectifying radius (n^4 term dropped)
_ALPHA = (_N / 2 - 2 * _N2 / 3 + 5 * _N3 / 16,
          13 * _N2 / 48 - 3 * _N3 / 5,
          61 * _N3 / 240)
_BETA = (_N / 2 - 2 * _N2 / 3 + 37 * _N3 / 96,
         _N2 / 48 + _N3 / 15,
         17 * _N3 / 480)
_DELTA = (2 * _N - 2 * _N2 / 3 - 2 * _N3,
          7 * _N2 / 3 - 8 * _N3 / 5,
          56 * _N3 / 15)
_E_FACTOR = 2 * math.sqrt(_N) / (1 + _N)  # first eccentricity e

# Map georeference. cid_exp3 = Cidade Universitária (USP) cut, SIRGAS 2000 / UTM 23S
# (EPSG:31983), native bbox X 322470.511-324580.511, Y 7392303.171-7394413.171 (2110 m);
# terrain 2048 px x 1.03 m = 2109.44 m at TerrainBlock.position (-1054.72, -1054.72).
# y_bias_m = one pixel row of bias, measured against OSM nodes and the decal roads.
MAPS = {
    "cid_exp3": {
        "zone": 23,
        "south": True,
        "native_x0": 322470.511,
        "native_y0": 7392303.171,
        "native_size_m": 2110.0,
        "terrain_x0": -1054.72,
        "terrain_y0": -1054.72,
        "terrain_size_m": 2109.44,
        "y_bias_m": 1.0,
    },
}


def _central_meridian(zone):
    return math.radians(zone * 6 - 183)


def utm_forward(lat, lon, zone=23, south=True):
    """(lat, lon) in degrees -> (E, N) in metres on the given UTM zone."""
    phi = math.radians(lat)
    dlam = math.radians(lon) - _central_meridian(zone)
    t = math.sinh(math.atanh(math.sin(phi)) - _E_FACTOR * math.atanh(_E_FACTOR * math.sin(phi)))
    xi_p = math.atan2(t, math.cos(dlam))
    eta_p = math.atanh(math.sin(dlam) / math.sqrt(1 + t * t))
    xi, eta = xi_p, eta_p
    for j, a in enumerate(_ALPHA, start=1):
        xi += a * math.sin(2 * j * xi_p) * math.cosh(2 * j * eta_p)
        eta += a * math.cos(2 * j * xi_p) * math.sinh(2 * j * eta_p)
    easting = UTM_FALSE_EASTING + UTM_K0 * _RECT_A * eta
    northing = (UTM_FALSE_NORTHING_SOUTH if south else 0.0) + UTM_K0 * _RECT_A * xi
    return easting, northing


def utm_inverse(easting, northing, zone=23, south=True):
    """(E, N) in metres on the given UTM zone -> (lat, lon) in degrees."""
    xi = (northing - (UTM_FALSE_NORTHING_SOUTH if south else 0.0)) / (UTM_K0 * _RECT_A)
    eta = (easting - UTM_FALSE_EASTING) / (UTM_K0 * _RECT_A)
    xi_p, eta_p = xi, eta
    for j, b in enumerate(_BETA, start=1):
        xi_p -= b * math.sin(2 * j * xi) * math.cosh(2 * j * eta)
        eta_p -= b * math.cos(2 * j * xi) * math.sinh(2 * j * eta)
    chi = math.asin(math.sin(xi_p) / math.cosh(eta_p))
    phi = chi
    for j, d in enumerate(_DELTA, start=1):
        phi += d * math.sin(2 * j * chi)
    lam = _central_meridian(zone) + math.atan2(math.sinh(eta_p), math.cos(xi_p))
    return math.degrees(phi), math.degrees(lam)


def world_to_utm(x, y, map_name="cid_exp3"):
    """BeamNG world (x, y) in metres -> (E, N) on the map's native UTM zone."""
    m = MAPS[map_name]
    s = m["terrain_size_m"] / m["native_size_m"]
    easting = m["native_x0"] + (x - m["terrain_x0"]) / s
    northing = m["native_y0"] + (y - m["terrain_y0"] + m["y_bias_m"]) / s
    return easting, northing


def world_to_wgs84(x, y, z, map_name="cid_exp3"):
    """BeamNG world (x, y, z) in metres -> (lat, lon, alt_m)."""
    m = MAPS[map_name]
    easting, northing = world_to_utm(x, y, map_name)
    lat, lon = utm_inverse(easting, northing, m["zone"], m["south"])
    return lat, lon, z


def _round_half_away(v):
    """Same as Dart's double.round(): ties go away from zero."""
    a = abs(v)
    r = math.floor(a)
    if a - r >= 0.5:  # exact subtraction; floor(a + 0.5) misrounds 0.49999999999999994
        r += 1
    return int(-r if v < 0 else r)


def to_e7(deg):
    """Degrees -> integer 1e-7 degrees, like the app's GpsFix.fromDegrees."""
    return _round_half_away(deg * 1e7)


_INT32_MIN, _INT32_MAX = -(1 << 31), (1 << 31) - 1


def to_mm(m):
    """Metres -> integer millimetres (clamped to int32), like the app's GpsFix.fromDegrees."""
    return max(_INT32_MIN, min(_INT32_MAX, _round_half_away(m * 1000)))
