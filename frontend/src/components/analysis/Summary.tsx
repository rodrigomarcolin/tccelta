"use client";

import { useState, type ReactNode } from "react";
import type { TripAnalysis } from "@/domain/trip/types";
import { fuelName } from "@/config";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { DataValue } from "@/components/ui/DataValue";
import { Indicator } from "@/components/ui/Indicator";
import { Note } from "@/components/ui/Note";
import { Overline } from "@/components/ui/Overline";
import { Table, Td, Th } from "@/components/ui/Table";
import { formatDuration, formatTime, formatNumber, formatPercent, DASH } from "@/format/numbers";
import { BAND_ORDER, BAND_LABEL } from "@/format/bands";

export function Legend({ analysis }: { analysis: TripAnalysis }) {
  const [showTable, setShowTable] = useState(false);
  const bands = BAND_ORDER.map((id) => analysis.faixas_velocidade.find((f) => f.id === id)).filter(
    (f): f is NonNullable<typeof f> => f !== undefined,
  );
  if (bands.length === 0) return null;
  return (
    <div>
      <ul
        className="m-0 grid list-none grid-cols-[repeat(auto-fit,minmax(120px,1fr))] gap-2 p-0"
        aria-label="Faixas de velocidade"
      >
        {bands.map((f) => (
          <li
            key={f.id}
            className="grid grid-cols-[14px_1fr] grid-rows-[auto_auto] items-center gap-x-2 text-[13px]"
          >
            <span
              className="size-[14px] rounded-[4px] border border-strong"
              style={{ background: `var(--band-${f.id})` }}
              aria-hidden="true"
            />
            <span>{BAND_LABEL[f.id]}</span>
            <DataValue className="col-start-2 text-[15px]">
              {f.id === "parado" ? formatDuration(f.tempo_s) : formatPercent(f.distancia_pct, 0)}
            </DataValue>
          </li>
        ))}
      </ul>
      <Note>% da distância percorrida em movimento. Parado mostra o tempo.</Note>
      <Button variant="link" aria-expanded={showTable} onClick={() => setShowTable((v) => !v)}>
        {showTable ? "Ocultar tabela das faixas" : "Ver tabela das faixas"}
      </Button>
      {showTable ? (
        <Table>
          <caption className="sr-only">Faixas de velocidade</caption>
          <thead>
            <tr>
              <Th scope="col">Faixa</Th>
              <Th scope="col" num>
                % da distância
              </Th>
              <Th scope="col" num>
                Tempo
              </Th>
            </tr>
          </thead>
          <tbody>
            {bands.map((f) => (
              <tr key={f.id}>
                <Th scope="row">{BAND_LABEL[f.id]}</Th>
                <Td num mono>
                  {formatPercent(f.distancia_pct, 1)}
                </Td>
                <Td num mono>
                  {formatDuration(f.tempo_s)}
                </Td>
              </tr>
            ))}
          </tbody>
        </Table>
      ) : null}
    </div>
  );
}

function Group({ title, children }: { title: string; children: ReactNode }) {
  return (
    <Card as="section" aria-label={title}>
      <Overline flush>{title}</Overline>
      <div className="grid grid-cols-[repeat(auto-fill,minmax(150px,1fr))] gap-4">{children}</div>
    </Card>
  );
}

export function Summary({ analysis }: { analysis: TripAnalysis }) {
  const r = analysis.resumo;
  const fuel = fuelName(analysis.parametros.combustivel);
  const clock = analysis.viagem.relogio;
  const timeRange =
    clock && analysis.viagem.inicio_t_ms !== null && analysis.viagem.fim_t_ms !== null
      ? `${formatTime(clock.deslocamento_ms + analysis.viagem.inicio_t_ms)}–${formatTime(clock.deslocamento_ms + analysis.viagem.fim_t_ms)}`
      : null;
  const estimated = `estimado · ${fuel}`;
  const noConsumption = r.combustivel_l === null;
  const idleText =
    r.marcha_lenta_pct === null
      ? DASH
      : `${formatNumber(r.marcha_lenta_pct, 0)}% (${formatDuration(r.marcha_lenta_s)})`;

  return (
    <div className="grid gap-5">
      <Group title="Uso">
        <Indicator label="Distância" value={formatNumber(r.distancia_km, 1)} unit="km" />
        <Indicator label="Duração" value={formatDuration(r.duracao_s)} />
        <Indicator label="Em movimento" value={formatDuration(r.em_movimento_s)} />
        {timeRange ? <Indicator label="Horário" value={timeRange} /> : null}
        {r.sem_dados_s !== null && r.sem_dados_s > 0 ? (
          <Indicator label="Sem dados" value={formatDuration(r.sem_dados_s)} />
        ) : null}
      </Group>
      <Group title={`Eficiência (${estimated})`}>
        <Indicator
          label="Consumo"
          value={formatNumber(r.km_por_l, 1)}
          unit="km/L"
          note={noConsumption ? "Sem leituras" : estimated}
        />
        <Indicator
          label="Combustível"
          value={formatNumber(r.combustivel_l, 2)}
          unit="L"
          note={noConsumption ? "Sem leituras" : estimated}
        />
        <Indicator label="Marcha lenta" value={idleText} />
      </Group>
      <Group title="Condução">
        <Indicator
          label="Velocidade média"
          value={formatNumber(r.velocidade_media_kmh, 0)}
          unit="km/h"
        />
        <Indicator
          label="Velocidade máxima"
          value={formatNumber(r.velocidade_maxima_kmh, 0)}
          unit="km/h"
        />
        <Indicator
          label="Frenagens bruscas"
          value={formatNumber(r.n_fren_bruscas, 0)}
          note={
            r.fren_por_100km === null
              ? undefined
              : `${formatNumber(r.fren_por_100km, 1)} por 100 km`
          }
        />
        <Indicator
          label="Acelerações bruscas"
          value={formatNumber(r.n_acel_bruscas, 0)}
          note={
            r.acel_por_100km === null
              ? undefined
              : `${formatNumber(r.acel_por_100km, 1)} por 100 km`
          }
        />
        <Indicator label="Parado" value={formatDuration(r.parado_s)} />
      </Group>
    </div>
  );
}
