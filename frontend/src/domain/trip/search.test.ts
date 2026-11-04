import { describe, expect, it } from "vitest";
import type { Route } from "@/domain/trip/types";
import { nearestIndex, routePointAtPlace, routePointAtTime, fillTimes } from "./search";

const route: Route = {
  t_ms: [0, 1000, 2000, 10000],
  lat: [-25.4, -25.401, -25.402, -25.41],
  lon: [-49.27, -49.271, -49.272, -49.28],
  segmentos: { velocidade_kmh: [10, 10, null], faixa: ["ate_30", "ate_30", "falha_gps"] },
};

describe("fillTimes", () => {
  it("replaces null separators with a value right after the previous one", () => {
    expect(fillTimes([10, 20, null, 50])).toEqual([10, 20, 21, 50]);
    expect(fillTimes([null, 5])).toEqual([1, 5]);
  });
});

describe("nearestIndex", () => {
  const ts = [0, 100, 200, 300];
  it("finds the nearest neighbour", () => {
    expect(nearestIndex(ts, 140)).toBe(1);
    expect(nearestIndex(ts, 160)).toBe(2);
    expect(nearestIndex(ts, -50)).toBe(0);
    expect(nearestIndex(ts, 999)).toBe(3);
  });
  it("returns -1 when empty", () => {
    expect(nearestIndex([], 1)).toBe(-1);
  });
});

describe("routePointAtTime", () => {
  it("finds the point nearest in time", () => {
    expect(routePointAtTime(route, 1100)?.index).toBe(1);
  });
  it("returns null without GPS", () => {
    expect(routePointAtTime(null, 1)).toBeNull();
  });
  it("rejects an instant more than 5 s from the point", () => {
    const r: Route = { ...route, t_ms: [0, 20000], lat: [1, 2], lon: [1, 2] };
    expect(routePointAtTime(r, 10000)).toBeNull();
  });
  it("handles null coordinates", () => {
    const r: Route = { ...route, lat: [null, null, null, null], lon: [null, null, null, null] };
    expect(routePointAtTime(r, 0)).toBeNull();
  });
});

describe("routePointAtPlace", () => {
  it("finds the nearby point and respects the maximum distance", () => {
    expect(routePointAtPlace(route, -25.4011, -49.2711, 50)?.index).toBe(1);
    expect(routePointAtPlace(route, -25.5, -49.5, 50)).toBeNull();
    expect(routePointAtPlace(null, 0, 0, 10)).toBeNull();
  });
  it("ignores points without coordinates", () => {
    const r: Route = {
      ...route,
      lat: [null, -25.401, null, null],
      lon: [null, -49.271, null, null],
    };
    expect(routePointAtPlace(r, -25.401, -49.271, 10)?.index).toBe(1);
  });
});
