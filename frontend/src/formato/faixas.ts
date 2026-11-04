import type { FaixaId, FaixaSegmento } from "@/contrato/tipos";

export const MOVIMENTO_KMH = 3;

export const ORDEM_FAIXAS: FaixaId[] = [
  "parado",
  "ate_30",
  "30_60",
  "60_80",
  "80_110",
  "acima_110",
];

export const ROTULO_FAIXA: Record<FaixaId, string> = {
  parado: "Parado",
  ate_30: "< 30 km/h",
  "30_60": "30–60 km/h",
  "60_80": "60–80 km/h",
  "80_110": "80–110 km/h",
  acima_110: "> 110 km/h",
};

export function rotuloFaixa(faixa: FaixaSegmento): string {
  if (faixa === "falha_gps") return "Falha de GPS";
  if (faixa === "sem_velocidade") return "Sem velocidade";
  return ROTULO_FAIXA[faixa];
}

/** Faixa de uma velocidade (spec 02): limites fechados à esquerda. */
export function faixaDeVelocidade(kmh: number): FaixaId {
  if (kmh < MOVIMENTO_KMH) return "parado";
  if (kmh < 30) return "ate_30";
  if (kmh < 60) return "30_60";
  if (kmh < 80) return "60_80";
  if (kmh < 110) return "80_110";
  return "acima_110";
}
