import { describe, expect, it } from "vitest";
import { createAxis } from "./axis";

const HOUR = 3_600_000;

describe("createAxis", () => {
  it("is the shifted identity with a single part", () => {
    const e = createAxis([{ start: 1000, end: 5000 }]);
    expect(e.toX(1000)).toBe(0);
    expect(e.toX(3000)).toBe(2000);
    expect(e.fromX(2000)).toBe(3000);
    expect(e.pauses).toHaveLength(0);
  });

  it("compresses a 5 h pause to 5% of the sum of the durations", () => {
    const e = createAxis([
      { start: 0, end: HOUR },
      { start: 6 * HOUR, end: 7 * HOUR },
    ]);
    expect(e.pauses).toHaveLength(1);
    expect(e.pauses[0].durationMs).toBe(5 * HOUR);
    const width = e.pauses[0].x1 - e.pauses[0].x0;
    expect(width).toBeCloseTo(0.05 * 2 * HOUR);
    const share = width / (e.xMax - e.xMin);
    expect(share).toBeGreaterThan(0.045);
    expect(share).toBeLessThan(0.055);
  });

  it("has an exact inverse on both sides of the pause", () => {
    const e = createAxis([
      { start: 0, end: HOUR },
      { start: 6 * HOUR, end: 7 * HOUR },
    ]);
    for (const t of [0, 1234, HOUR, 6 * HOUR, 6 * HOUR + 999, 7 * HOUR, 3 * HOUR]) {
      expect(e.fromX(e.toX(t))).toBeCloseTo(t, 3);
    }
    expect(e.fromX(e.toX(HOUR - 1000))).toBeCloseTo(HOUR - 1000);
    expect(e.fromX(e.toX(6 * HOUR + 1000))).toBeCloseTo(6 * HOUR + 1000);
  });

  it("is monotonic and extrapolates outside the bounds", () => {
    const e = createAxis([
      { start: 100, end: 200 },
      { start: 1000, end: 1100 },
    ]);
    expect(e.toX(50)).toBe(-50);
    expect(e.fromX(e.toX(50))).toBe(50);
    expect(e.toX(5000)).toBeGreaterThan(e.toX(1100));
    expect(e.fromX(e.xMax + 10)).toBe(1110);
  });

  it("does not widen a pause smaller than the limit", () => {
    const e = createAxis([
      { start: 0, end: 100_000 },
      { start: 101_000, end: 200_000 },
    ]);
    expect(e.pauses[0].x1 - e.pauses[0].x0).toBe(1000);
  });

  it("accepts out-of-order and touching parts and an empty list", () => {
    const e = createAxis([
      { start: 500, end: 900 },
      { start: 0, end: 500 },
    ]);
    expect(e.pauses).toHaveLength(0);
    expect(e.toX(500)).toBe(500);
    expect(createAxis([]).toX(0)).toBe(0);
  });
});
