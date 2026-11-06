"use client";

import type { TripEvent } from "@/domain/trip/types";
import { EVENT_LABEL } from "@/domain/trip/analysis";
import { formatDuration, formatNumber } from "@/format/numbers";
import { Button } from "@/components/ui/Button";
import { Note } from "@/components/ui/Note";
import { Table, Td, Th } from "@/components/ui/Table";

type Props = {
  events: TripEvent[];
  formatT: (t: number) => string;
  onViewOnMap: (e: TripEvent) => void;
};

export function peakText(e: TripEvent): string {
  return e.pico_unidade === "s" ? formatDuration(e.pico) : `${formatNumber(e.pico, 1)} m/s²`;
}

export function EventsTable({ events, formatT, onViewOnMap }: Props) {
  if (events.length === 0) return <Note>Nenhum evento nesta viagem.</Note>;
  return (
    <div className="overflow-x-auto">
      <Table>
        <caption className="sr-only">Eventos da viagem</caption>
        <thead>
          <tr>
            <Th scope="col">Tipo</Th>
            <Th scope="col">Hora</Th>
            <Th scope="col" num>
              Duração
            </Th>
            <Th scope="col" num>
              Pico
            </Th>
            <Th scope="col" num>
              Velocidade
            </Th>
            <Th scope="col">Ação</Th>
          </tr>
        </thead>
        <tbody>
          {events.map((e) => {
            const noPosition = e.lat === null || e.lon === null;
            return (
              <tr key={`${e.tipo}-${e.t_inicio_ms}`}>
                <Th scope="row">{EVENT_LABEL[e.tipo]}</Th>
                <Td mono>{formatT(e.t_inicio_ms)}</Td>
                <Td num mono>
                  {formatDuration(e.duracao_s)}
                </Td>
                <Td num mono>
                  {peakText(e)}
                </Td>
                <Td num mono>
                  {formatNumber(e.velocidade_inicial_kmh, 0)} →{" "}
                  {formatNumber(e.velocidade_final_kmh, 0)} km/h
                </Td>
                <Td>
                  <span title={noPosition ? "Sem posição de GPS" : undefined}>
                    <Button
                      variant="secondary"
                      size="small"
                      disabled={noPosition}
                      aria-label={`Ver no mapa: ${EVENT_LABEL[e.tipo]} às ${formatT(e.t_inicio_ms)}`}
                      onClick={() => onViewOnMap(e)}
                    >
                      Ver no mapa
                    </Button>
                  </span>
                </Td>
              </tr>
            );
          })}
        </tbody>
      </Table>
    </div>
  );
}
