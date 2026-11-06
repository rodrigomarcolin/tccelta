import type { SegmentBand } from "@/domain/trip/types";

/** Colors read at draw time from the active theme (including when printing). */
export function cssColor(name: string, fallback = "#888888"): string {
  if (typeof document === "undefined") return fallback;
  const v = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
  return v || fallback;
}

export function bandVar(band: SegmentBand): string {
  if (band === "falha_gps" || band === "sem_velocidade") return "--band-fault";
  return `--band-${band}`;
}

export function bandColor(band: SegmentBand): string {
  return cssColor(bandVar(band));
}

export function gearVar(gear: number): string {
  if (gear <= 0) return "--gear-0";
  return `--gear-${Math.min(gear, 6)}`;
}

export function gearColor(gear: number): string {
  return cssColor(gearVar(gear));
}
