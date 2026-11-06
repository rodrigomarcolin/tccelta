"use client";

import { useMemo } from "react";
import type { TripAnalysis } from "@/domain/trip/types";
import type { Axis } from "@/domain/trip/axis";
import { hasAnyValue, seriesTimes, pauseText } from "@/domain/trip/analysis";
import { fillTimes } from "@/domain/trip/search";
import { fuelName } from "@/config";
import { Indicator, IndicatorRow } from "@/components/ui/Indicator";
import { Card } from "@/components/ui/Card";
import { ChartTitle } from "@/components/ui/ChartTitle";
import { TwoColumns } from "@/components/ui/TwoColumns";
import { Unit } from "@/components/ui/DataValue";
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
    <div>
      <IndicatorRow>
        <Indicator label="Tempo até aquecer" value={formatDuration(m.aquecimento_s)} />
        <Indicator
          label="Temperatura máxima"
          value={formatNumber(m.temperatura_maxima_c, 0)}
          unit="°C"
        />
        <Indicator label="Esforço com motor frio" value={formatDuration(m.esforco_frio_s)} />
      </IndicatorRow>
      <TwoColumns>
        <Card as="section" avoidBreak aria-label="Consumo instantâneo">
          <ChartTitle>
            Consumo instantâneo <Unit>L/h · estimado · {fuel}</Unit>
          </ChartTitle>
          <Chart
            {...base}
            x={x}
            title="Consumo instantâneo estimado"
            series={[{ label: "Consumo", values: s.consumo_l_h, color: "--accent" }]}
            formatY={(v) => formatNumber(v, 1)}
            noReadings={!hasAnyValue(s.consumo_l_h)}
          />
        </Card>
        <Card as="section" avoidBreak aria-label="Combustível acumulado">
          <ChartTitle>
            Combustível acumulado <Unit>L · estimado · {fuel}</Unit>
          </ChartTitle>
          <Chart
            {...base}
            x={x}
            title="Combustível acumulado estimado"
            series={[{ label: "Acumulado", values: s.combustivel_acumulado_l, color: "--accent" }]}
            formatY={(v) => formatNumber(v, 2)}
            noReadings={!hasAnyValue(s.combustivel_acumulado_l)}
          />
        </Card>
        <Card as="section" avoidBreak aria-label="Carga do motor">
          <ChartTitle>
            Carga do motor <Unit>%</Unit>
          </ChartTitle>
          <Chart
            {...base}
            x={x}
            title="Carga do motor"
            series={[{ label: "Carga", values: s.carga_pct, color: "--text-secondary" }]}
            bandY={[0, 100]}
            formatY={(v) => formatNumber(v, 0)}
            noReadings={!hasAnyValue(s.carga_pct)}
          />
        </Card>
        <Card as="section" avoidBreak aria-label="Temperatura do líquido">
          <ChartTitle>
            Temperatura do líquido <Unit>°C</Unit>
          </ChartTitle>
          <Chart
            {...base}
            x={xTemp}
            title="Temperatura do líquido de arrefecimento"
            series={[{ label: "Temperatura", values: l.temperatura.c, color: "--text-primary" }]}
            referenceY={{ y: warmedUp, label: `Motor aquecido (${warmedUp} °C)` }}
            formatY={(v) => formatNumber(v, 0)}
            noReadings={!hasAnyValue(l.temperatura.c)}
          />
        </Card>
        <Card as="section" avoidBreak aria-label="Nível do tanque">
          <ChartTitle>
            Nível do tanque <Unit>% · leitura e mediana móvel</Unit>
          </ChartTitle>
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
        </Card>
      </TwoColumns>
    </div>
  );
}
