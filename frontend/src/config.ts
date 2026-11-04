import type { CombustivelId } from "@/contrato/tipos";

/** Fuso de exibição fixo: a feira é local e não deve depender do navegador. */
export const FUSO = "America/Sao_Paulo";

/** Valor inicial do campo dongle (a INT1 troca por ANALISE_DONGLE_ID). */
export const DONGLE_INICIAL = 1;

export const COMBUSTIVEIS: { id: CombustivelId; nome: string }[] = [
  { id: "gasolina_e30", nome: "Gasolina E30" },
  { id: "gasolina_e32", nome: "Gasolina E32" },
  { id: "gasolina_e27", nome: "Gasolina E27" },
  { id: "gasolina_pura", nome: "Gasolina pura" },
  { id: "etanol_hidratado", nome: "Etanol hidratado" },
  { id: "diesel", nome: "Diesel" },
];

export function nomeCombustivel(id: string): string {
  return COMBUSTIVEIS.find((c) => c.id === id)?.nome ?? id;
}

export const POLLING_ESPERA_MS = 1000;
export const POLLING_PROVISORIA_MS = 5000;
export const POLLING_PROVISORIA_MAX_MS = 5 * 60 * 1000;
/** Recuo do polling após falhas seguidas: 1, 2, 4 e no máximo 5 s. */
export const POLLING_RECUO_MAX_MS = 5000;
/** Falhas seguidas até avisar "Sem conexão". */
export const POLLING_FALHAS_AVISO = 3;
/** Limite de espera em pendente ou aguardando_dados (3 vezes o limite do worker). */
export const LIMITE_ESPERA_MS = 60_000;

/** Fração da soma das durações das partes que cada pausa ocupa no eixo X. */
export const FRACAO_PAUSA_EIXO = 0.05;

export const TEMA_COOKIE = "tema";
export type Tema = "escuro" | "claro";
export const TEMA_PADRAO: Tema = "escuro";

export function temaValido(valor: string | undefined | null): Tema {
  return valor === "claro" ? "claro" : TEMA_PADRAO;
}

export const TILES = {
  claro: "https://{s}.basemaps.cartocdn.com/light_all/{z}/{x}/{y}{r}.png",
  escuro: "https://{s}.basemaps.cartocdn.com/dark_all/{z}/{x}/{y}{r}.png",
} as const;

export const ATRIBUICAO_TILES =
  '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a> &copy; <a href="https://carto.com/attributions">CARTO</a>';
