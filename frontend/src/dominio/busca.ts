import type { Rota } from "@/contrato/tipos";

/**
 * Torna um vetor de tempos crescente, trocando separadores `null`
 * (entre partes) por um valor logo após o anterior.
 */
export function preencherTempos(ts: readonly (number | null)[]): number[] {
  const saida: number[] = [];
  let anterior = 0;
  for (let i = 0; i < ts.length; i++) {
    const v = ts[i];
    if (v === null || v === undefined) {
      anterior += 1;
      saida.push(anterior);
    } else {
      anterior = v;
      saida.push(v);
    }
  }
  return saida;
}

/** Índice do tempo mais próximo em um vetor crescente. -1 se vazio. */
export function indiceMaisProximo(ts: readonly number[], t: number): number {
  if (ts.length === 0) return -1;
  let lo = 0;
  let hi = ts.length - 1;
  while (lo < hi) {
    const mid = (lo + hi) >> 1;
    if ((ts[mid] as number) < t) lo = mid + 1;
    else hi = mid;
  }
  const anterior = lo > 0 ? lo - 1 : lo;
  return Math.abs((ts[anterior] as number) - t) <= Math.abs((ts[lo] as number) - t) ? anterior : lo;
}

export type PontoRota = { indice: number; t: number; lat: number; lon: number };

const TOLERANCIA_GPS_MS = 5000;

/** Ponto de `rota` mais próximo no tempo; null se não houver GPS naquele instante. */
export function pontoDaRotaNoTempo(rota: Rota | null, t: number): PontoRota | null {
  if (!rota || rota.t_ms.length === 0) return null;
  const tempos = preencherTempos(rota.t_ms);
  const i = indiceMaisProximo(tempos, t);
  const lat = rota.lat[i];
  const lon = rota.lon[i];
  const ti = tempos[i];
  if (i < 0 || lat === null || lat === undefined || lon === null || lon === undefined) return null;
  if (ti === undefined || Math.abs(ti - t) > TOLERANCIA_GPS_MS) return null;
  return { indice: i, t: ti, lat, lon };
}

function metros(lat1: number, lon1: number, lat2: number, lon2: number): number {
  const k = 111_320;
  const dx = (lon2 - lon1) * k * Math.cos(((lat1 + lat2) / 2) * (Math.PI / 180));
  const dy = (lat2 - lat1) * k;
  return Math.hypot(dx, dy);
}

/** Ponto da rota mais próximo de uma posição, dentro de `maxMetros`. */
export function pontoDaRotaNoLugar(
  rota: Rota | null,
  lat: number,
  lon: number,
  maxMetros: number,
): PontoRota | null {
  if (!rota) return null;
  let melhor: PontoRota | null = null;
  let menor = Infinity;
  const tempos = preencherTempos(rota.t_ms);
  for (let i = 0; i < rota.lat.length; i++) {
    const la = rota.lat[i];
    const lo = rota.lon[i];
    if (la === null || la === undefined || lo === null || lo === undefined) continue;
    const d = metros(lat, lon, la, lo);
    if (d < menor) {
      menor = d;
      melhor = { indice: i, t: tempos[i] as number, lat: la, lon: lo };
    }
  }
  return melhor && menor <= maxMetros ? melhor : null;
}
