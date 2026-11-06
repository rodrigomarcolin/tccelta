import type { TripAnalysis } from "@/domain/trip/types";

/**
 * Validates an analysis against the spec 02 contract: parallel arrays of equal
 * length, `null` (not zero) where data is missing, separators between parts and
 * moving bands summing to 100%. Returns the list of violations.
 */
export function contractViolations(a: TripAnalysis): string[] {
  const errors: string[] = [];
  const n = a.series.t_ms.length;

  for (const [key, value] of Object.entries(a.series)) {
    if (Array.isArray(value) && value.length !== n) {
      errors.push(`series.${key} has ${value.length} items, expected ${n}`);
    }
  }

  const slow = a.series_lentas;
  const sameLength = (name: string, arrs: unknown[][]) => {
    const t = arrs[0].length;
    if (arrs.some((x) => x.length !== t)) errors.push(`${name}: arrays of different lengths`);
  };
  sameLength("temperatura", [slow.temperatura.t_ms, slow.temperatura.c]);
  sameLength("nivel_tanque", [
    slow.nivel_tanque.t_ms,
    slow.nivel_tanque.pct,
    slow.nivel_tanque.mediana_movel_pct,
  ]);

  if (a.rota) {
    sameLength("rota", [a.rota.t_ms, a.rota.lat, a.rota.lon]);
    const seg = a.rota.segmentos;
    if (seg.velocidade_kmh.length !== seg.faixa.length) errors.push("rota.segmentos uneven");
    if (seg.faixa.length !== Math.max(0, a.rota.t_ms.length - 1)) {
      errors.push("rota.segmentos must have one item fewer than the points");
    }
    seg.faixa.forEach((f, i) => {
      if ((f === "falha_gps" || f === "sem_velocidade") && seg.velocidade_kmh[i] !== null) {
        errors.push(`segment ${i} without data must have null speed`);
      }
    });
  }

  // Separators between parts: one row of null values per pause.
  const parts = a.viagem.partes.length;
  if (n > 0) {
    const separators = a.series.t_ms.filter(
      (_, i) => a.series.rpm[i] === null && a.series.velocidade_kmh[i] === null,
    );
    if (parts > 1 && separators.length !== parts - 1) {
      errors.push(`expected ${parts - 1} separators, found ${separators.length}`);
    }
  }
  for (const v of a.series.velocidade_kmh) {
    if (v !== null && v < 0) errors.push("negative speed");
  }

  const moving = a.faixas_velocidade.filter((f) => f.id !== "parado");
  if (moving.length) {
    const sum = moving.reduce((s, f) => s + (f.distancia_pct ?? 0), 0);
    if (Math.abs(sum - 100) > 0.5) errors.push(`moving bands sum to ${sum}`);
  }
  const stopped = a.faixas_velocidade.find((f) => f.id === "parado");
  if (stopped && stopped.distancia_pct !== null) errors.push("parado must have null distancia_pct");

  if (a.viagem.relogio === null && (a.viagem.inicio_em !== null || a.viagem.fim_em !== null)) {
    errors.push("without a clock, inicio_em and fim_em must be null");
  }
  if (a.rota === null) {
    for (const x of [...a.paradas, ...a.eventos]) {
      if (x.lat !== null || x.lon !== null) errors.push("without a route, positions must be null");
    }
  }
  if (a.marchas.patamares.length === 0 && a.marchas.trocas.length > 0) {
    errors.push("shifts without plateaus");
  }
  for (const f of a.distribuicoes.rotacao) {
    if (f.pct < 0 || f.pct > 100) errors.push("pct outside 0..100");
  }
  return errors;
}
