"use client";

import { useMemo } from "react";
import type { TripAnalysis } from "@/domain/trip/types";
import type { Axis } from "@/domain/trip/axis";
import {
  hasAnyValue,
  fullTimeFormatter,
  nearestInstant,
  seriesTimes,
  pauseText,
} from "@/domain/trip/analysis";
import { formatNumber } from "@/format/numbers";
import { Card } from "@/components/ui/Card";
import { ChartTitle } from "@/components/ui/ChartTitle";
import { Unit } from "@/components/ui/DataValue";
import { Chart } from "./Chart";
import type { Marker, Pause } from "./UPlotChart";
import { InstantTooltip } from "./Instant";

export type Cursor = { t: number | null; source: "chart" | "map" | "pinned" };

type Props = {
  analysis: TripAnalysis;
  axis: Axis;
  formatX: (x: number) => string;
  cursor: Cursor;
  onCursor: (t: number | null, source: Cursor["source"]) => void;
};

const SYNC = "timeline";
const MARKERS = "m-0 mt-2 text-[12px] text-secondary";

function labelY(v: number): string {
  return formatNumber(v, 0);
}

export function Timeline({ analysis, axis, formatX, cursor, onCursor }: Props) {
  const s = analysis.series;
  const times = useMemo(() => seriesTimes(analysis), [analysis]);
  const x = useMemo(() => times.map((t) => axis.toX(t)), [times, axis]);
  const pauses: Pause[] = useMemo(
    () => axis.pauses.map((p) => ({ x0: p.x0, x1: p.x1, label: pauseText(p.durationMs) })),
    [axis],
  );
  const formatT = useMemo(() => fullTimeFormatter(analysis), [analysis]);

  const speedMarkers: Marker[] = useMemo(
    () =>
      analysis.eventos
        .filter((e) => e.tipo !== "marcha_lenta_longa")
        .map((e) => ({
          x: axis.toX(e.t_inicio_ms),
          y: e.velocidade_inicial_kmh ?? 0,
          shape: e.tipo === "frenagem_brusca" ? ("tri-down" as const) : ("tri-up" as const),
        })),
    [analysis.eventos, axis],
  );
  const rpmMarkers: Marker[] = useMemo(
    () =>
      analysis.marchas.trocas.map((t) => ({
        x: axis.toX(t.t_ms),
        y: t.rpm_antes,
        shape: t.direcao === "subida" ? ("circle" as const) : ("circle-empty" as const),
      })),
    [analysis.marchas.trocas, axis],
  );

  const maxGear = Math.max(0, ...s.marcha.map((m) => m ?? 0));
  const gearTicks = Array.from({ length: maxGear + 1 }, (_, i) => i);

  const instant = cursor.t === null ? null : nearestInstant(analysis, times, cursor.t);
  const cursorX = cursor.t === null ? null : axis.toX(cursor.t);
  const apply = cursor.source !== "chart";
  const onMove = (xv: number | null) => onCursor(xv === null ? null : axis.fromX(xv), "chart");

  const common = {
    x,
    formatX,
    pauses,
    sync: SYNC,
    cursorX,
    applyCursor: apply,
    onCursorX: onMove,
  };

  return (
    <div>
      <InstantTooltip instant={instant} formatT={formatT} />
      <Card avoidBreak className="mb-3">
        <ChartTitle>
          Velocidade <Unit>km/h</Unit>
        </ChartTitle>
        <Chart
          {...common}
          title="Velocidade ao longo da viagem"
          series={[{ label: "Velocidade", values: s.velocidade_kmh, color: "--text-primary" }]}
          markers={speedMarkers}
          formatY={labelY}
          noReadings={!hasAnyValue(s.velocidade_kmh)}
        />
        <p className={MARKERS}>
          <span aria-hidden="true">▼</span> Frenagem brusca · <span aria-hidden="true">▲</span>{" "}
          Aceleração brusca
        </p>
      </Card>
      <Card avoidBreak className="mb-3">
        <ChartTitle>
          Rotação <Unit>rpm</Unit>
        </ChartTitle>
        <Chart
          {...common}
          title="Rotação do motor ao longo da viagem"
          series={[{ label: "Rotação", values: s.rpm, color: "--accent" }]}
          markers={rpmMarkers}
          formatY={labelY}
          noReadings={!hasAnyValue(s.rpm)}
        />
        <p className={MARKERS}>
          <span aria-hidden="true">●</span> Subida de marcha · <span aria-hidden="true">○</span>{" "}
          Redução de marcha
        </p>
      </Card>
      <Card avoidBreak className="mb-3">
        <ChartTitle>Marcha estimada</ChartTitle>
        <Chart
          {...common}
          title="Marcha estimada ao longo da viagem"
          series={[{ label: "Marcha", values: s.marcha, color: "--text-secondary", step: true }]}
          height={120}
          ticksY={gearTicks}
          bandY={[-0.5, maxGear + 0.5]}
          formatY={(v) => (v === 0 ? "N/E" : `${v}ª`)}
          noReadings={!hasAnyValue(s.marcha)}
        />
        <p className={MARKERS}>N/E: ponto morto ou embreagem. Marcha indefinida fica vazia.</p>
      </Card>
      <Card avoidBreak className="mb-3">
        <ChartTitle>
          Pedal <Unit>%</Unit>
        </ChartTitle>
        <Chart
          {...common}
          title="Posição do pedal ao longo da viagem"
          series={[{ label: "Pedal", values: s.pedal_pct, color: "--text-secondary" }]}
          height={120}
          bandY={[0, 100]}
          formatY={labelY}
          noReadings={!hasAnyValue(s.pedal_pct)}
        />
      </Card>
    </div>
  );
}
