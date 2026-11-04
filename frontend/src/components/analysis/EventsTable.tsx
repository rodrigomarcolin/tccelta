"use client";

import type { TripEvent } from "@/domain/trip/types";
import { EVENT_LABEL } from "@/domain/trip/analysis";
import { formatDuration, formatNumber } from "@/format/numbers";

type Props = {
  events: TripEvent[];
  formatT: (t: number) => string;
  onViewOnMap: (e: TripEvent) => void;
};

export function peakText(e: TripEvent): string {
  return e.pico_unidade === "s" ? formatDuration(e.pico) : `${formatNumber(e.pico, 1)} m/s²`;
}

export function EventsTable({ events, formatT, onViewOnMap }: Props) {
  if (events.length === 0) return <p className="note">Nenhum evento nesta viagem.</p>;
  return (
    <div className="table-scroll">
      <table className="table events-table">
        <caption className="sr-only">Eventos da viagem</caption>
        <thead>
          <tr>
            <th scope="col">Tipo</th>
            <th scope="col">Hora</th>
            <th scope="col" className="num">
              Duração
            </th>
            <th scope="col" className="num">
              Pico
            </th>
            <th scope="col" className="num">
              Velocidade
            </th>
            <th scope="col">Ação</th>
          </tr>
        </thead>
        <tbody>
          {events.map((e) => {
            const noPosition = e.lat === null || e.lon === null;
            return (
              <tr key={`${e.tipo}-${e.t_inicio_ms}`}>
                <th scope="row">{EVENT_LABEL[e.tipo]}</th>
                <td className="data-value">{formatT(e.t_inicio_ms)}</td>
                <td className="num data-value">{formatDuration(e.duracao_s)}</td>
                <td className="num data-value">{peakText(e)}</td>
                <td className="num data-value">
                  {formatNumber(e.velocidade_inicial_kmh, 0)} →{" "}
                  {formatNumber(e.velocidade_final_kmh, 0)} km/h
                </td>
                <td>
                  <span title={noPosition ? "Sem posição de GPS" : undefined}>
                    <button
                      type="button"
                      className="button button-secondary button-small"
                      disabled={noPosition}
                      aria-label={`Ver no mapa: ${EVENT_LABEL[e.tipo]} às ${formatT(e.t_inicio_ms)}`}
                      onClick={() => onViewOnMap(e)}
                    >
                      Ver no mapa
                    </button>
                  </span>
                </td>
              </tr>
            );
          })}
        </tbody>
      </table>
    </div>
  );
}
