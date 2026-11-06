import { describe, expect, it } from "vitest";
import { layoutPreview, tileUrl, TILE_SIZE } from "./preview";
import type { RoutePreviewData } from "./types";

const SIZE = { width: 220, height: 132 };

// About 3.3 km of a street in São Paulo.
const ROUTE: RoutePreviewData = [
  [
    [-23.55, -46.64],
    [-23.54, -46.63],
    [-23.53, -46.62],
  ],
];

describe("layoutPreview", () => {
  it("is null without usable points", () => {
    expect(layoutPreview(null, SIZE)).toBeNull();
    expect(layoutPreview([], SIZE)).toBeNull();
    expect(layoutPreview([[]], SIZE)).toBeNull();
    expect(layoutPreview([[[Number.NaN, 1]]], SIZE)).toBeNull();
  });

  it("picks the zoom where the route fits with a margin", () => {
    const layout = layoutPreview(ROUTE, SIZE, 14)!;
    expect(layout.zoom).toBeGreaterThanOrEqual(10);
    expect(layout.zoom).toBeLessThanOrEqual(17);
    const points = [...layout.paths[0].matchAll(/[ML]([\d.-]+) ([\d.-]+)/g)].map((m) => [
      Number(m[1]),
      Number(m[2]),
    ]);
    expect(points).toHaveLength(3);
    for (const [x, y] of points) {
      expect(x).toBeGreaterThanOrEqual(14 - 0.5);
      expect(x).toBeLessThanOrEqual(SIZE.width - 14 + 0.5);
      expect(y).toBeGreaterThanOrEqual(14 - 0.5);
      expect(y).toBeLessThanOrEqual(SIZE.height - 14 + 0.5);
    }
  });

  it("uses the next zoom up only when the route no longer fits", () => {
    const layout = layoutPreview(ROUTE, SIZE, 14)!;
    // One level closer doubles the extent: it must overflow the available box.
    const closer = layoutPreview(
      ROUTE.map((s) => s.map(([la, lo]) => [la, lo] as [number, number])),
      { width: SIZE.width * 2, height: SIZE.height * 2 },
      28,
    )!;
    expect(closer.zoom).toBe(layout.zoom + 1);
  });

  it("covers the box with at most four tiles that match the position in %", () => {
    const layout = layoutPreview(ROUTE, SIZE)!;
    expect(layout.tiles.length).toBeGreaterThanOrEqual(1);
    expect(layout.tiles.length).toBeLessThanOrEqual(4);
    for (const t of layout.tiles) {
      expect(t.z).toBe(layout.zoom);
      expect(t.width).toBeCloseTo((TILE_SIZE / SIZE.width) * 100);
      expect(t.height).toBeCloseTo((TILE_SIZE / SIZE.height) * 100);
    }
    // Together they cover the whole box.
    const left = Math.min(...layout.tiles.map((t) => t.left));
    const top = Math.min(...layout.tiles.map((t) => t.top));
    const right = Math.max(...layout.tiles.map((t) => t.left + t.width));
    const bottom = Math.max(...layout.tiles.map((t) => t.top + t.height));
    expect(left).toBeLessThanOrEqual(0.0001);
    expect(top).toBeLessThanOrEqual(0.0001);
    expect(right).toBeGreaterThanOrEqual(99.9999);
    expect(bottom).toBeGreaterThanOrEqual(99.9999);
    // Neighbouring tiles are exactly one tile apart.
    const xs = [...new Set(layout.tiles.map((t) => t.x))].sort((a, b) => a - b);
    for (let i = 1; i < xs.length; i++) expect(xs[i] - xs[i - 1]).toBe(1);
  });

  it("a narrow box needs no more than four tiles either", () => {
    const layout = layoutPreview(ROUTE, { width: 110, height: 132 })!;
    expect(layout.tiles.length).toBeLessThanOrEqual(4);
  });

  it("a single point draws a dot at the center and no path", () => {
    const layout = layoutPreview([[[-23.55, -46.63]]], SIZE)!;
    expect(layout.paths).toEqual([]);
    expect(layout.dots).toEqual([{ x: SIZE.width / 2, y: SIZE.height / 2 }]);
    expect(layout.zoom).toBe(16);
    expect(layout.start).toEqual(layout.end);
  });

  it("a gap keeps separate stretches, and a lone point in between becomes a dot", () => {
    const layout = layoutPreview(
      [
        [
          [-23.55, -46.64],
          [-23.545, -46.635],
        ],
        [[-23.54, -46.63]],
        [
          [-23.535, -46.625],
          [-23.53, -46.62],
        ],
      ],
      SIZE,
    )!;
    expect(layout.paths).toHaveLength(2);
    expect(layout.dots).toHaveLength(1);
    expect(layout.start.x).toBeLessThan(layout.end.x);
  });

  it("start and end are the first and last point of the route", () => {
    const layout = layoutPreview(ROUTE, SIZE)!;
    // North is up: the first point is south (below) and west (left) of the last.
    expect(layout.start.y).toBeGreaterThan(layout.end.y);
    expect(layout.start.x).toBeLessThan(layout.end.x);
  });

  it("clamps the zoom for a tiny route and for a route across the world", () => {
    const tiny = layoutPreview(
      [
        [
          [-23.55, -46.63],
          [-23.5500001, -46.6300001],
        ],
      ],
      SIZE,
    )!;
    expect(tiny.zoom).toBe(17);
    const huge = layoutPreview(
      [
        [
          [-60, -170],
          [70, 170],
        ],
      ],
      SIZE,
    )!;
    expect(huge.zoom).toBe(0);
    expect(huge.tiles.length).toBeGreaterThan(0);
    for (const t of huge.tiles) {
      expect(t.x).toBeGreaterThanOrEqual(0);
      expect(t.y).toBeGreaterThanOrEqual(0);
    }
  });
});

describe("tileUrl", () => {
  it("fills the template", () => {
    expect(tileUrl("https://t/{z}/{x}/{y}.png", { z: 5, x: 3, y: 9 })).toBe("https://t/5/3/9.png");
  });
});
