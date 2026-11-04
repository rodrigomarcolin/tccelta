import type { FaixaSegmento } from "@/contrato/tipos";

/** Cores lidas em tempo de desenho, do tema ativo (inclusive na impressão). */
export function corCss(nome: string, fallback = "#888888"): string {
  if (typeof document === "undefined") return fallback;
  const v = getComputedStyle(document.documentElement).getPropertyValue(nome).trim();
  return v || fallback;
}

export function varFaixa(faixa: FaixaSegmento): string {
  if (faixa === "falha_gps" || faixa === "sem_velocidade") return "--faixa-falha";
  return `--faixa-${faixa}`;
}

export function corFaixa(faixa: FaixaSegmento): string {
  return corCss(varFaixa(faixa));
}

export function varMarcha(marcha: number): string {
  if (marcha <= 0) return "--marcha-0";
  return `--marcha-${Math.min(marcha, 6)}`;
}

export function corMarcha(marcha: number): string {
  return corCss(varMarcha(marcha));
}
