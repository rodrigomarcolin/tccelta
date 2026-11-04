import type { FuelId } from "@/domain/trip/types";

/** Fixed display timezone: the fair is local and must not depend on the browser. */
export const TIMEZONE = "America/Sao_Paulo";

/** Initial value of the dongle field (INT1 replaces it with ANALISE_DONGLE_ID). */
export const INITIAL_DONGLE = 1;

export const FUELS: { id: FuelId; name: string }[] = [
  { id: "gasolina_e30", name: "Gasolina E30" },
  { id: "gasolina_e32", name: "Gasolina E32" },
  { id: "gasolina_e27", name: "Gasolina E27" },
  { id: "gasolina_pura", name: "Gasolina pura" },
  { id: "etanol_hidratado", name: "Etanol hidratado" },
  { id: "diesel", name: "Diesel" },
];

export function fuelName(id: string): string {
  return FUELS.find((c) => c.id === id)?.name ?? id;
}

export const POLL_WAITING_MS = 1000;
export const POLL_PROVISIONAL_MS = 5000;
export const POLL_PROVISIONAL_MAX_MS = 5 * 60 * 1000;
/** Polling backoff after consecutive failures: 1, 2, 4 and at most 5 s. */
export const POLL_BACKOFF_MAX_MS = 5000;
/** Consecutive failures before showing "Sem conexão". */
export const POLL_FAILURES_NOTICE = 3;
/** Wait limit in pendente or aguardando_dados (3 times the worker limit). */
export const WAIT_LIMIT_MS = 60_000;

/** Fraction of the summed part durations that each pause takes on the X axis. */
export const AXIS_PAUSE_FRACTION = 0.05;

export const THEME_COOKIE = "theme";
export type Theme = "dark" | "light";
export const DEFAULT_THEME: Theme = "dark";

export function validTheme(value: string | undefined | null): Theme {
  return value === "light" ? "light" : DEFAULT_THEME;
}

export const TILES = {
  light: "https://tile.openstreetmap.org/{z}/{x}/{y}.png",
  dark: "https://tile.openstreetmap.org/{z}/{x}/{y}.png",
} as const;

export const TILES_ATTRIBUTION =
  '&copy; <a href="https://www.openstreetmap.org/copyright">OpenStreetMap</a>';
