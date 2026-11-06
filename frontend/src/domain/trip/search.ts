import type { Route } from "@/domain/trip/types";

/**
 * Makes a time vector increasing, replacing `null` separators
 * (between parts) with a value right after the previous one.
 */
export function fillTimes(ts: readonly (number | null)[]): number[] {
  const out: number[] = [];
  let previous = 0;
  for (let i = 0; i < ts.length; i++) {
    const v = ts[i];
    if (v === null || v === undefined) {
      previous += 1;
      out.push(previous);
    } else {
      previous = v;
      out.push(v);
    }
  }
  return out;
}

/** Index of the nearest time in an increasing vector. -1 if empty. */
export function nearestIndex(ts: readonly number[], t: number): number {
  if (ts.length === 0) return -1;
  let lo = 0;
  let hi = ts.length - 1;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if ((ts[mid] as number) < t) lo = mid + 1;
    else hi = mid;
  }
  const previous = lo > 0 ? lo - 1 : lo;
  return Math.abs((ts[previous] as number) - t) <= Math.abs((ts[lo] as number) - t) ? previous : lo;
}

export type RoutePoint = { index: number; t: number; lat: number; lon: number };

const GPS_TOLERANCE_MS = 5000;

/** Point of `route` nearest in time; null if there is no GPS at that instant. */
export function routePointAtTime(route: Route | null, t: number): RoutePoint | null {
  if (!route || route.t_ms.length === 0) return null;
  const times = fillTimes(route.t_ms);
  const i = nearestIndex(times, t);
  const lat = route.lat[i];
  const lon = route.lon[i];
  const ti = times[i];
  if (i < 0 || lat === null || lat === undefined || lon === null || lon === undefined) return null;
  if (ti === undefined || Math.abs(ti - t) > GPS_TOLERANCE_MS) return null;
  return { index: i, t: ti, lat, lon };
}

function meters(lat1: number, lon1: number, lat2: number, lon2: number): number {
  const k = 111_320;
  const dx = (lon2 - lon1) * k * Math.cos(((lat1 + lat2) / 2) * (Math.PI / 180));
  const dy = (lat2 - lat1) * k;
  return Math.hypot(dx, dy);
}

/** Route point nearest to a position, within `maxMeters`. */
export function routePointAtPlace(
  route: Route | null,
  lat: number,
  lon: number,
  maxMeters: number,
): RoutePoint | null {
  if (!route) return null;
  let best: RoutePoint | null = null;
  let smallest = Infinity;
  const times = fillTimes(route.t_ms);
  for (let i = 0; i < route.lat.length; i++) {
    const la = route.lat[i];
    const lo = route.lon[i];
    if (la === null || la === undefined || lo === null || lo === undefined) continue;
    const d = meters(lat, lon, la, lo);
    if (d < smallest) {
      smallest = d;
      best = { index: i, t: times[i] as number, lat: la, lon: lo };
    }
  }
  return best && smallest <= maxMeters ? best : null;
}
