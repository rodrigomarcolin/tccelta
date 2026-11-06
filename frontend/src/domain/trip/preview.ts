import type { RoutePreviewData } from "./types";

/** Side of a map tile in pixels at scale 1. */
export const TILE_SIZE = 256;
const MAX_ZOOM = 17;
/** Zoom of a trip that never moved (a single point): street level. */
const POINT_ZOOM = 16;
const MAX_LATITUDE = 85.0511;

export type PreviewSize = { width: number; height: number };

export type PreviewTile = {
  z: number;
  x: number;
  y: number;
  /** Position and size inside the box, in %. */
  left: number;
  top: number;
  width: number;
  height: number;
};

export type PreviewPoint = { x: number; y: number };

export type PreviewLayout = {
  zoom: number;
  tiles: PreviewTile[];
  /** SVG path of each stretch with two or more points. */
  paths: string[];
  /** Stretches of a single point (a trip that stayed put). */
  dots: PreviewPoint[];
  start: PreviewPoint;
  end: PreviewPoint;
};

type Pair = [number, number];

/** Web Mercator, in pixels of the whole world at `zoom`. */
function project([lat, lon]: Pair, zoom: number): PreviewPoint {
  const scale = TILE_SIZE * 2 ** zoom;
  const clamped = Math.max(-MAX_LATITUDE, Math.min(MAX_LATITUDE, lat));
  const sin = Math.sin((clamped * Math.PI) / 180);
  return {
    x: ((lon + 180) / 360) * scale,
    y: (0.5 - Math.log((1 + sin) / (1 - sin)) / (4 * Math.PI)) * scale,
  };
}

const valid = (p: unknown): p is Pair =>
  Array.isArray(p) && p.length === 2 && Number.isFinite(p[0]) && Number.isFinite(p[1]);

const round1 = (v: number) => Math.round(v * 10) / 10;

/**
 * Where everything goes in a minimap of `size` pixels: the highest whole zoom at which the
 * route fits with `margin` to spare (so the tiles stay at scale 1 and at most four cover the
 * box), the tiles under it with their position in %, and the route in box pixels.
 * `null` when there is no usable point.
 */
export function layoutPreview(
  route: RoutePreviewData | null | undefined,
  size: PreviewSize,
  margin = 14,
): PreviewLayout | null {
  const stretches = (route ?? []).map((s) => (Array.isArray(s) ? s.filter(valid) : []));
  const all = stretches.flat();
  if (all.length === 0) return null;

  // Extent at zoom 0, where the world is one tile wide.
  const flat = all.map((p) => project(p, 0));
  const xs = flat.map((p) => p.x);
  const ys = flat.map((p) => p.y);
  const spanX = Math.max(...xs) - Math.min(...xs);
  const spanY = Math.max(...ys) - Math.min(...ys);
  const availX = Math.max(1, size.width - 2 * margin);
  const availY = Math.max(1, size.height - 2 * margin);
  const fit =
    spanX === 0 && spanY === 0
      ? POINT_ZOOM
      : Math.floor(
          Math.log2(
            Math.min(
              spanX === 0 ? Infinity : availX / spanX,
              spanY === 0 ? Infinity : availY / spanY,
            ),
          ),
        );
  const zoom = Math.max(0, Math.min(MAX_ZOOM, fit));

  const zeroToZoom = 2 ** zoom;
  const cx = ((Math.max(...xs) + Math.min(...xs)) / 2) * zeroToZoom;
  const cy = ((Math.max(...ys) + Math.min(...ys)) / 2) * zeroToZoom;
  const originX = cx - size.width / 2;
  const originY = cy - size.height / 2;

  const toBox = (p: Pair): PreviewPoint => {
    const w = project(p, zoom);
    return { x: round1(w.x - originX), y: round1(w.y - originY) };
  };

  const tiles: PreviewTile[] = [];
  const count = 2 ** zoom;
  const x0 = Math.floor(originX / TILE_SIZE);
  const x1 = Math.floor((originX + size.width - 1e-6) / TILE_SIZE);
  const y0 = Math.floor(originY / TILE_SIZE);
  const y1 = Math.floor((originY + size.height - 1e-6) / TILE_SIZE);
  for (let ty = y0; ty <= y1; ty++) {
    if (ty < 0 || ty >= count) continue;
    for (let tx = x0; tx <= x1; tx++) {
      tiles.push({
        z: zoom,
        x: ((tx % count) + count) % count,
        y: ty,
        left: ((tx * TILE_SIZE - originX) / size.width) * 100,
        top: ((ty * TILE_SIZE - originY) / size.height) * 100,
        width: (TILE_SIZE / size.width) * 100,
        height: (TILE_SIZE / size.height) * 100,
      });
    }
  }

  const paths: string[] = [];
  const dots: PreviewPoint[] = [];
  for (const s of stretches) {
    if (s.length === 1) dots.push(toBox(s[0]));
    else if (s.length > 1) {
      paths.push(
        s
          .map(toBox)
          .map((p, i) => `${i === 0 ? "M" : "L"}${p.x} ${p.y}`)
          .join(" "),
      );
    }
  }

  const nonEmpty = stretches.filter((s) => s.length > 0);
  const first = nonEmpty[0];
  const last = nonEmpty[nonEmpty.length - 1];
  return {
    zoom,
    tiles,
    paths,
    dots,
    start: toBox(first[0]),
    end: toBox(last[last.length - 1]),
  };
}

/** Tile URL from a template with `{z}`, `{x}` and `{y}`. */
export function tileUrl(template: string, t: Pick<PreviewTile, "z" | "x" | "y">): string {
  return template
    .replace("{z}", String(t.z))
    .replace("{x}", String(t.x))
    .replace("{y}", String(t.y));
}
