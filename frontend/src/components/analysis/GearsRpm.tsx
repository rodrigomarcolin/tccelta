"use client";

import { useMemo, useState } from "react";
import type { TripAnalysis, DistributionBin } from "@/domain/trip/types";
import { Indicator } from "@/components/ui/Indicator";
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
    <div className="section-content">
      <div className="indicators-row">
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
      </div>

      <div className="grid-2">
        <section className="card" aria-label="Rotação por velocidade">
          <h3 className="chart-title">Rotação × velocidade</h3>
          {noRpm || scatter.points.length === 0 ? (
            <p className="note">Sem leituras</p>
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
          <p className="note">Pontos de ponto morto ou embreagem ficam fora, sem reta.</p>
        </section>

        <section className="card" aria-label="Tempo em cada marcha">
          <h3 className="chart-title">Tempo em cada marcha</h3>
          {gearBars.length === 0 ? (
            <p className="note">Sem leituras</p>
          ) : (
            <HorizontalBars title="Tempo em cada marcha" items={gearBars} />
          )}
          <p className="note">
            A numeração é relativa ao que foi observado, não às marchas reais do veículo.
          </p>
        </section>

        <section className="card" aria-label="Rotação nas trocas">
          <h3 className="chart-title">Rotação nas trocas</h3>
          {shifts.length === 0 ? (
            <p className="note">Sem trocas de marcha identificadas</p>
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
          <button
            type="button"
            className="button button-link"
            aria-expanded={tableOpen}
            onClick={() => setTableOpen((v) => !v)}
          >
            {tableOpen ? "Ocultar tabela de trocas" : "Ver tabela de trocas"}
          </button>
          {tableOpen ? (
            <table className="table">
              <caption className="sr-only">Trocas de marcha</caption>
              <thead>
                <tr>
                  <th scope="col">Hora</th>
                  <th scope="col">Tipo</th>
                  <th scope="col">Marchas</th>
                  <th scope="col" className="num">
                    Rotação antes
                  </th>
                  <th scope="col" className="num">
                    Rotação depois
                  </th>
                  <th scope="col" className="num">
                    Velocidade
                  </th>
                </tr>
              </thead>
              <tbody>
                {shifts.map((t) => (
                  <tr key={t.t_ms}>
                    <td className="data-value">{formatT(t.t_ms)}</td>
                    <td>{t.direcao === "subida" ? "Subida" : "Redução"}</td>
                    <td className="data-value">{`${t.de}ª → ${t.para}ª`}</td>
                    <td className="num data-value">{formatNumber(t.rpm_antes, 0)}</td>
                    <td className="num data-value">{formatNumber(t.rpm_depois, 0)}</td>
                    <td className="num data-value">{formatNumber(t.velocidade_kmh, 0)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          ) : null}
        </section>

        <section className="card" aria-label="Rotação por pedal">
          <h3 className="chart-title">Rotação × pedal</h3>
          <Heatmap
            timeS={d.rotacao_pedal.tempo_s}
            rpmBin={d.rotacao_pedal.rpm_bin}
            pedalBin={d.rotacao_pedal.pedal_bin}
          />
        </section>

        <section className="card" aria-label="Posição do pedal">
          <h3 className="chart-title">Posição do pedal</h3>
          {d.pedal.length === 0 ? (
            <p className="note">Sem leituras</p>
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
        </section>

        <section className="card" aria-label="Faixas de rotação">
          <h3 className="chart-title">Faixas de rotação</h3>
          {d.rotacao.length === 0 ? (
            <p className="note">Sem leituras</p>
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
        </section>
      </div>
      <p className="note">
        Marcha lenta:{" "}
        {m.marcha_lenta
          ? `${formatNumber(m.marcha_lenta.rpm, 0)} rpm (${m.marcha_lenta.origem})`
          : DASH}
        . Tempo parado: {formatDuration(r.parado_s)}.
      </p>
    </div>
  );
}
