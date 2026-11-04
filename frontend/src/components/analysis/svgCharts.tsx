import type { ReactNode } from "react";
import { formatDuration, formatNumber } from "@/format/numbers";
import { DataValue } from "@/components/ui/DataValue";
import { Note } from "@/components/ui/Note";

/* Custom charts in HTML and SVG: few elements, no library. */

// SVG text and strokes take their look from classes, so they follow the theme variables.
const AXIS_TEXT = "fill-secondary font-mono text-[11px]";
const MONO_11 = "font-mono text-[11px]";
const AXIS_TITLE = "fill-secondary font-ui text-[11px]";

export type BarItem = {
  label: string;
  pct: number | null;
  timeS: number | null;
  /** CSS variable or color; neutral by default. */
  color?: string;
  note?: string;
};

/** Horizontal bars with % and time. */
export function HorizontalBars({ items, title }: { items: BarItem[]; title: string }) {
  const max = Math.max(1, ...items.map((i) => i.pct ?? 0));
  return (
    <ul className="m-0 grid list-none gap-2 p-0" aria-label={title}>
      {items.map((i) => {
        const barColor = i.color
          ? i.color.startsWith("--")
            ? `var(${i.color})`
            : i.color
          : "var(--text-secondary)";
        return (
          <li
            key={i.label}
            className="grid grid-cols-[minmax(90px,1.4fr)_2fr_minmax(110px,1.3fr)] items-center gap-3 text-[13px] phone:grid-cols-[1fr]"
          >
            <span>{i.label}</span>
            <span className="h-2.5 overflow-hidden rounded-pill bg-track" aria-hidden="true">
              <span
                className="block h-full rounded-pill transition-[width] duration-(--dur-value) ease-value"
                style={{ width: `${((i.pct ?? 0) / max) * 100}%`, background: barColor }}
              />
            </span>
            <DataValue className="text-right phone:text-left">
              {i.pct === null ? "—" : `${formatNumber(i.pct, 0)}%`} · {formatDuration(i.timeS)}
            </DataValue>
          </li>
        );
      })}
    </ul>
  );
}

export type ScatterPoint = {
  x: number;
  y: number;
  color: string;
  label?: string;
  onClick?: () => void;
};

export type ScatterLine = {
  x1: number;
  y1: number;
  x2: number;
  y2: number;
  color: string;
  label?: string;
};

type ScatterProps = {
  title: string;
  points: ScatterPoint[];
  lines?: ScatterLine[];
  /** Horizontal dashes (for example, the median of each type). */
  dashes?: { x1: number; x2: number; y: number; label?: string }[];
  domainX: [number, number];
  domainY: [number, number];
  ticksX?: { v: number; label: string }[];
  ticksY?: { v: number; label: string }[];
  labelX: string;
  labelY: string;
  width?: number;
  height?: number;
  radius?: number;
};

const MARGIN = { l: 54, r: 16, t: 10, b: 38 };

function cssColorValue(c: string): string {
  return c.startsWith("--") ? `var(${c})` : c;
}

/** SVG scatter plot, with optional rows and dashes. */
export function Scatter(p: ScatterProps) {
  const L = p.width ?? 520;
  const A = p.height ?? 300;
  const w = L - MARGIN.l - MARGIN.r;
  const h = A - MARGIN.t - MARGIN.b;
  const [x0, x1] = p.domainX;
  const [y0, y1] = p.domainY;
  const px = (v: number) => MARGIN.l + ((v - x0) / (x1 - x0 || 1)) * w;
  const py = (v: number) => MARGIN.t + h - ((v - y0) / (y1 - y0 || 1)) * h;
  const ticksX = p.ticksX ?? [];
  const ticksY = p.ticksY ?? [];
  return (
    <svg className="h-auto w-full" viewBox={`0 0 ${L} ${A}`} role="group" aria-label={p.title}>
      <title>{p.title}</title>
      {ticksY.map((t) => (
        <g key={`y${t.v}`}>
          <line
            x1={MARGIN.l}
            x2={MARGIN.l + w}
            y1={py(t.v)}
            y2={py(t.v)}
            className="stroke-hairline"
          />
          <text x={MARGIN.l - 6} y={py(t.v) + 4} textAnchor="end" className={AXIS_TEXT}>
            {t.label}
          </text>
        </g>
      ))}
      {ticksX.map((t) => (
        <g key={`x${t.v}`}>
          <line
            x1={px(t.v)}
            x2={px(t.v)}
            y1={MARGIN.t}
            y2={MARGIN.t + h}
            className="stroke-hairline"
          />
          <text x={px(t.v)} y={MARGIN.t + h + 14} textAnchor="middle" className={AXIS_TEXT}>
            {t.label}
          </text>
        </g>
      ))}
      <text x={MARGIN.l + w / 2} y={A - 4} textAnchor="middle" className={AXIS_TITLE}>
        {p.labelX}
      </text>
      <text
        x={12}
        y={MARGIN.t + h / 2}
        textAnchor="middle"
        className={AXIS_TITLE}
        transform={`rotate(-90 12 ${MARGIN.t + h / 2})`}
      >
        {p.labelY}
      </text>
      {(p.lines ?? []).map((l, i) => (
        <g key={`l${i}`}>
          <line
            x1={px(l.x1)}
            y1={py(l.y1)}
            x2={px(l.x2)}
            y2={py(l.y2)}
            stroke={cssColorValue(l.color)}
            strokeWidth={1.2}
            strokeDasharray="4 3"
          />
          {l.label ? (
            <text
              x={px(l.x2) - 4}
              y={py(l.y2) - 4}
              textAnchor="end"
              className={AXIS_TEXT}
              fill={cssColorValue(l.color)}
              style={{ fill: cssColorValue(l.color) }}
            >
              {l.label}
            </text>
          ) : null}
        </g>
      ))}
      {(p.dashes ?? []).map((t, i) => (
        <g key={`t${i}`}>
          <line
            x1={px(t.x1)}
            x2={px(t.x2)}
            y1={py(t.y)}
            y2={py(t.y)}
            className="stroke-primary stroke-2"
          />
          {t.label ? (
            <text x={px(t.x2) + 3} y={py(t.y) + 4} className={AXIS_TEXT}>
              {t.label}
            </text>
          ) : null}
        </g>
      ))}
      {p.points.map((pt, i) => {
        const common = {
          cx: px(pt.x),
          cy: py(pt.y),
          r: p.radius ?? 2.6,
          fill: cssColorValue(pt.color),
          opacity: 0.85,
        };
        if (!pt.onClick) return <circle key={i} {...common} />;
        return (
          <circle
            key={i}
            {...common}
            r={5}
            className="cursor-pointer"
            role="button"
            tabIndex={0}
            aria-label={pt.label}
            onClick={pt.onClick}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                pt.onClick?.();
              }
            }}
          >
            <title>{pt.label}</title>
          </circle>
        );
      })}
    </svg>
  );
}

type HeatmapProps = {
  /** Rows = RPM bands; cols = pedal bands. */
  timeS: number[][];
  rpmBin: number;
  pedalBin: number;
};

/** Heatmap of RPM x pedal, in a single hue. */
export function Heatmap({ timeS, rpmBin, pedalBin }: HeatmapProps): ReactNode {
  const rows = timeS.length;
  const cols = Math.max(...timeS.map((l) => l.length), 0);
  const max = Math.max(0, ...timeS.flat());
  if (rows === 0 || max === 0) return <Note>Sem leituras</Note>;
  // Only shows from the first to the last RPM bin that has time.
  const withTimeb = timeS.map((l) => l.some((v) => v > 0));
  const firstRow = withTimeb.indexOf(true);
  const lastRow = withTimeb.lastIndexOf(true);
  const shown = lastRow - firstRow + 1;
  const C = 34;
  const L = 24;
  const ml = 56;
  const mb = 34;
  const width = ml + cols * C + 8;
  const height = shown * L + mb + 6;
  return (
    <figure className="[&_svg]:h-auto [&_svg]:w-full">
      <svg
        viewBox={`0 0 ${width} ${height}`}
        role="img"
        aria-label="Mapa de calor do tempo por rotação e posição do pedal"
      >
        {Array.from({ length: shown }, (_, k) => {
          const i = lastRow - k; // highest RPM on top
          const row = timeS[i] ?? [];
          return (
            <g key={i}>
              <text x={ml - 6} y={k * L + L / 2 + 4} textAnchor="end" className={AXIS_TEXT}>
                {i * rpmBin}
              </text>
              {Array.from({ length: cols }, (_, j) => {
                const v = row[j] ?? 0;
                const alpha = v === 0 ? 0 : 0.12 + 0.88 * (v / max);
                return (
                  <rect
                    key={j}
                    x={ml + j * C}
                    y={k * L}
                    width={C - 2}
                    height={L - 2}
                    rx={3}
                    style={{ fill: `rgba(var(--heat), ${alpha.toFixed(3)})` }}
                  >
                    <title>{`${i * rpmBin}–${(i + 1) * rpmBin} rpm, pedal ${j * pedalBin}–${(j + 1) * pedalBin}%: ${formatDuration(v)}`}</title>
                  </rect>
                );
              })}
            </g>
          );
        })}
        {Array.from({ length: cols }, (_, j) => (
          <text
            key={j}
            x={ml + j * C + (C - 2) / 2}
            y={shown * L + 14}
            textAnchor="middle"
            className={AXIS_TEXT}
          >
            {j * pedalBin}
          </text>
        ))}
        <text x={ml + (cols * C) / 2} y={shown * L + 30} textAnchor="middle" className={AXIS_TITLE}>
          Pedal (%)
        </text>
      </svg>
      <Note as="figcaption">
        Linhas: rotação (rpm, início de cada faixa). Cor: tempo, de 0 s a{" "}
        <DataValue>{formatDuration(max)}</DataValue>.
      </Note>
      <div className={`flex items-center gap-2 text-secondary ${MONO_11}`} aria-hidden="true">
        <span>0 s</span>
        <span className="h-2 flex-1 rounded-pill bg-[linear-gradient(to_right,rgba(var(--heat),0.12),rgba(var(--heat),1))]" />
        <span>{formatDuration(max)}</span>
      </div>
    </figure>
  );
}
