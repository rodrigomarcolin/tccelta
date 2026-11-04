import { AXIS_PAUSE_FRACTION } from "@/config";

export type Interval = { start: number; end: number };

type Segment = { t0: number; t1: number; x0: number; x1: number; paused: boolean };

export type Axis = {
  /** Actual time (ms) to axis coordinate. Piecewise linear. */
  toX(t: number): number;
  /** Inverse of `toX`. */
  fromX(x: number): number;
  xMin: number;
  xMax: number;
  segments: Segment[];
  /** Compressed pauses, with their actual duration. */
  pauses: { x0: number; x1: number; durationMs: number }[];
};

/**
 * X axis with each pause between parts compressed to `fraction` of the sum of
 * the part durations. The transform is piecewise linear and invertible.
 */
export function createAxis(intervals: Interval[], fraction = AXIS_PAUSE_FRACTION): Axis {
  const sorted = [...intervals].sort((a, b) => a.start - b.start);
  const total = sorted.reduce((s, i) => s + Math.max(0, i.end - i.start), 0);
  const segments: Segment[] = [];
  const pauses: Axis["pauses"] = [];
  let x = 0;
  sorted.forEach((iv, i) => {
    const previous = sorted[i - 1];
    if (previous) {
      const actual = iv.start - previous.end;
      if (actual > 0) {
        const shown = Math.min(actual, fraction * total);
        segments.push({ t0: previous.end, t1: iv.start, x0: x, x1: x + shown, paused: true });
        pauses.push({ x0: x, x1: x + shown, durationMs: actual });
        x += shown;
      }
    }
    const len = Math.max(0, iv.end - iv.start);
    segments.push({ t0: iv.start, t1: iv.end, x0: x, x1: x + len, paused: false });
    x += len;
  });
  if (segments.length === 0) segments.push({ t0: 0, t1: 1, x0: 0, x1: 1, paused: false });

  const first = segments[0]!;
  const last = segments[segments.length - 1]!;

  const toX = (t: number): number => {
    if (t <= first.t0) return first.x0 + (t - first.t0);
    if (t >= last.t1) return last.x1 + (t - last.t1);
    for (const tr of segments) {
      if (t <= tr.t1) {
        const dt = tr.t1 - tr.t0;
        return dt === 0 ? tr.x0 : tr.x0 + ((t - tr.t0) / dt) * (tr.x1 - tr.x0);
      }
    }
    return last.x1;
  };

  const fromX = (xv: number): number => {
    if (xv <= first.x0) return first.t0 + (xv - first.x0);
    if (xv >= last.x1) return last.t1 + (xv - last.x1);
    for (const tr of segments) {
      if (xv <= tr.x1) {
        const dx = tr.x1 - tr.x0;
        return dx === 0 ? tr.t0 : tr.t0 + ((xv - tr.x0) / dx) * (tr.t1 - tr.t0);
      }
    }
    return last.t1;
  };

  return { toX, fromX, xMin: first.x0, xMax: last.x1, segments, pauses };
}
