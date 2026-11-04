"use client";

import { useMemo, useState } from "react";
import type { TripAnalysis, DistributionBin } from "@/domain/trip/types";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { ChartTitle } from "@/components/ui/ChartTitle";
import { Indicator, IndicatorRow } from "@/components/ui/Indicator";
import { Note } from "@/components/ui/Note";
import { Table, Td, Th } from "@/components/ui/Table";
import { TwoColumns } from "@/components/ui/TwoColumns";
import { hasAnyValue, gearLabel } from "@/domain/trip/analysis";
import { formatDuration, formatNumber, DASH } from "@/format/numbers";
import { HorizontalBars, Scatter, Heatmap, type BarItem } from "./svgCharts";
import { gearVar } from "./colors";

type Props = {
  analysis: TripAnalysis;
  formatT: (t: number) => string;
  onMarkInstant: (t: number) => void;
};

function rpmBinLabel(f: DistributionBin): string {
  return f.ate === null
    ? `≥ ${formatNumber(f.de, 0)} rpm`
    : `${formatNumber(f.de, 0)}–${formatNumber(f.ate, 0)} rpm`;
}

function pedalBinLabel(f: DistributionBin): string {
  const bin = `${f.de}–${f.ate}%`;
  return f.rotulo ? `${f.rotulo} (${bin})` : bin;
}

function roundUp(v: number, step: number): number {
  return Math.max(step, Math.ceil(v / step) * step);
}

export function GearsRpm({ analysis, formatT, onMarkInstant }: Props) {
  const { series: s, marchas: m, condutor: c, distribuicoes: d, resumo: r } = analysis;
  const [tableOpen, setTableOpen] = useState(false);
  const noRpm = !hasAnyValue(s.rpm);

  const scatter = useMemo(() => {
    const points: { x: number; y: number; color: string }[] = [];
    const step = Math.max(1, Math.ceil(s.t_ms.length / 1200));
    let vMax = 20;
    let rMax = 1000;
    for (let i = 0; i < s.t_ms.length; i += step) {
      const v = s.velocidade_kmh[i];
      const rpm = s.rpm[i];
      const g = s.marcha[i];
      if (
        v === null ||
        rpm === null ||
        g === null ||
        g === undefined ||
        v === undefined ||
        rpm === undefined ||
        v < 8
      )
        continue;
      points.push({ x: v, y: rpm, color: gearVar(g) });
      vMax = Math.max(vMax, v);
      rMax = Math.max(rMax, rpm);
    }
    return { points, vMax: roundUp(vMax, 20), rMax: roundUp(rMax, 500) };
  }, [s]);

  const shifts = m.trocas;
  const upshifts = shifts.filter((t) => t.direcao === "subida");
  const downshifts = shifts.filter((t) => t.direcao === "reducao");
  const rMaxShifts = roundUp(Math.max(1000, ...shifts.map((t) => t.rpm_antes)), 500);

  const shiftPoints = (list: typeof shifts, center: number) =>
    list.map((t, i) => ({
      x: center + (((i * 37) % 21) - 10) / 40,
      y: t.rpm_antes,
      color: "--accent",
      label: `${t.direcao === "subida" ? "Subida" : "Redução"} de ${t.de}ª para ${t.para}ª às ${formatT(t.t_ms)}: ${formatNumber(t.rpm_antes, 0)} rpm. Marcar na linha do tempo`,
      onClick: () => onMarkInstant(t.t_ms),
    }));

  const gearBars: BarItem[] = [
    ...m.patamares.map((p) => ({
      label: gearLabel(p.marcha),
      pct: p.tempo_pct,
      timeS: p.tempo_s,
      color: gearVar(p.marcha),
    })),
    ...(m.ponto_morto_embreagem_s !== null && m.ponto_morto_embreagem_s > 0
      ? [
          {
            label: "Ponto morto ou embreagem",
            pct: r.em_movimento_s ? (m.ponto_morto_embreagem_s / r.em_movimento_s) * 100 : null,
            timeS: m.ponto_morto_embreagem_s,
            color: "--gear-0",
          },
        ]
      : []),
  ];

  return (
    <div>
      <IndicatorRow>
        <Indicator
          label="Rotação mediana ao subir"
          value={formatNumber(m.subida_rpm.mediana, 0)}
          unit="rpm"
          note={m.subida_rpm.n ? `${m.subida_rpm.n} trocas` : undefined}
        />
        <Indicator
          label="Rotação mediana ao reduzir"
          value={formatNumber(m.reducao_rpm.mediana, 0)}
          unit="rpm"
          note={m.reducao_rpm.n ? `${m.reducao_rpm.n} trocas` : undefined}
        />
        <Indicator
          label="Pedal médio em movimento"
          value={formatNumber(c.pedal_medio_pct, 0)}
          unit="%"
        />
        <Indicator label="Freio-motor" value={formatNumber(c.freio_motor_pct, 0)} unit="%" />
        <Indicator
          label="Variação do pedal (p95)"
          value={formatNumber(c.pedal_variacao_p95_pct_s, 0)}
          unit="%/s"
        />
      </IndicatorRow>

      <TwoColumns>
        <Card as="section" avoidBreak aria-label="Rotação por velocidade">
          <ChartTitle>Rotação × velocidade</ChartTitle>
          {noRpm || scatter.points.length === 0 ? (
            <Note>Sem leituras</Note>
          ) : (
            <Scatter
              title="Rotação por velocidade, colorida por marcha"
              points={scatter.points}
              lines={m.patamares
                .filter((p) => p.marcha > 0)
                .map((p) => ({
                  x1: 8,
                  y1: p.rpm_por_kmh * 8,
                  x2: scatter.vMax,
                  y2: Math.min(scatter.rMax, p.rpm_por_kmh * scatter.vMax),
                  color: gearVar(p.marcha),
                  label: `${p.marcha}ª`,
                }))}
              domainX={[0, scatter.vMax]}
              domainY={[0, scatter.rMax]}
              ticksX={Array.from({ length: scatter.vMax / 20 + 1 }, (_, i) => ({
                v: i * 20,
                label: String(i * 20),
              }))}
              ticksY={Array.from({ length: scatter.rMax / 1000 + 1 }, (_, i) => ({
                v: i * 1000,
                label: formatNumber(i * 1000, 0),
              }))}
              labelX="Velocidade (km/h)"
              labelY="Rotação (rpm)"
            />
          )}
          <Note>Pontos de ponto morto ou embreagem ficam fora, sem reta.</Note>
        </Card>

        <Card as="section" avoidBreak aria-label="Tempo em cada marcha">
          <ChartTitle>Tempo em cada marcha</ChartTitle>
          {gearBars.length === 0 ? (
            <Note>Sem leituras</Note>
          ) : (
            <HorizontalBars title="Tempo em cada marcha" items={gearBars} />
          )}
          <Note>A numeração é relativa ao que foi observado, não às marchas reais do veículo.</Note>
        </Card>

        <Card as="section" avoidBreak aria-label="Rotação nas trocas">
          <ChartTitle>Rotação nas trocas</ChartTitle>
          {shifts.length === 0 ? (
            <Note>Sem trocas de marcha identificadas</Note>
          ) : (
            <Scatter
              title="Rotação antes de cada troca, separada em subidas e reduções"
              points={[...shiftPoints(upshifts, 1), ...shiftPoints(downshifts, 3)]}
              dashes={[
                ...(m.subida_rpm.mediana !== null
                  ? [{ x1: 0.6, x2: 1.4, y: m.subida_rpm.mediana }]
                  : []),
                ...(m.reducao_rpm.mediana !== null
                  ? [{ x1: 2.6, x2: 3.4, y: m.reducao_rpm.mediana }]
                  : []),
              ]}
              domainX={[0, 4]}
              domainY={[0, rMaxShifts]}
              ticksX={[
                { v: 1, label: "Subidas" },
                { v: 3, label: "Reduções" },
              ]}
              ticksY={Array.from({ length: rMaxShifts / 1000 + 1 }, (_, i) => ({
                v: i * 1000,
                label: formatNumber(i * 1000, 0),
              }))}
              labelX="Tipo de troca (traço: mediana)"
              labelY="Rotação antes (rpm)"
              height={260}
            />
          )}
          <Button variant="link" aria-expanded={tableOpen} onClick={() => setTableOpen((v) => !v)}>
            {tableOpen ? "Ocultar tabela de trocas" : "Ver tabela de trocas"}
          </Button>
          {tableOpen ? (
            <Table>
              <caption className="sr-only">Trocas de marcha</caption>
              <thead>
                <tr>
                  <Th scope="col">Hora</Th>
                  <Th scope="col">Tipo</Th>
                  <Th scope="col">Marchas</Th>
                  <Th scope="col" num>
                    Rotação antes
                  </Th>
                  <Th scope="col" num>
                    Rotação depois
                  </Th>
                  <Th scope="col" num>
                    Velocidade
                  </Th>
                </tr>
              </thead>
              <tbody>
                {shifts.map((t) => (
                  <tr key={t.t_ms}>
                    <Td mono>{formatT(t.t_ms)}</Td>
                    <Td>{t.direcao === "subida" ? "Subida" : "Redução"}</Td>
                    <Td mono>{`${t.de}ª → ${t.para}ª`}</Td>
                    <Td num mono>
                      {formatNumber(t.rpm_antes, 0)}
                    </Td>
                    <Td num mono>
                      {formatNumber(t.rpm_depois, 0)}
                    </Td>
                    <Td num mono>
                      {formatNumber(t.velocidade_kmh, 0)}
                    </Td>
                  </tr>
                ))}
              </tbody>
            </Table>
          ) : null}
        </Card>

        <Card as="section" avoidBreak aria-label="Rotação por pedal">
          <ChartTitle>Rotação × pedal</ChartTitle>
          <Heatmap
            timeS={d.rotacao_pedal.tempo_s}
            rpmBin={d.rotacao_pedal.rpm_bin}
            pedalBin={d.rotacao_pedal.pedal_bin}
          />
        </Card>

        <Card as="section" avoidBreak aria-label="Posição do pedal">
          <ChartTitle>Posição do pedal</ChartTitle>
          {d.pedal.length === 0 ? (
            <Note>Sem leituras</Note>
          ) : (
            <HorizontalBars
              title="Tempo por posição do pedal"
              items={d.pedal.map((f) => ({
                label: pedalBinLabel(f),
                pct: f.pct,
                timeS: f.tempo_s,
              }))}
            />
          )}
        </Card>

        <Card as="section" avoidBreak aria-label="Faixas de rotação">
          <ChartTitle>Faixas de rotação</ChartTitle>
          {d.rotacao.length === 0 ? (
            <Note>Sem leituras</Note>
          ) : (
            <HorizontalBars
              title="Tempo por faixa de rotação"
              items={d.rotacao.map((f) => ({
                label: rpmBinLabel(f),
                pct: f.pct,
                timeS: f.tempo_s,
              }))}
            />
          )}
        </Card>
      </TwoColumns>
      <Note>
        Marcha lenta:{" "}
        {m.marcha_lenta
          ? `${formatNumber(m.marcha_lenta.rpm, 0)} rpm (${m.marcha_lenta.origem})`
          : DASH}
        . Tempo parado: {formatDuration(r.parado_s)}.
      </Note>
    </div>
  );
}
