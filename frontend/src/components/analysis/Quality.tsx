"use client";

import { useEffect, useState, type ReactNode } from "react";
import type { TripAnalysis } from "@/domain/trip/types";
import { fuelName } from "@/config";
import { fullTimeFormatter } from "@/domain/trip/analysis";
import { formatDuration, formatNumber, formatPercent, DASH } from "@/format/numbers";
import { Card } from "@/components/ui/Card";
import { DATA_VALUE, DataValue, Unit } from "@/components/ui/DataValue";
import { Note } from "@/components/ui/Note";
import { Overline } from "@/components/ui/Overline";
import { Table, Td, Th } from "@/components/ui/Table";

const ORIGIN: Record<string, string> = {
  estimada: "estimada",
  informada: "informada",
  reserva: "valor de reserva",
};

const DEFINITIONS = "m-0 grid gap-x-5 gap-y-1";

/** Term and value pairs; the compact form has two columns and smaller text. */
function Definitions({ compact, children }: { compact?: boolean; children: ReactNode }) {
  const columns = compact
    ? "grid-cols-[repeat(2,minmax(0,1fr))] text-[12px]"
    : "grid-cols-[minmax(180px,1fr)_minmax(0,2fr)]";
  return (
    <dl className={`${DEFINITIONS} ${columns} phone:grid-cols-[minmax(0,1fr)]`}>{children}</dl>
  );
}

function Term({ mono, children }: { mono?: boolean; children: ReactNode }) {
  return <dt className={mono ? `text-secondary ${DATA_VALUE}` : "text-secondary"}>{children}</dt>;
}

function Def({ children }: { children: ReactNode }) {
  return <dd className={`m-0 ${DATA_VALUE}`}>{children}</dd>;
}

const PLAIN_LIST = "m-0 pl-7 text-secondary";

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
    <Card
      as="details"
      className="print:break-before-page"
      open={open || printing}
      onToggle={(e) => setOpen((e.currentTarget as HTMLDetailsElement).open)}
    >
      <summary className="cursor-pointer type-title">Qualidade dos dados e metodologia</summary>
      <div className="mt-4">
        <Overline>Partes da viagem</Overline>
        {analysis.viagem.partes.length === 0 ? (
          <Note>Nenhuma parte com dados.</Note>
        ) : (
          <Table>
            <caption className="sr-only">Partes da viagem</caption>
            <thead>
              <tr>
                <Th scope="col">Boot</Th>
                <Th scope="col">Período com dados</Th>
                <Th scope="col">Período analisado</Th>
              </tr>
            </thead>
            <tbody>
              {analysis.viagem.partes.map((pt) => (
                <tr key={`${pt.boot_generation}-${pt.boot_id}`}>
                  <Th scope="row">{`boot ${pt.boot_id}`}</Th>
                  <Td mono>{`${fmt(pt.dados_inicio_t_ms)}–${fmt(pt.dados_fim_t_ms)}`}</Td>
                  <Td mono>{`${fmt(pt.inicio_t_ms)}–${fmt(pt.fim_t_ms)}`}</Td>
                </tr>
              ))}
            </tbody>
          </Table>
        )}
        {ignored.map((a, i) => (
          <Note key={i}>
            Boot ignorado por não ter hora absoluta{a.detalhe ? `: ${a.detalhe}` : ""}.
          </Note>
        ))}
        {overlapping ? (
          <Note>Houve partes sobrepostas; a anterior foi cortada no início da seguinte.</Note>
        ) : null}

        <Overline>Leituras</Overline>
        <Definitions>
          <Term>Leituras</Term>
          <Def>{`${formatNumber(q.leituras_total, 0)} (${formatNumber(q.leituras_por_s, 1)} por segundo)`}</Def>
          {Object.entries(q.periodos_mediana_ms).map(([pid, ms]) => (
            <div key={pid} className="contents">
              <Term>{`Período mediano do PID ${pid}`}</Term>
              <Def>{ms === null ? DASH : `${formatNumber(ms, 0)} ms`}</Def>
            </div>
          ))}
        </Definitions>

        <Overline>Razão rotação/velocidade por marcha</Overline>
        {m.patamares.length === 0 ? (
          <Note>Nenhuma marcha identificada.</Note>
        ) : (
          <ul className={PLAIN_LIST}>
            {m.patamares.map((pt) => (
              <li key={pt.marcha}>
                <span>{`${pt.marcha}ª`}</span>{" "}
                <DataValue>{formatNumber(pt.rpm_por_kmh, 1)}</DataValue>
                <Unit>rpm por km/h</Unit>
              </li>
            ))}
          </ul>
        )}

        <Overline>GPS</Overline>
        <Definitions>
          <Term>Cobertura</Term>
          <Def>{formatPercent(q.gps.cobertura_pct, 0)}</Def>
          <Term>Falhas</Term>
          <Def>
            {q.gps.n_falhas === null
              ? DASH
              : `${q.gps.n_falhas} (maior: ${formatDuration(q.gps.maior_falha_s)})`}
          </Def>
          <Term>Distância pelo OBD</Term>
          <Def>{`${formatNumber(analysis.resumo.distancia_km, 2)} km`}</Def>
          <Term>Distância pelo GPS</Term>
          <Def>
            {q.gps.distancia_km === null
              ? DASH
              : `${formatNumber(q.gps.distancia_km, 2)} km (${formatNumber(q.gps.diferenca_distancia_pct, 1)}%)`}
          </Def>
        </Definitions>

        <Overline>Combustível e marcha lenta</Overline>
        <Definitions>
          <Term>Combustível usado na estimativa</Term>
          <Def>{`${fuelName(p.combustivel)} · AFR ${formatNumber(p.afr, 1)} · ${formatNumber(p.densidade_g_l, 0)} g/L`}</Def>
          <Term>Cobertura do sensor de ar (MAF)</Term>
          <Def>{formatPercent(q.combustivel_cobertura_pct, 1)}</Def>
          <Term>Rotação de marcha lenta</Term>
          <Def>
            {m.marcha_lenta
              ? `${formatNumber(m.marcha_lenta.rpm, 0)} rpm (${ORIGIN[m.marcha_lenta.origem] ?? m.marcha_lenta.origem})`
              : DASH}
          </Def>
        </Definitions>

        <Overline>Parâmetros (limiares do projeto)</Overline>
        <Note>
          Valores de projeto, não padrões da literatura. Versão do cálculo:{" "}
          {analysis.versao_calculo}.
        </Note>
        <Definitions compact>
          {thresholds.map(([k, v]) => (
            <div key={k} className="contents">
              <Term mono>{k}</Term>
              <Def>{String(v)}</Def>
            </div>
          ))}
        </Definitions>

        <Overline>Limites do método</Overline>
        <ul className={PLAIN_LIST}>
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
    </Card>
  );
}
