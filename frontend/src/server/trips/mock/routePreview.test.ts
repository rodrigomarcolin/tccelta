import { describe, expect, it } from "vitest";
import type { Route, SegmentBand } from "@/domain/trip/types";
import { PREVIEW_MAX_POINTS, previewOf } from "./routePreview";

function route(n: number, gapAt: number[] = []): Route {
  const faixa: SegmentBand[] = Array.from({ length: n - 1 }, (_, i) =>
    gapAt.includes(i) ? "falha_gps" : "30_60",
  );
  return {
    t_ms: Array.from({ length: n }, (_, i) => i * 1000),
    lat: Array.from({ length: n }, (_, i) => -23.5 + i * 0.0001),
    lon: Array.from({ length: n }, (_, i) => -46.6 + i * 0.0001),
    segmentos: { velocidade_kmh: Array(n - 1).fill(40), faixa },
  };
}

describe("previewOf", () => {
  it("is null without a route", () => {
    expect(previewOf(null)).toBeNull();
  });

  it("cuts a stretch where the segment is a GPS gap", () => {
    const stretches = previewOf(route(10, [3]))!;
    expect(stretches).toHaveLength(2);
    expect(stretches[0]).toHaveLength(4);
    expect(stretches[1]).toHaveLength(6);
  });

  it("cuts at a missing position", () => {
    const r = route(6);
    r.lat[2] = null;
    r.lon[2] = null;
    expect(previewOf(r)!.map((s) => s.length)).toEqual([2, 3]);
  });

  it("samples to the limit, keeping the ends, rounded to 5 decimals", () => {
    const r = route(1000);
    const lat = r.lat as number[];
    const lon = r.lon as number[];
    const [stretch] = previewOf(r)!;
    expect(stretch.length).toBeLessThanOrEqual(PREVIEW_MAX_POINTS);
    expect(stretch[0]).toEqual([Math.round(lat[0] * 1e5) / 1e5, Math.round(lon[0] * 1e5) / 1e5]);
    expect(stretch[stretch.length - 1][0]).toBeCloseTo(lat[999], 5);
    for (const [lat] of stretch) expect(String(lat).split(".")[1].length).toBeLessThanOrEqual(5);
  });

  it("keeps a single-point stretch", () => {
    expect(previewOf(route(1))).toEqual([[[-23.5, -46.6]]]);
  });

  it("rotation moves the route around its first point", () => {
    const plain = previewOf(route(20))!;
    const turned = previewOf(route(20), 90)!;
    expect(turned[0][0]).toEqual(plain[0][0]);
    expect(turned[0][19]).not.toEqual(plain[0][19]);
  });
});
