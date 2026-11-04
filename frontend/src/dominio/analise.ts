import type { Analise, Evento } from "@/contrato/tipos";
import { dataCompleta, diaMes, hora, horaCompleta, minutosSegundos } from "@/formato/numeros";
import { duracao } from "@/formato/numeros";
import { indiceMaisProximo, preencherTempos } from "./busca";
import { criarEixo, type Eixo } from "./eixo";

/** Eixo X da linha do tempo: pausas entre partes comprimidas. */
export function eixoDaAnalise(a: Analise): Eixo {
  return criarEixo(a.viagem.partes.map((p) => ({ inicio: p.inicio_t_ms, fim: p.fim_t_ms })));
}

/**
 * Formatador do eixo X. Com `relogio` mostra a hora absoluta
 * (`deslocamento_ms + t_ms`); senão, o tempo desde `inicio_t_ms` em `m:ss`.
 */
export function formatadorDeTempo(a: Analise): (t: number) => string {
  const relogio = a.viagem.relogio;
  const inicio = a.viagem.inicio_t_ms ?? 0;
  if (relogio) return (t) => hora(relogio.deslocamento_ms + t);
  return (t) => minutosSegundos(t - inicio);
}

export function formatadorDeTempoCompleto(a: Analise): (t: number) => string {
  const relogio = a.viagem.relogio;
  const inicio = a.viagem.inicio_t_ms ?? 0;
  if (relogio) return (t) => horaCompleta(relogio.deslocamento_ms + t);
  return (t) => minutosSegundos(t - inicio);
}

export type Instante = {
  t: number;
  velocidade: number | null;
  rpm: number | null;
  marcha: number | null;
  pedal: number | null;
};

/** Leituras do ponto da grade mais próximo no tempo. */
export function instanteMaisProximo(a: Analise, tempos: number[], t: number): Instante | null {
  const i = indiceMaisProximo(tempos, t);
  if (i < 0) return null;
  const s = a.series;
  return {
    t: tempos[i],
    velocidade: s.velocidade_kmh[i] ?? null,
    rpm: s.rpm[i] ?? null,
    marcha: s.marcha[i] ?? null,
    pedal: s.pedal_pct[i] ?? null,
  };
}

export function temposDaSerie(a: Analise): number[] {
  return preencherTempos(a.series.t_ms);
}

export const ROTULO_EVENTO: Record<Evento["tipo"], string> = {
  frenagem_brusca: "Frenagem brusca",
  aceleracao_brusca: "Aceleração brusca",
  marcha_lenta_longa: "Marcha lenta longa",
};

export function rotuloMarcha(marcha: number | null): string {
  if (marcha === null) return "—";
  if (marcha === 0) return "Ponto morto ou embreagem";
  return `${marcha}ª`;
}

export function textoPausa(ms: number): string {
  return `Sem dados · ${duracao(ms / 1000)}`;
}

/** Há leituras? Falso para viagem vazia. */
export function temLeituras(a: Analise): boolean {
  return a.series.t_ms.length > 0;
}

export function algumValor(xs: readonly (number | null)[]): boolean {
  return xs.some((v) => v !== null && v !== undefined);
}

/**
 * Cabeçalho (spec 01): identificação conforme a entrada e, em boot com hora
 * absoluta, data e hora. Sem hora absoluta, omite data e hora.
 */
export function descricaoCabecalho(a: Analise): { identificacao: string; quando: string | null } {
  const e = a.viagem.entrada;
  if (e.tipo === "boot") {
    const { inicio_em: ini, fim_em: fim } = a.viagem;
    const quando =
      ini && fim
        ? `${dataCompleta(Date.parse(ini))} ${hora(Date.parse(ini))}–${hora(Date.parse(fim))}`
        : null;
    return { identificacao: `Dongle ${e.dongle_id} · boot ${e.boot_id}`, quando };
  }
  const t0 = Date.parse(e.t0);
  const t1 = Date.parse(e.t1);
  const mesmoDia = diaMes(t0) === diaMes(t1);
  const periodo = mesmoDia
    ? `${diaMes(t0)} ${hora(t0)}–${hora(t1)}`
    : `${diaMes(t0)} ${hora(t0)}–${diaMes(t1)} ${hora(t1)}`;
  const partes = a.viagem.partes.length > 1 ? ` · ${a.viagem.partes.length} partes` : "";
  return { identificacao: `Dongle ${e.dongle_id} · ${periodo}${partes}`, quando: null };
}
