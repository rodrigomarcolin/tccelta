import { FUSO } from "@/config";

export const TRACO = "—";

const cache = new Map<number, Intl.NumberFormat>();

function formatador(casas: number): Intl.NumberFormat {
  let f = cache.get(casas);
  if (!f) {
    f = new Intl.NumberFormat("pt-BR", {
      minimumFractionDigits: casas,
      maximumFractionDigits: casas,
      useGrouping: true,
    });
    cache.set(casas, f);
  }
  return f;
}

/** Número no padrão pt-BR; `—` para null, undefined ou não finito. */
export function numero(valor: number | null | undefined, casas = 0): string {
  if (valor === null || valor === undefined || !Number.isFinite(valor)) return TRACO;
  const texto = formatador(casas).format(valor);
  // Evita "-0".
  return /^-0(,0*)?$/.test(texto) ? texto.slice(1) : texto;
}

export function porcento(valor: number | null | undefined, casas = 0): string {
  return valor === null || valor === undefined ? TRACO : `${numero(valor, casas)}%`;
}

/** Duração em segundos: `3 min 55 s`, `23 s`, `1 h 05 min`. */
export function duracao(segundos: number | null | undefined): string {
  if (segundos === null || segundos === undefined || !Number.isFinite(segundos)) return TRACO;
  const total = Math.max(0, Math.round(segundos));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0) return `${h} h ${String(m).padStart(2, "0")} min`;
  if (m > 0) return `${m} min ${s} s`;
  return `${s} s`;
}

/** Milissegundos como `m:ss` (tempo desde o início). */
export function minutosSegundos(ms: number | null | undefined): string {
  if (ms === null || ms === undefined || !Number.isFinite(ms)) return TRACO;
  const total = Math.max(0, Math.round(ms / 1000));
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

function partes(epochMs: number, opcoes: Intl.DateTimeFormatOptions): Record<string, string> {
  const saida: Record<string, string> = {};
  for (const p of new Intl.DateTimeFormat("pt-BR", {
    timeZone: FUSO,
    hourCycle: "h23",
    ...opcoes,
  }).formatToParts(new Date(epochMs))) {
    saida[p.type] = p.value;
  }
  return saida;
}

/** `HH:mm` no fuso fixo. */
export function hora(epochMs: number | null | undefined): string {
  if (epochMs === null || epochMs === undefined || !Number.isFinite(epochMs)) return TRACO;
  const p = partes(epochMs, { hour: "2-digit", minute: "2-digit" });
  return `${p.hour}:${p.minute}`;
}

/** `HH:mm:ss` no fuso fixo. */
export function horaCompleta(epochMs: number | null | undefined): string {
  if (epochMs === null || epochMs === undefined || !Number.isFinite(epochMs)) return TRACO;
  const p = partes(epochMs, { hour: "2-digit", minute: "2-digit", second: "2-digit" });
  return `${p.hour}:${p.minute}:${p.second}`;
}

/** `dd/MM` no fuso fixo. */
export function diaMes(epochMs: number | null | undefined): string {
  if (epochMs === null || epochMs === undefined || !Number.isFinite(epochMs)) return TRACO;
  const p = partes(epochMs, { day: "2-digit", month: "2-digit" });
  return `${p.day}/${p.month}`;
}

/** `dd/MM/yyyy` no fuso fixo. */
export function dataCompleta(epochMs: number | null | undefined): string {
  if (epochMs === null || epochMs === undefined || !Number.isFinite(epochMs)) return TRACO;
  const p = partes(epochMs, { day: "2-digit", month: "2-digit", year: "numeric" });
  return `${p.day}/${p.month}/${p.year}`;
}
