import { render, screen } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import type { ChartProps } from "./UPlotChart";

type Opts = {
  hooks: { setCursor: ((u: Fake) => void)[]; draw: ((u: Fake) => void)[] };
  axes: { values: (u: Fake, s: number[]) => string[]; splits?: () => number[] }[];
  series: { stroke?: string; dash?: number[]; paths?: unknown }[];
  scales: { y: { range?: () => [number, number] } };
  cursor: { sync?: { key: string } };
};

const { Fake } = vi.hoisted(() => {
  class FakeUPlot {
    static instances: FakeUPlot[] = [];
    static paths = { stepped: () => "step" };
    static ctxCalls: string[] = [];
    static hookOnCreate = false;
    cursor: { idx: number | null; left: number | null } = { idx: null, left: -1 };
    data: number[][];
    bbox = { left: 0, top: 0, width: 400, height: 100 };
    ctx = new Proxy(
      {},
      {
        get: (_t, p) =>
          p === "canvas" ? {} : FakeUPlot.ctxCalls.push(String(p)) && (() => undefined),
        set: () => true,
      },
    ) as unknown as CanvasRenderingContext2D;
    setCursor = vi.fn();
    setSize = vi.fn();
    destroy = vi.fn();
    valToPos = vi.fn((v: number) => v / 10);
    constructor(
      public opts: Opts,
      data: number[][],
      public el: HTMLElement,
    ) {
      this.data = data;
      FakeUPlot.instances.push(this);
      // The real uPlot fires the cursor hook during creation, with the cursor outside.
      if (FakeUPlot.hookOnCreate) for (const h of opts.hooks.setCursor) h(this);
    }
  }
  return { Fake: FakeUPlot };
});
type Fake = InstanceType<typeof Fake>;

vi.mock("uplot", () => ({ default: Fake }));

class RO {
  static last: RO;
  constructor(public cb: () => void) {
    RO.last = this;
  }
  observe = vi.fn();
  disconnect = vi.fn();
}
vi.stubGlobal("ResizeObserver", RO);

import UPlotChart from "./UPlotChart";
import { renderWithTheme } from "@/tests/helpers";

const base = (extra: Partial<ChartProps> = {}): ChartProps => ({
  title: "Velocidade",
  x: [0, 100, 200],
  series: [{ label: "v", values: [1, null, 3], color: "--accent" }],
  formatX: (x) => `t${x}`,
  ...extra,
});

beforeEach(() => {
  Fake.instances = [];
  Fake.ctxCalls = [];
});

describe("UPlotChart", () => {
  it("creates the chart with the data, sync and formatted axes", () => {
    const fmtY = vi.fn((v: number) => `y${v}`);
    renderWithTheme(
      <UPlotChart {...base({ sync: "timeline", formatY: fmtY, ticksY: [0, 1], bandY: [0, 5] })} />,
    );
    const u = Fake.instances[0];
    expect(u.data).toEqual([
      [0, 100, 200],
      [1, null, 3],
    ]);
    expect(u.opts.cursor.sync).toEqual({ key: "timeline" });
    expect(u.opts.axes[0].values(u, [0, 100])).toEqual(["t0", "t100"]);
    expect(u.opts.axes[1].values(u, [0, 1])).toEqual(["y0", "y1"]);
    expect(u.opts.axes[1].splits?.()).toEqual([0, 1]);
    expect(u.opts.scales.y.range?.()).toEqual([0, 5]);
    expect(screen.getByRole("img", { name: "Velocidade" })).toBeInTheDocument();
  });

  it("uses steps and dashes when requested, and a literal color", () => {
    renderWithTheme(
      <UPlotChart
        {...base({
          series: [{ label: "m", values: [1, 2, 3], color: "#123456", step: true, dashed: true }],
        })}
      />,
    );
    const s = Fake.instances[0].opts.series[1];
    expect(s.stroke).toBe("#123456");
    expect(s.dash).toEqual([4, 4]);
    expect(s.paths).toBe("step");
  });

  it("destroys the chart on unmount and follows resizing", () => {
    const { unmount } = renderWithTheme(<UPlotChart {...base()} />);
    const u = Fake.instances[0];
    RO.last.cb();
    expect(u.setSize).toHaveBeenCalled();
    unmount();
    expect(u.destroy).toHaveBeenCalled();
    expect(RO.last.disconnect).toHaveBeenCalled();
  });

  it("forwards the mouse cursor as an X coordinate, or null when leaving", () => {
    const onCursorX = vi.fn();
    renderWithTheme(<UPlotChart {...base({ onCursorX })} />);
    const u = Fake.instances[0];
    u.cursor = { idx: 1, left: 30 };
    u.opts.hooks.setCursor[0](u);
    expect(onCursorX).toHaveBeenLastCalledWith(100);
    u.cursor = { idx: null, left: -1 };
    u.opts.hooks.setCursor[0](u);
    expect(onCursorX).toHaveBeenLastCalledWith(null);
  });

  it("ignores the cursor event without a callback", () => {
    renderWithTheme(<UPlotChart {...base()} />);
    const u = Fake.instances[0];
    u.cursor = { idx: 1, left: 30 };
    expect(() => u.opts.hooks.setCursor[0](u)).not.toThrow();
  });

  it("applies the cursor coming from outside without firing the callback", () => {
    const onCursorX = vi.fn();
    const p = base({ onCursorX, cursorX: 100, applyCursor: true });
    const { rerender } = render(<UPlotChart {...p} />);
    const u = Fake.instances[0];
    expect(u.setCursor).toHaveBeenCalledWith({ left: 10, top: 10 });
    // The programmatic setCursor fires the hook, which must be ignored.
    u.setCursor.mockImplementation(() => {
      u.cursor = { idx: 1, left: 10 };
      u.opts.hooks.setCursor[0](u);
    });
    rerender(<UPlotChart {...p} cursorX={200} />);
    expect(onCursorX).not.toHaveBeenCalled();
    rerender(<UPlotChart {...p} cursorX={null} />);
    expect(u.setCursor).toHaveBeenLastCalledWith({ left: -10, top: -10 });
  });

  it("does not apply the cursor when it came from the chart itself", () => {
    renderWithTheme(<UPlotChart {...base({ cursorX: 100, applyCursor: false })} />);
    expect(Fake.instances[0].setCursor).not.toHaveBeenCalled();
  });

  it("draws markers, pauses and the reference line", () => {
    renderWithTheme(
      <UPlotChart
        {...base({
          markers: [
            { x: 0, y: 1, shape: "tri-down" },
            { x: 100, y: 1, shape: "tri-up" },
            { x: 200, y: 1, shape: "circle" },
            { x: 200, y: 2, shape: "circle-empty" },
          ],
          pauses: [{ x0: 50, x1: 80, label: "Sem dados · 5 h 00 min" }],
          referenceY: { y: 80, label: "Motor aquecido (80 °C)" },
        })}
      />,
    );
    const u = Fake.instances[0];
    u.opts.hooks.draw[0](u);
    expect(Fake.ctxCalls).toEqual(
      expect.arrayContaining(["fillRect", "fillText", "stroke", "fill", "arc", "lineTo"]),
    );
  });

  it("does not recreate the chart when only the `series` reference changes", () => {
    const x = [0, 100, 200];
    const values = [1, null, 3];
    const makeSeries = () => [{ label: "v", values: values, color: "--accent" }];
    const { rerender } = render(<UPlotChart {...base({ x, series: makeSeries() })} />);
    rerender(<UPlotChart {...base({ x, series: makeSeries(), cursorX: 100 })} />);
    expect(Fake.instances).toHaveLength(1);
    expect(Fake.instances[0].destroy).not.toHaveBeenCalled();
    // Different data recreates it.
    rerender(
      <UPlotChart {...base({ series: [{ label: "v", values: [1, 2, 3], color: "--accent" }] })} />,
    );
    expect(Fake.instances).toHaveLength(2);
  });

  it("ignores the setCursor fired by the chart creation itself", () => {
    const onCursorX = vi.fn();
    Fake.hookOnCreate = true;
    renderWithTheme(<UPlotChart {...base({ onCursorX })} />);
    Fake.hookOnCreate = false;
    const u = Fake.instances[0];
    // During `new uPlot` the hook runs with the cursor outside; the component must not propagate null.
    expect(onCursorX).not.toHaveBeenCalled();
    u.cursor = { idx: null, left: -1 };
    u.opts.hooks.setCursor[0](u);
    expect(onCursorX).toHaveBeenCalledWith(null);
  });

  it("shows 'Sem leituras' instead of the chart when the signal is missing", () => {
    renderWithTheme(<UPlotChart {...base({ noReadings: true })} />);
    expect(screen.getByText("Sem leituras")).toBeInTheDocument();
    expect(Fake.instances).toHaveLength(0);
  });

  it("does not create a chart without points", () => {
    renderWithTheme(
      <UPlotChart {...base({ x: [], series: [{ label: "v", values: [], color: "--accent" }] })} />,
    );
    expect(Fake.instances).toHaveLength(0);
  });
});
