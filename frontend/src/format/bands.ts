import type { BandId, SegmentBand } from "@/domain/trip/types";

export const MOVING_KMH = 3;

export const BAND_ORDER: BandId[] = ["parado", "ate_30", "30_60", "60_80", "80_110", "acima_110"];

export const BAND_LABEL: Record<BandId, string> = {
  parado: "Parado",
  ate_30: "< 30 km/h",
  "30_60": "30–60 km/h",
  "60_80": "60–80 km/h",
  "80_110": "80–110 km/h",
  acima_110: "> 110 km/h",
};

export function bandLabel(band: SegmentBand): string {
  if (band === "falha_gps") return "Falha de GPS";
  if (band === "sem_velocidade") return "Sem velocidade";
  return BAND_LABEL[band];
}

/** Band of a speed (spec 02): left-closed limits. */
export function bandForSpeed(kmh: number): BandId {
  if (kmh < MOVING_KMH) return "parado";
  if (kmh < 30) return "ate_30";
  if (kmh < 60) return "30_60";
  if (kmh < 80) return "60_80";
  if (kmh < 110) return "80_110";
  return "acima_110";
}
