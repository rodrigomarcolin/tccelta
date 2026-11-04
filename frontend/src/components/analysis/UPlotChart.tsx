"use client";

import { useEffect, useRef, useState } from "react";
import uPlot from "uplot";
import { useTheme } from "@/components/ui/Theme";
import { cssColor } from "./colors";

export type ChartSeries = {
  label: string;
  values: (number | null)[];
  /** CSS variable name (`--accent`) or hex color. */
  color: string;
  step?: boolean;
  dashed?: boolean;
};

export type Marker = {
  x: number;
  y: number;
  shape: "tri-down" | "tri-up" | "circle" | "circle-empty";
  label?: string;
};

export type Pause = { x0: number; x1: number; label: string };

export type ChartProps = {
  title: string;
  x: number[];
  series: ChartSeries[];
  height?: number;
  formatX: (x: number) => string;
  formatY?: (v: number) => string;
  ticksY?: number[];
  bandY?: [number, number];
  /** Horizontal reference line (for example, warmed-up engine). */
  referenceY?: { y: number; label: string };
  markers?: Marker[];
  pauses?: Pause[];
  /** Cursor sync key shared between charts. */
  sync?: string;
  /** Cursor X position coming from outside the chart. */
  cursorX?: number | null;
  applyCursor?: boolean;
  onCursorX?: (x: number | null) => void;
  noReadings?: boolean;
};

function resolveColor(color: string): string {
  return color.startsWith("--") ? cssColor(color) : color;
}

function drawMarker(ctx: CanvasRenderingContext2D, m: Marker, px: number, py: number, r: number) {
  const outline = cssColor("--text-primary");
  const fill = cssColor("--surface-card");
  ctx.save();
  ctx.lineWidth = Math.max(1, r / 4);
  ctx.strokeStyle = outline;
  ctx.fillStyle = m.shape === "circle" ? outline : fill;
  ctx.beginPath();
  if (m.shape === "tri-down") {
    ctx.moveTo(px - r, py - r);
    ctx.lineTo(px + r, py - r);
    ctx.lineTo(px, py + r);
  } else if (m.shape === "tri-up") {
    ctx.moveTo(px - r, py + r);
    ctx.lineTo(px + r, py + r);
    ctx.lineTo(px, py - r);
  } else {
    ctx.arc(px, py, r * 0.8, 0, Math.PI * 2);
  }
  ctx.closePath();
  ctx.fill();
  ctx.stroke();
  ctx.restore();
}

function sameSeries(a: ChartSeries[], b: ChartSeries[]): boolean {
  return (
    a.length === b.length &&
    a.every(
      (s, i) =>
        s.values === b[i].values &&
        s.label === b[i].label &&
        s.color === b[i].color &&
        s.step === b[i].step &&
        s.dashed === b[i].dashed,
    )
  );
}

/** Keeps the same reference while the content is unchanged, so the chart is not recreated on every render. */
function useStableSeries(series: ChartSeries[]): ChartSeries[] {
  const [stable, setStable] = useState(series);
  if (!sameSeries(stable, series)) setStable(series);
  return stable;
}

/** Line chart (uPlot) with `null` as a gap, synced cursor and markers. */
export default function UPlotChart(props: ChartProps) {
  const { title, x, height = 150, noReadings } = props;
  const series = useStableSeries(props.series);
  const theme = useTheme();
  const root = useRef<HTMLDivElement>(null);
  const plot = useRef<uPlot | null>(null);
  const applying = useRef(false);
  const creating = useRef(false);
  const cursorCallback = useRef(props.onCursorX);
  useEffect(() => {
    cursorCallback.current = props.onCursorX;
  });

  useEffect(() => {
    const el = root.current;
    if (!el || noReadings || x.length === 0) return;
    const font = '11px "JetBrains Mono", ui-monospace, monospace';
    const secondary = cssColor("--text-secondary");
    const grid = cssColor("--border-hairline");
    const formatY = props.formatY ?? ((v: number) => String(v));

    const opts: uPlot.Options = {
      width: el.clientWidth || 600,
      height,
      legend: { show: false },
      padding: [10, 8, 0, 0],
      scales: {
        x: { time: false },
        y: props.bandY ? { range: () => props.bandY as [number, number] } : {},
      },
      axes: [
        {
          stroke: secondary,
          font: font,
          grid: { stroke: grid, width: 1 },
          ticks: { stroke: grid, width: 1 },
          values: (_u, splits) => splits.map((s) => props.formatX(s)),
        },
        {
          stroke: secondary,
          font: font,
          size: 58,
          grid: { stroke: grid, width: 1 },
          ticks: { stroke: grid, width: 1 },
          ...(props.ticksY ? { splits: () => props.ticksY as number[] } : {}),
          values: (_u, splits) => splits.map((s) => formatY(s)),
        },
      ],
      series: [
        {},
        ...series.map((s) => ({
          label: s.label,
          stroke: resolveColor(s.color),
          width: 1.6,
          spanGaps: false,
          points: { show: false },
          ...(s.dashed ? { dash: [4, 4] } : {}),
          ...(s.step ? { paths: uPlot.paths.stepped?.({ align: 1 }) } : {}),
        })),
      ],
      cursor: {
        drag: { x: false, y: false },
        points: { size: 7, stroke: cssColor("--accent"), fill: cssColor("--surface-card") },
        ...(props.sync ? { sync: { key: props.sync } } : {}),
      },
      hooks: {
        setCursor: [
          (u) => {
            // Creating the chart fires this hook with the cursor outside; that is not the mouse leaving.
            if (applying.current || creating.current) return;
            const i = u.cursor.idx;
            const cb = cursorCallback.current;
            if (!cb) return;
            const left = u.cursor.left ?? -1;
            cb(i === null || i === undefined || left < 0 ? null : (u.data[0][i] as number));
          },
        ],
        draw: [
          (u) => {
            const ctx = u.ctx;
            const dpr = devicePixelRatio || 1;
            const plotArea = u.bbox;
            for (const p of props.pauses ?? []) {
              const a = u.valToPos(p.x0, "x", true);
              const b = u.valToPos(p.x1, "x", true);
              ctx.save();
              ctx.fillStyle = cssColor("--surface-sunken");
              ctx.fillRect(a, plotArea.top, Math.max(1, b - a), plotArea.height);
              ctx.fillStyle = cssColor("--text-secondary");
              ctx.font = `${10 * dpr}px "JetBrains Mono", monospace`;
              ctx.textAlign = "center";
              ctx.fillText(
                p.label,
                (a + b) / 2,
                plotArea.top + 14 * dpr,
                Math.max(60 * dpr, b - a + 40 * dpr),
              );
              ctx.restore();
            }
            if (props.referenceY) {
              const yy = u.valToPos(props.referenceY.y, "y", true);
              ctx.save();
              ctx.strokeStyle = cssColor("--text-tertiary");
              ctx.setLineDash([6 * dpr, 4 * dpr]);
              ctx.beginPath();
              ctx.moveTo(plotArea.left, yy);
              ctx.lineTo(plotArea.left + plotArea.width, yy);
              ctx.stroke();
              ctx.fillStyle = cssColor("--text-secondary");
              ctx.font = `${10 * dpr}px "JetBrains Mono", monospace`;
              ctx.fillText(props.referenceY.label, plotArea.left + 4 * dpr, yy - 4 * dpr);
              ctx.restore();
            }
            for (const m of props.markers ?? []) {
              drawMarker(ctx, m, u.valToPos(m.x, "x", true), u.valToPos(m.y, "y", true), 5 * dpr);
            }
          },
        ],
      },
    };

    creating.current = true;
    const u = new uPlot(opts, [x, ...series.map((s) => s.values)] as uPlot.AlignedData, el);
    creating.current = false;
    plot.current = u;
    const ro = new ResizeObserver(() => u.setSize({ width: el.clientWidth || 600, height }));
    ro.observe(el);
    return () => {
      ro.disconnect();
      u.destroy();
      plot.current = null;
    };
    // The chart is recreated when the data, the theme or the height change.
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [x, series, theme, height, noReadings, props.markers, props.pauses, props.referenceY]);

  useEffect(() => {
    const u = plot.current;
    if (!u || !props.applyCursor) return;
    applying.current = true;
    if (props.cursorX === null || props.cursorX === undefined) {
      u.setCursor({ left: -10, top: -10 });
    } else {
      u.setCursor({ left: Math.round(u.valToPos(props.cursorX, "x")), top: 10 });
    }
    applying.current = false;
  }, [props.cursorX, props.applyCursor]);

  if (noReadings) {
    return (
      <div
        className="grid min-h-10 w-full place-items-center rounded-md bg-list text-secondary"
        role="img"
        aria-label={`${title}: sem leituras`}
        style={{ height }}
      >
        Sem leituras
      </div>
    );
  }
  return (
    <div
      className="min-h-10 w-full [&_.u-over]:font-mono [&_.uplot]:font-mono"
      ref={root}
      role="img"
      aria-label={title}
    />
  );
}
