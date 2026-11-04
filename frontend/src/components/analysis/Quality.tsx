"use client";

import { useEffect, useState } from "react";
import type { TripAnalysis } from "@/domain/trip/types";
import { fuelName } from "@/config";
import { fullTimeFormatter } from "@/domain/trip/analysis";
import { formatDuration, formatNumber, formatPercent, DASH } from "@/format/numbers";

const ORIGIN: Record<string, string> = {
  estimada: "estimada",
  informada: "informada",
  reserva: "valor de reserva",
};

/** Section 7. Collapsed on screen; open when printing. The content is always in the DOM. */
export function Quality({ analysis }: { analysis: TripAnalysis }) {
  const [open, setOpen] = useState(false);
  const [printing, setPrinting] = useState(false);
  useEffect(() => {
    const before = () => setPrinting(true);
    const after = () => setPrinting(false);
    window.addEventListener("beforeprint", before);
    window.addEventListener("afterprint", after);
    return () => {
      window.removeEventListener("beforeprint", before);
      window.removeEventListener("afterprint", after);
    };
  }, []);

  const q = analysis.qualidade;
  const p = analysis.parametros;
  const m = analysis.marchas;
  const fmt = fullTimeFormatter(analysis);
  const ignored = analysis.avisos.filter((a) => a.codigo === "boot_sem_hora");
  const overlapping = analysis.avisos.some((a) => a.codigo === "partes_sobrepostas");
  const thresholds = Object.entries(p).filter(
    ([k]) => !["combustivel", "afr", "densidade_g_l"].includes(k),
  );

  return (
    <details
      className="quality"
      open={open || printing}
      onToggle={(e) => setOpen((e.currentTarget as HTMLDetailsElement).open)}
    >
      <summary>Qualidade dos dados e metodologia</summary>
      <div className="quality-body">
        <h3 className="overline">Partes da viagem</h3>
        {analysis.viagem.partes.length === 0 ? (
          <p className="note">Nenhuma parte com dados.</p>
        ) : (
          <table className="table">
            <caption className="sr-only">Partes da viagem</caption>
            <thead>
              <tr>
                <th scope="col">Boot</th>
                <th scope="col">Período com dados</th>
                <th scope="col">Período analisado</th>
              </tr>
            </thead>
            <tbody>
              {analysis.viagem.partes.map((pt) => (
                <tr key={`${pt.boot_generation}-${pt.boot_id}`}>
                  <th scope="row">{`boot ${pt.boot_id}`}</th>
                  <td className="data-value">{`${fmt(pt.dados_inicio_t_ms)}–${fmt(pt.dados_fim_t_ms)}`}</td>
                  <td className="data-value">{`${fmt(pt.inicio_t_ms)}–${fmt(pt.fim_t_ms)}`}</td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
        {ignored.map((a, i) => (
          <p key={i} className="note">
            Boot ignorado por não ter hora absoluta{a.detalhe ? `: ${a.detalhe}` : ""}.
          </p>
        ))}
        {overlapping ? (
          <p className="note">
            Houve partes sobrepostas; a anterior foi cortada no início da seguinte.
          </p>
        ) : null}

        <h3 className="overline">Leituras</h3>
        <dl className="definitions">
          <dt>Leituras</dt>
          <dd className="data-value">{`${formatNumber(q.leituras_total, 0)} (${formatNumber(q.leituras_por_s, 1)} por segundo)`}</dd>
          {Object.entries(q.periodos_mediana_ms).map(([pid, ms]) => (
            <div key={pid} className="definition-row">
              <dt>{`Período mediano do PID ${pid}`}</dt>
              <dd className="data-value">{ms === null ? DASH : `${formatNumber(ms, 0)} ms`}</dd>
            </div>
          ))}
        </dl>

        <h3 className="overline">Razão rotação/velocidade por marcha</h3>
        {m.patamares.length === 0 ? (
          <p className="note">Nenhuma marcha identificada.</p>
        ) : (
          <ul className="plain-list">
            {m.patamares.map((pt) => (
              <li key={pt.marcha}>
                <span>{`${pt.marcha}ª`}</span>{" "}
                <span className="data-value">{formatNumber(pt.rpm_por_kmh, 1)}</span>
                <span className="unit">rpm por km/h</span>
              </li>
            ))}
          </ul>
        )}

        <h3 className="overline">GPS</h3>
        <dl className="definitions">
          <dt>Cobertura</dt>
          <dd className="data-value">{formatPercent(q.gps.cobertura_pct, 0)}</dd>
          <dt>Falhas</dt>
          <dd className="data-value">
            {q.gps.n_falhas === null
              ? DASH
              : `${q.gps.n_falhas} (maior: ${formatDuration(q.gps.maior_falha_s)})`}
          </dd>
          <dt>Distância pelo OBD</dt>
          <dd className="data-value">{`${formatNumber(analysis.resumo.distancia_km, 2)} km`}</dd>
          <dt>Distância pelo GPS</dt>
          <dd className="data-value">
            {q.gps.distancia_km === null
              ? DASH
              : `${formatNumber(q.gps.distancia_km, 2)} km (${formatNumber(q.gps.diferenca_distancia_pct, 1)}%)`}
          </dd>
        </dl>

        <h3 className="overline">Combustível e marcha lenta</h3>
        <dl className="definitions">
          <dt>Combustível usado na estimativa</dt>
          <dd className="data-value">{`${fuelName(p.combustivel)} · AFR ${formatNumber(p.afr, 1)} · ${formatNumber(p.densidade_g_l, 0)} g/L`}</dd>
          <dt>Cobertura do sensor de ar (MAF)</dt>
          <dd className="data-value">{formatPercent(q.combustivel_cobertura_pct, 1)}</dd>
          <dt>Rotação de marcha lenta</dt>
          <dd className="data-value">
            {m.marcha_lenta
              ? `${formatNumber(m.marcha_lenta.rpm, 0)} rpm (${ORIGIN[m.marcha_lenta.origem] ?? m.marcha_lenta.origem})`
              : DASH}
          </dd>
        </dl>

        <h3 className="overline">Parâmetros (limiares do projeto)</h3>
        <p className="note">
          Valores de projeto, não padrões da literatura. Versão do cálculo:{" "}
          {analysis.versao_calculo}.
        </p>
        <dl className="definitions definitions-compact">
          {thresholds.map(([k, v]) => (
            <div key={k} className="definition-row">
              <dt className="data-value">{k}</dt>
              <dd className="data-value">{String(v)}</dd>
            </div>
          ))}
        </dl>

        <h3 className="overline">Limites do método</h3>
        <ul className="plain-list">
          <li>
            A estimativa de combustível supõe mistura estequiométrica e não considera correções de
            mistura nem corte de combustível.
          </li>
          <li>
            A posição do GPS é marcada na chegada ao dongle: a 130 km/h, 1 s de atraso equivale a
            cerca de 36 m.
          </li>
          <li>A razão rotação/velocidade sozinha não identifica a marcha real do veículo.</li>
          <li>Os limiares de eventos são heurísticas do projeto.</li>
        </ul>
      </div>
    </details>
  );
}
