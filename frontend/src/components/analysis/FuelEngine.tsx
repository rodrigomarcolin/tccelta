"use client";

import { useMemo } from "react";
import type { TripAnalysis } from "@/domain/trip/types";
import type { Axis } from "@/domain/trip/axis";
import { hasAnyValue, seriesTimes, pauseText } from "@/domain/trip/analysis";
import { fillTimes } from "@/domain/trip/search";
import { fuelName } from "@/config";
import { Indicator } from "@/components/ui/Indicator";
import { formatDuration, formatNumber } from "@/format/numbers";
import { Chart } from "./Chart";
import type { Pause } from "./UPlotChart";

type Props = { analysis: TripAnalysis; axis: Axis; formatX: (x: number) => string };

export function FuelEngine({ analysis, axis, formatX }: Props) {
  const { series: s, series_lentas: l, motor: m, parametros: p } = analysis;
  const times = useMemo(() => seriesTimes(analysis), [analysis]);
  const x = useMemo(() => times.map((t) => axis.toX(t)), [times, axis]);
  const xTemp = useMemo(
    () => fillTimes(l.temperatura.t_ms).map((t) => axis.toX(t)),
    [l.temperatura.t_ms, axis],
  );
  const xTank = useMemo(
    () => fillTimes(l.nivel_tanque.t_ms).map((t) => axis.toX(t)),
    [l.nivel_tanque.t_ms, axis],
  );
  const pauses: Pause[] = useMemo(
    () => axis.pauses.map((q) => ({ x0: q.x0, x1: q.x1, label: pauseText(q.durationMs) })),
    [axis],
  );
  const fuel = fuelName(p.combustivel);
  const warmedUp = typeof p.aquecido_c === "number" ? p.aquecido_c : 80;

  const base = { formatX, pauses, height: 140 };
  return (
    <div className="section-content">
      <div className="indicators-row">
        <Indicator label="Tempo até aquecer" value={formatDuration(m.aquecimento_s)} />
        <Indicator
          label="Temperatura máxima"
          value={formatNumber(m.temperatura_maxima_c, 0)}
          unit="°C"
        />
        <Indicator label="Esforço com motor frio" value={formatDuration(m.esforco_frio_s)} />
      </div>
      <div className="grid-2">
        <section className="card" aria-label="Consumo instantâneo">
          <h3 className="chart-title">
            Consumo instantâneo <span className="unit">L/h · estimado · {fuel}</span>
          </h3>
          <Chart
            {...base}
            x={x}
            title="Consumo instantâneo estimado"
            series={[{ label: "Consumo", values: s.consumo_l_h, color: "--accent" }]}
            formatY={(v) => formatNumber(v, 1)}
            noReadings={!hasAnyValue(s.consumo_l_h)}
          />
        </section>
        <section className="card" aria-label="Combustível acumulado">
          <h3 className="chart-title">
            Combustível acumulado <span className="unit">L · estimado · {fuel}</span>
          </h3>
          <Chart
            {...base}
            x={x}
            title="Combustível acumulado estimado"
            series={[{ label: "Acumulado", values: s.combustivel_acumulado_l, color: "--accent" }]}
            formatY={(v) => formatNumber(v, 2)}
            noReadings={!hasAnyValue(s.combustivel_acumulado_l)}
          />
        </section>
        <section className="card" aria-label="Carga do motor">
          <h3 className="chart-title">
            Carga do motor <span className="unit">%</span>
          </h3>
          <Chart
            {...base}
            x={x}
            title="Carga do motor"
            series={[{ label: "Carga", values: s.carga_pct, color: "--text-secondary" }]}
            bandY={[0, 100]}
            formatY={(v) => formatNumber(v, 0)}
            noReadings={!hasAnyValue(s.carga_pct)}
          />
        </section>
        <section className="card" aria-label="Temperatura do líquido">
          <h3 className="chart-title">
            Temperatura do líquido <span className="unit">°C</span>
          </h3>
          <Chart
            {...base}
            x={xTemp}
            title="Temperatura do líquido de arrefecimento"
            series={[{ label: "Temperatura", values: l.temperatura.c, color: "--text-primary" }]}
            referenceY={{ y: warmedUp, label: `Motor aquecido (${warmedUp} °C)` }}
            formatY={(v) => formatNumber(v, 0)}
            noReadings={!hasAnyValue(l.temperatura.c)}
          />
        </section>
        <section className="card" aria-label="Nível do tanque">
          <h3 className="chart-title">
            Nível do tanque <span className="unit">% · leitura e mediana móvel</span>
          </h3>
          <Chart
            {...base}
            x={xTank}
            title="Nível do tanque"
            series={[
              { label: "Leitura", values: l.nivel_tanque.pct, color: "--text-tertiary" },
              {
                label: "Mediana móvel",
                values: l.nivel_tanque.mediana_movel_pct,
                color: "--accent",
              },
            ]}
            bandY={[0, 100]}
            formatY={(v) => formatNumber(v, 0)}
            noReadings={!hasAnyValue(l.nivel_tanque.pct)}
          />
        </section>
      </div>
    </div>
  );
}
