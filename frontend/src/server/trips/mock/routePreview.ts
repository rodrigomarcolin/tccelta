import type { Route, RoutePreviewData } from "@/domain/trip/types";

/** Same limit the real API applies to `rota_resumo`. */
export const PREVIEW_MAX_POINTS = 120;

const round5 = (v: number) => Math.round(v * 1e5) / 1e5;

/** Keeps `keep` points of a stretch, evenly spaced and always including the first and last. */
function sample<T>(points: T[], keep: number): T[] {
  if (points.length <= keep) return points;
  const out: T[] = [];
  for (let i = 0; i < keep; i++)
    out.push(points[Math.round((i * (points.length - 1)) / (keep - 1))]);
  return out;
}

/**
 * Route summary of a mock analysis: stretches cut where the segment band is `falha_gps` (or a
 * position is missing), sampled to at most `PREVIEW_MAX_POINTS` points. `null` without GPS.
 * `rotationDeg` turns the route around its first point, so the seeded list entries differ.
 */
export function previewOf(route: Route | null, rotationDeg = 0): RoutePreviewData | null {
  if (!route) return null;
  const stretches: [number, number][][] = [];
  let current: [number, number][] = [];
  const close = () => {
    if (current.length) stretches.push(current);
    current = [];
  };
  for (let i = 0; i < route.lat.length; i++) {
    const lat = route.lat[i];
    const lon = route.lon[i];
    if (lat === null || lon === null || lat === undefined || lon === undefined) {
      close();
      continue;
    }
    current.push([lat, lon]);
    if (route.segmentos.faixa[i] === "falha_gps") close();
  }
  close();
  if (!stretches.length) return null;

  const total = stretches.reduce((n, s) => n + s.length, 0);
  const [lat0, lon0] = stretches[0][0];
  const k = Math.cos((lat0 * Math.PI) / 180);
  const angle = (rotationDeg * Math.PI) / 180;
  const turn = ([lat, lon]: [number, number]): [number, number] => {
    if (!angle) return [round5(lat), round5(lon)];
    const x = (lon - lon0) * k;
    const y = lat - lat0;
    return [
      round5(lat0 + x * Math.sin(angle) + y * Math.cos(angle)),
      round5(lon0 + (x * Math.cos(angle) - y * Math.sin(angle)) / k),
    ];
  };

  return stretches.map((s) => {
    const keep =
      s.length <= 1 ? s.length : Math.max(2, Math.floor((s.length * PREVIEW_MAX_POINTS) / total));
    return sample(s, keep).map(turn);
  });
}
