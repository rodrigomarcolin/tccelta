import type { TripAnalysis, TripEvent } from "@/domain/trip/types";
import { fullDate, dayMonth, formatTime, formatFullTime, minutesSeconds } from "@/format/numbers";
import { formatDuration } from "@/format/numbers";
import { nearestIndex, fillTimes } from "./search";
import { createAxis, type Axis } from "./axis";

/** X axis of the timeline: pauses between parts are compressed. */
export function analysisAxis(a: TripAnalysis): Axis {
  return createAxis(a.viagem.partes.map((p) => ({ start: p.inicio_t_ms, end: p.fim_t_ms })));
}

/**
 * X axis formatter. With `relogio` it shows the absolute time
 * (`deslocamento_ms + t_ms`); otherwise the time since `inicio_t_ms` as `m:ss`.
 */
export function timeFormatter(a: TripAnalysis): (t: number) => string {
  const clock = a.viagem.relogio;
  const start = a.viagem.inicio_t_ms ?? 0;
  if (clock) return (t) => formatTime(clock.deslocamento_ms + t);
  return (t) => minutesSeconds(t - start);
}

export function fullTimeFormatter(a: TripAnalysis): (t: number) => string {
  const clock = a.viagem.relogio;
  const start = a.viagem.inicio_t_ms ?? 0;
  if (clock) return (t) => formatFullTime(clock.deslocamento_ms + t);
  return (t) => minutesSeconds(t - start);
}

export type Instant = {
  t: number;
  speed: number | null;
  rpm: number | null;
  gear: number | null;
  pedal: number | null;
};

/** Readings at the grid point nearest in time. */
export function nearestInstant(a: TripAnalysis, tempos: number[], t: number): Instant | null {
  const i = nearestIndex(tempos, t);
  if (i < 0) return null;
  const s = a.series;
  return {
    t: tempos[i],
    speed: s.velocidade_kmh[i] ?? null,
    rpm: s.rpm[i] ?? null,
    gear: s.marcha[i] ?? null,
    pedal: s.pedal_pct[i] ?? null,
  };
}

export function seriesTimes(a: TripAnalysis): number[] {
  return fillTimes(a.series.t_ms);
}

export const EVENT_LABEL: Record<TripEvent["tipo"], string> = {
  frenagem_brusca: "Frenagem brusca",
  aceleracao_brusca: "Aceleração brusca",
  marcha_lenta_longa: "Marcha lenta longa",
};

export function gearLabel(gear: number | null): string {
  if (gear === null) return "—";
  if (gear === 0) return "Ponto morto ou embreagem";
  return `${gear}ª`;
}

export function pauseText(ms: number): string {
  return `Sem dados · ${formatDuration(ms / 1000)}`;
}

/** Are there readings? False for an empty trip. */
export function hasReadings(a: TripAnalysis): boolean {
  return a.series.t_ms.length > 0;
}

export function hasAnyValue(xs: readonly (number | null)[]): boolean {
  return xs.some((v) => v !== null && v !== undefined);
}

/**
 * Header (spec 01): identification according to the input and, for a boot with
 * absolute time, the date and time. Without absolute time, omits date and time.
 */
export function headerDescription(a: TripAnalysis): {
  identification: string;
  when: string | null;
} {
  const e = a.viagem.entrada;
  if (e.tipo === "boot") {
    const { inicio_em: startIso, fim_em: endIso } = a.viagem;
    const when =
      startIso && endIso
        ? `${fullDate(Date.parse(startIso))} ${formatTime(Date.parse(startIso))}–${formatTime(Date.parse(endIso))}`
        : null;
    return { identification: `Dongle ${e.dongle_id} · boot ${e.boot_id}`, when };
  }
  const t0 = Date.parse(e.t0);
  const t1 = Date.parse(e.t1);
  const sameDay = dayMonth(t0) === dayMonth(t1);
  const period = sameDay
    ? `${dayMonth(t0)} ${formatTime(t0)}–${formatTime(t1)}`
    : `${dayMonth(t0)} ${formatTime(t0)}–${dayMonth(t1)} ${formatTime(t1)}`;
  const partsText = a.viagem.partes.length > 1 ? ` · ${a.viagem.partes.length} partes` : "";
  return { identification: `Dongle ${e.dongle_id} · ${period}${partsText}`, when: null };
}
