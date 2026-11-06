import { TIMEZONE } from "@/config";

export const DASH = "—";

const cache = new Map<number, Intl.NumberFormat>();

function formatter(digits: number): Intl.NumberFormat {
  let f = cache.get(digits);
  if (!f) {
    f = new Intl.NumberFormat("pt-BR", {
      minimumFractionDigits: digits,
      maximumFractionDigits: digits,
      useGrouping: true,
    });
    cache.set(digits, f);
  }
  return f;
}

/** Number in pt-BR format; `—` for null, undefined or non-finite. */
export function formatNumber(value: number | null | undefined, digits = 0): string {
  if (value === null || value === undefined || !Number.isFinite(value)) return DASH;
  const text = formatter(digits).format(value);
  // Avoids "-0".
  return /^-0(,0*)?$/.test(text) ? text.slice(1) : text;
}

export function formatPercent(value: number | null | undefined, digits = 0): string {
  return value === null || value === undefined ? DASH : `${formatNumber(value, digits)}%`;
}

/** Duration in seconds: `3 min 55 s`, `23 s`, `1 h 05 min`. */
export function formatDuration(seconds: number | null | undefined): string {
  if (seconds === null || seconds === undefined || !Number.isFinite(seconds)) return DASH;
  const total = Math.max(0, Math.round(seconds));
  const h = Math.floor(total / 3600);
  const m = Math.floor((total % 3600) / 60);
  const s = total % 60;
  if (h > 0) return `${h} h ${String(m).padStart(2, "0")} min`;
  if (m > 0) return `${m} min ${s} s`;
  return `${s} s`;
}

/** Milliseconds as `m:ss` (time since the start). */
export function minutesSeconds(ms: number | null | undefined): string {
  if (ms === null || ms === undefined || !Number.isFinite(ms)) return DASH;
  const total = Math.max(0, Math.round(ms / 1000));
  const m = Math.floor(total / 60);
  const s = total % 60;
  return `${m}:${String(s).padStart(2, "0")}`;
}

function dateParts(epochMs: number, options: Intl.DateTimeFormatOptions): Record<string, string> {
  const out: Record<string, string> = {};
  for (const p of new Intl.DateTimeFormat("pt-BR", {
    timeZone: TIMEZONE,
    hourCycle: "h23",
    ...options,
  }).formatToParts(new Date(epochMs))) {
    out[p.type] = p.value;
  }
  return out;
}

/** `HH:mm` in the fixed timezone. */
export function formatTime(epochMs: number | null | undefined): string {
  if (epochMs === null || epochMs === undefined || !Number.isFinite(epochMs)) return DASH;
  const p = dateParts(epochMs, { hour: "2-digit", minute: "2-digit" });
  return `${p.hour}:${p.minute}`;
}

/** `HH:mm:ss` in the fixed timezone. */
export function formatFullTime(epochMs: number | null | undefined): string {
  if (epochMs === null || epochMs === undefined || !Number.isFinite(epochMs)) return DASH;
  const p = dateParts(epochMs, { hour: "2-digit", minute: "2-digit", second: "2-digit" });
  return `${p.hour}:${p.minute}:${p.second}`;
}

/** `dd/MM` in the fixed timezone. */
export function dayMonth(epochMs: number | null | undefined): string {
  if (epochMs === null || epochMs === undefined || !Number.isFinite(epochMs)) return DASH;
  const p = dateParts(epochMs, { day: "2-digit", month: "2-digit" });
  return `${p.day}/${p.month}`;
}

/** `dd/MM/yyyy` in the fixed timezone. */
export function fullDate(epochMs: number | null | undefined): string {
  if (epochMs === null || epochMs === undefined || !Number.isFinite(epochMs)) return DASH;
  const p = dateParts(epochMs, { day: "2-digit", month: "2-digit", year: "numeric" });
  return `${p.day}/${p.month}/${p.year}`;
}
