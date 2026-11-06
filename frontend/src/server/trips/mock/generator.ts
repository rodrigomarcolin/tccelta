import type {
  TripAnalysis,
  TripWarning,
  FuelId,
  TripInput,
  TripEvent,
  BandId,
  SegmentBand,
  SpeedBand,
  Stop,
  TripPart,
  GearShift,
} from "@/domain/trip/types";
import { bandForSpeed, BAND_ORDER } from "@/format/bands";

/**
 * Deterministic synthetic trip generator. It is not part of the product:
 * it only feeds the FE1 mock. The numbers follow the rules of spec 02 in a
 * simplified but consistent way (parallel arrays, bands adding up to 100%).
 */

export const PRESETS: Record<FuelId, { afr: number; density: number }> = {
  gasolina_e30: { afr: 13.0, density: 750 },
  gasolina_e32: { afr: 13.0, density: 750 },
  gasolina_e27: { afr: 13.2, density: 754 },
  gasolina_pura: { afr: 14.7, density: 745 },
  etanol_hidratado: { afr: 9.0, density: 809 },
  diesel: { afr: 14.5, density: 832 },
};

const LAT0 = -25.43;
const LON0 = -49.27;
const IDLE = 800;
const K_GEAR = [100, 57, 38, 28, 21]; // rpm per km/h of each gear
const UPSHIFT_KMH = [0, 14, 28, 48, 70];

/** Speed profile (s, km/h, kind of the stretch ending at this point). */
const PROFILE: [number, number, "l" | "s"][] = [
  [0, 0, "s"],
  [22, 0, "s"],
  [40, 35, "s"],
  [70, 50, "s"],
  [100, 50, "s"],
  [103, 95, "l"], // harsh acceleration
  [130, 95, "s"],
  [150, 112, "s"],
  [175, 112, "s"],
  [177.2, 70, "l"], // harsh braking
  [200, 60, "s"],
  [215, 0, "s"],
  [235, 0, "s"], // stop
  [255, 40, "s"],
  [290, 60, "s"],
  [330, 85, "s"],
  [380, 85, "s"],
  [400, 45, "s"],
  [420, 20, "s"],
  [440, 0, "s"],
  [500, 0, "s"], // long idling
];

const CLUTCH: [number, number][] = [
  [205, 213],
  [410, 418],
];

export const PROFILE_DURATION_S = 500;

function mulberry32(a: number): () => number {
  let s = a >>> 0;
  return () => {
    s = (s + 0x6d2b79f5) >>> 0;
    let t = s;
    t = Math.imul(t ^ (t >>> 15), t | 1);
    t ^= t + Math.imul(t ^ (t >>> 7), t | 61);
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

function profileSpeed(s: number): number {
  if (s <= 0) return 0;
  for (let i = 1; i < PROFILE.length; i++) {
    const [t1, v1, kind] = PROFILE[i];
    const [t0, v0] = PROFILE[i - 1];
    if (s <= t1) {
      const f = (s - t0) / (t1 - t0);
      const g = kind === "l" ? f : f * f * (3 - 2 * f);
      return v0 + (v1 - v0) * g;
    }
  }
  return PROFILE[PROFILE.length - 1][1];
}

const round = (v: number, c: number) => Math.round(v * 10 ** c) / 10 ** c;
const clamp = (v: number, a: number, b: number) => Math.min(b, Math.max(a, v));

function median(xs: number[]): number | null {
  if (xs.length === 0) return null;
  const s = [...xs].sort((a, b) => a - b);
  const m = s.length >> 1;
  return s.length % 2 ? s[m] : (s[m - 1] + s[m]) / 2;
}

function quantile(xs: number[], q: number): number | null {
  if (xs.length === 0) return null;
  const s = [...xs].sort((a, b) => a - b);
  const pos = (s.length - 1) * q;
  const lo = Math.floor(pos);
  const hi = Math.ceil(pos);
  return s[lo] + (s[hi] - s[lo]) * (pos - lo);
}

export type PartSpec = {
  bootId: number;
  /** Start of the part window on the trip t_ms axis. */
  startMs: number;
  /** Profile stretch (s) used by this part. */
  profileFrom: number;
  profileUntil: number;
  seed: number;
};

export type GenerateOptions = {
  input: TripInput;
  driver: { id: string; nome: string };
  category: string;
  fuel: FuelId;
  parts: PartSpec[];
  /** Absolute time (epoch ms) matching t_ms = 0; null = no time. */
  offsetMs: number | null;
  stepMs?: number;
};

type Point = {
  t: number;
  part: number;
  v: number;
  rpm: number;
  gear: number | null;
  pedal: number;
  load: number;
  lh: number;
  a: number | null;
};

/** Points of one part (single grid). */
function generatePart(spec: PartSpec, index: number, stepMs: number): Point[] {
  const rnd = mulberry32(spec.seed);
  const n = Math.floor(((spec.profileUntil - spec.profileFrom) * 1000) / stepMs) + 1;
  const pts: Point[] = [];
  let currentGear = 1;
  let lastShift = -1e9;
  for (let k = 0; k < n; k++) {
    const profileS = spec.profileFrom + (k * stepMs) / 1000;
    let v = profileSpeed(profileS);
    if (v > 3) v += (rnd() - 0.5) * 0.9;
    v = Math.max(0, Math.round(v));
    const t = spec.startMs + k * stepMs;
    const clutch = CLUTCH.some(([a, b]) => profileS >= a && profileS <= b);

    // Gear with hysteresis.
    let target = 1;
    for (let g = 0; g < UPSHIFT_KMH.length; g++) {
      if (v >= UPSHIFT_KMH[g]) target = g + 1;
    }
    if (target < currentGear && v >= UPSHIFT_KMH[currentGear - 1] - 4) target = currentGear;
    let gear: number | null = currentGear;
    if (target !== currentGear) {
      currentGear = target;
      lastShift = t;
    }
    gear = currentGear;
    let rpm = Math.max(IDLE, K_GEAR[currentGear - 1] * v) * (1 + (rnd() - 0.5) * 0.03);
    if (t - lastShift < 2 * stepMs && t >= lastShift) {
      gear = null; // shift in progress: gear undefined
      rpm = Math.max(IDLE, rpm * 0.82);
    }
    if (v < 8) {
      gear = v < 3 ? null : gear;
    }
    if (clutch && v >= 8) {
      gear = 0;
      rpm = IDLE + 60 + rnd() * 50;
    }
    if (v < 3) rpm = IDLE + (rnd() - 0.5) * 30;
    pts.push({
      t,
      part: index,
      v,
      rpm: Math.round(rpm),
      gear,
      pedal: 0,
      load: 0,
      lh: 0,
      a: null,
    });
  }

  // Acceleration (central difference of ±0.5 s).
  const h = Math.max(1, Math.round(500 / stepMs));
  const dt = stepMs / 1000;
  for (let i = 0; i < pts.length; i++) {
    if (i - h >= 0 && i + h < pts.length) {
      pts[i].a = round((pts[i + h].v - pts[i - h].v) / 3.6 / (2 * h * dt), 3);
    }
  }
  for (const p of pts) {
    const a = p.a ?? 0;
    let pedal = p.v < 1 ? 0 : 6 + 0.17 * p.v + 16 * Math.max(a, 0) + (rnd() - 0.5) * 3;
    if (a < -0.3) pedal = 0;
    if (p.v < 1) pedal = 0;
    p.pedal = round(clamp(pedal, 0, 100), 1);
    p.load = round(clamp(14 + 0.65 * p.pedal + p.rpm / 400, 8, 100), 1);
    let maf = 1.5 + (p.rpm / 1000) * (p.load / 100) * 14;
    if (p.pedal === 0 && p.rpm > 1200 && p.v > 15) maf = 1.5;
    p.lh = maf; // provisional: g/s; converted later
  }
  return pts;
}

function haversineKm(lat1: number, lon1: number, lat2: number, lon2: number): number {
  const r = (g: number) => (g * Math.PI) / 180;
  const dLat = r(lat2 - lat1);
  const dLon = r(lon2 - lon1);
  const a =
    Math.sin(dLat / 2) ** 2 + Math.cos(r(lat1)) * Math.cos(r(lat2)) * Math.sin(dLon / 2) ** 2;
  return 2 * 6371 * Math.asin(Math.sqrt(a));
}

function temperatureAt(sSinceStart: number): number {
  return 90 - 42 * Math.exp(-sSinceStart / 90);
}

export function generateAnalysis(opts: GenerateOptions): TripAnalysis {
  const stepMs = opts.stepMs ?? 250;
  const dt = stepMs / 1000;
  const { afr, density } = PRESETS[opts.fuel];

  const partPoints = opts.parts.map((p, i) => generatePart(p, i, stepMs));
  for (const pts of partPoints) for (const p of pts) p.lh = round((p.lh / afr / density) * 3600, 3);
  const all = partPoints.flat();

  // ---- Summary ----------------------------------------------------------
  const distance = all.reduce((s, p) => s + (p.v * dt) / 3600, 0);
  const duration = all.length * dt;
  const tripStart = opts.parts[0].startMs;
  const lastPart = partPoints[partPoints.length - 1];
  const tripEnd = lastPart[lastPart.length - 1].t + stepMs;
  const noData = (tripEnd - tripStart) / 1000 - duration;
  const moving = all.filter((p) => p.v >= 3);
  const stopped = all.filter((p) => p.v < 3);
  const running = all.filter((p) => p.rpm >= 300);
  const idling = all.filter((p) => p.v < 3 && p.rpm >= 300);
  const liters = all.reduce((s, p) => s + (p.lh * dt) / 3600, 0);

  // Bands.
  const acc: Record<BandId, { t: number; d: number }> = {
    parado: { t: 0, d: 0 },
    ate_30: { t: 0, d: 0 },
    "30_60": { t: 0, d: 0 },
    "60_80": { t: 0, d: 0 },
    "80_110": { t: 0, d: 0 },
    acima_110: { t: 0, d: 0 },
  };
  for (const p of all) {
    const f = bandForSpeed(p.v);
    acc[f].t += dt;
    acc[f].d += (p.v * dt) / 3600;
  }
  const movingDistance = BAND_ORDER.filter((f) => f !== "parado").reduce((s, f) => s + acc[f].d, 0);
  const limits: Record<BandId, [number, number | null]> = {
    parado: [0, 3],
    ate_30: [3, 30],
    "30_60": [30, 60],
    "60_80": [60, 80],
    "80_110": [80, 110],
    acima_110: [110, null],
  };
  const bands: SpeedBand[] = BAND_ORDER.map((id) => ({
    id,
    de_kmh: limits[id][0],
    ate_kmh: limits[id][1],
    tempo_s: round(acc[id].t, 2),
    distancia_km: round(acc[id].d, 4),
    distancia_pct:
      id === "parado" || movingDistance === 0 ? null : round((acc[id].d / movingDistance) * 100, 2),
  }));

  // ---- Events -----------------------------------------------------------
  const events: TripEvent[] = [];
  const stops: Stop[] = [];
  const nearestPosition = (t: number, route: { t: number; lat: number; lon: number }[]) => {
    let best: { t: number; lat: number; lon: number } | null = null;
    let d = Infinity;
    for (const r of route) {
      const x = Math.abs(r.t - t);
      if (x < d) {
        d = x;
        best = r;
      }
    }
    return best && d <= 5000 ? best : null;
  };

  // GPS positions (1 Hz), integrated from speed.
  const positions: { t: number; lat: number; lon: number; part: number }[] = [];
  {
    let x = 0;
    let y = 0;
    partPoints.forEach((pts, ip) => {
      if (ip > 0) {
        x += 350;
        y -= 120;
      }
      let nextAt = pts[0].t;
      for (let i = 0; i < pts.length; i++) {
        const p = pts[i];
        const s = (p.t - pts[0].t) / 1000 + opts.parts[ip].profileFrom;
        const heading =
          0.6 +
          0.35 * Math.sin(s / 45) +
          (s > 60 ? 0.9 * smooth((s - 60) / 8) : 0) -
          (s > 250 ? 1.2 * smooth((s - 250) / 10) : 0) +
          (s > 340 ? 0.8 * smooth((s - 340) / 10) : 0);
        const d = (p.v / 3.6) * dt;
        x += d * Math.cos(heading);
        y += d * Math.sin(heading);
        if (p.t >= nextAt) {
          nextAt += 1000;
          positions.push({
            t: p.t,
            lat: round(LAT0 + y / 111320, 6),
            lon: round(LON0 + x / (111320 * Math.cos((LAT0 * Math.PI) / 180)), 6),
            part: ip,
          });
        }
      }
    });
  }

  const partAt = (ip: number) => partPoints[ip];
  partPoints.forEach((_, ip) => {
    const pts = partAt(ip);
    const detect = (
      type: "aceleracao_brusca" | "frenagem_brusca",
      matches: (a: number) => boolean,
      peakOf: (xs: number[]) => number,
    ) => {
      let i = 0;
      while (i < pts.length) {
        if (pts[i].a !== null && matches(pts[i].a as number)) {
          let j = i;
          while (j + 1 < pts.length && pts[j + 1].a !== null && matches(pts[j + 1].a as number))
            j++;
          const dur = (pts[j].t - pts[i].t + stepMs) / 1000;
          if (dur * 1000 >= 500) {
            const pos = nearestPosition(pts[i].t, positions);
            events.push({
              tipo: type,
              t_inicio_ms: pts[i].t,
              duracao_s: round(dur, 2),
              pico: round(peakOf(pts.slice(i, j + 1).map((p) => p.a as number)), 2),
              pico_unidade: "m/s2",
              velocidade_inicial_kmh: pts[i].v,
              velocidade_final_kmh: pts[j].v,
              lat: pos?.lat ?? null,
              lon: pos?.lon ?? null,
            });
          }
          i = j + 1;
        } else i++;
      }
    };
    detect(
      "aceleracao_brusca",
      (a) => a >= 3.0,
      (xs) => Math.max(...xs),
    );
    detect(
      "frenagem_brusca",
      (a) => a <= -3.5,
      (xs) => Math.min(...xs),
    );

    // Stops and long idling.
    let i = 0;
    while (i < pts.length) {
      if (pts[i].v < 3) {
        let j = i;
        while (j + 1 < pts.length && pts[j + 1].v < 3) j++;
        const dur = (pts[j].t - pts[i].t + stepMs) / 1000;
        const isRunning = pts.slice(i, j + 1).every((p) => p.rpm >= 300);
        if (dur >= 5) {
          const pos = nearestPosition(pts[i].t, positions);
          stops.push({
            t_inicio_ms: pts[i].t,
            duracao_s: round(dur, 2),
            motor_ligado: isRunning,
            lat: pos?.lat ?? null,
            lon: pos?.lon ?? null,
          });
          if (isRunning && dur * 1000 >= 60000) {
            events.push({
              tipo: "marcha_lenta_longa",
              t_inicio_ms: pts[i].t,
              duracao_s: round(dur, 2),
              pico: round(dur, 2),
              pico_unidade: "s",
              velocidade_inicial_kmh: pts[i].v,
              velocidade_final_kmh: pts[j].v,
              lat: pos?.lat ?? null,
              lon: pos?.lon ?? null,
            });
          }
        }
        i = j + 1;
      } else i++;
    }
  });
  events.sort((a, b) => a.t_inicio_ms - b.t_inicio_ms);
  stops.sort((a, b) => a.t_inicio_ms - b.t_inicio_ms);

  // ---- Route ------------------------------------------------------------
  const routeT = positions.map((p) => p.t);
  const meanSpeedBetween = (t0: number, t1: number, ip: number): number | null => {
    const xs = partAt(ip)
      .filter((p) => p.t >= t0 && p.t < t1)
      .map((p) => p.v);
    return xs.length ? xs.reduce((a, b) => a + b, 0) / xs.length : null;
  };
  const segSpeed: (number | null)[] = [];
  const segBand: SegmentBand[] = [];
  for (let i = 0; i < positions.length - 1; i++) {
    const a = positions[i];
    const b = positions[i + 1];
    if (a.part !== b.part || b.t - a.t > 5000) {
      segSpeed.push(null);
      segBand.push("falha_gps");
      continue;
    }
    const v = meanSpeedBetween(a.t, b.t, a.part);
    segSpeed.push(v === null ? null : round(v, 1));
    segBand.push(v === null ? "sem_velocidade" : bandForSpeed(v));
  }

  // ---- Gears -----------------------------------------------------------
  const shifts: GearShift[] = [];
  partPoints.forEach((pts) => {
    let last: { g: number; endT: number; rpm: number; v: number } | null = null;
    for (let i = 0; i < pts.length; i++) {
      const g = pts[i].gear;
      if (g === null || g === 0) continue;
      if (last && g !== last.g && pts[i].t - last.endT <= 4000) {
        shifts.push({
          t_ms: last.endT,
          direcao: g > last.g ? "subida" : "reducao",
          de: last.g,
          para: g,
          rpm_antes: last.rpm,
          rpm_depois: pts[i].rpm,
          velocidade_kmh: last.v,
        });
      }
      last = { g, endT: pts[i].t, rpm: pts[i].rpm, v: pts[i].v };
    }
  });
  const upshifts = shifts.filter((t) => t.direcao === "subida").map((t) => t.rpm_antes);
  const downshifts = shifts.filter((t) => t.direcao === "reducao").map((t) => t.rpm_antes);
  const gears = [...new Set(all.map((p) => p.gear).filter((g): g is number => !!g))].sort(
    (a, b) => a - b,
  );
  const movingTime = moving.length * dt;
  const plateaus = gears.map((g) => {
    const ps = moving.filter((p) => p.gear === g);
    return {
      marcha: g,
      rpm_por_kmh: round(median(ps.map((p) => p.rpm / Math.max(1, p.v))) ?? 0, 1),
      tempo_s: round(ps.length * dt, 2),
      tempo_pct: round(movingTime ? ((ps.length * dt) / movingTime) * 100 : 0, 2),
    };
  });
  const neutral = all.filter((p) => p.gear === 0).length * dt;
  const idleRpm = median(idling.map((p) => p.rpm));

  // ---- Distributions ---------------------------------------------------
  const runningTime = running.length * dt;
  const rpmCuts = [0, 1000, 1500, 2000, 2500, 3000, 3500, 4000, 5000];
  const rpmBins = rpmCuts.map((from, i) => {
    const to = i + 1 < rpmCuts.length ? rpmCuts[i + 1] : null;
    const n = running.filter((p) => p.rpm >= from && (to === null || p.rpm < to)).length;
    return {
      de: from,
      ate: to,
      tempo_s: round(n * dt, 2),
      pct: round(runningTime ? ((n * dt) / runningTime) * 100 : 0, 2),
    };
  });
  const pedalCuts = [0, 5, 20, 40, 60, 80];
  const pedalBins = pedalCuts.map((from, i) => {
    const to = i + 1 < pedalCuts.length ? pedalCuts[i + 1] : 100;
    const n = running.filter((p) => p.pedal >= from && (to === 100 ? true : p.pedal < to)).length;
    return {
      de: from,
      ate: to,
      ...(i === 0 ? { rotulo: "Solto" } : {}),
      tempo_s: round(n * dt, 2),
      pct: round(runningTime ? ((n * dt) / runningTime) * 100 : 0, 2),
    };
  });
  // Rows = rpm bands (500 rpm); columns = pedal bands (10%).
  const nRpm = 12;
  const nPedal = 10;
  const grid: number[][] = Array.from({ length: nRpm }, () => Array<number>(nPedal).fill(0));
  for (const p of running) {
    const r = Math.min(nRpm - 1, Math.floor(p.rpm / 500));
    const c = Math.min(nPedal - 1, Math.floor(p.pedal / 10));
    grid[r][c] += dt;
  }

  // ---- Driver ----------------------------------------------------------
  const variations: number[] = [];
  partPoints.forEach((pts) => {
    for (let i = 1; i < pts.length; i++)
      variations.push(Math.abs(pts[i].pedal - pts[i - 1].pedal) / dt);
  });
  const engineBrake = moving.filter((p) => p.pedal <= 3 && p.rpm >= 1200 && p.v >= 15).length;
  const withAccel = moving.filter((p) => p.a !== null).map((p) => p.a as number);
  const rms = withAccel.length
    ? Math.sqrt(withAccel.reduce((s, a) => s + a * a, 0) / withAccel.length)
    : null;
  let cruise = 0;
  partPoints.forEach((pts) => {
    let i = 0;
    while (i < pts.length) {
      if (pts[i].v < 40) {
        i++;
        continue;
      }
      let j = i;
      let mn = pts[i].v;
      let mx = pts[i].v;
      while (j + 1 < pts.length && pts[j + 1].v >= 40) {
        const nmn = Math.min(mn, pts[j + 1].v);
        const nmx = Math.max(mx, pts[j + 1].v);
        if (nmx - nmn > 10) break;
        mn = nmn;
        mx = nmx;
        j++;
      }
      const dur = (pts[j].t - pts[i].t + stepMs) / 1000;
      if (dur >= 30) {
        cruise += dur;
        i = j + 1;
      } else i++;
    }
  });
  const highPedal = running.filter((p) => p.pedal >= 80).length * dt;

  // ---- Series -----------------------------------------------------------
  const seriesData = {
    t_ms: [] as (number | null)[],
    velocidade_kmh: [] as (number | null)[],
    rpm: [] as (number | null)[],
    rpm_por_kmh: [] as (number | null)[],
    marcha: [] as (number | null)[],
    pedal_pct: [] as (number | null)[],
    aceleracao_ms2: [] as (number | null)[],
    carga_pct: [] as (number | null)[],
    consumo_l_h: [] as (number | null)[],
    combustivel_acumulado_l: [] as (number | null)[],
    distancia_acumulada_km: [] as (number | null)[],
  };
  let accL = 0;
  let accD = 0;
  partPoints.forEach((pts, ip) => {
    for (const p of pts) {
      accL += (p.lh * dt) / 3600;
      accD += (p.v * dt) / 3600;
      seriesData.t_ms.push(p.t);
      seriesData.velocidade_kmh.push(p.v);
      seriesData.rpm.push(p.rpm);
      seriesData.rpm_por_kmh.push(p.v >= 8 ? round(p.rpm / p.v, 2) : null);
      seriesData.marcha.push(p.gear);
      seriesData.pedal_pct.push(p.pedal);
      seriesData.aceleracao_ms2.push(p.a);
      seriesData.carga_pct.push(p.load);
      seriesData.consumo_l_h.push(p.lh);
      seriesData.combustivel_acumulado_l.push(round(accL, 5));
      seriesData.distancia_acumulada_km.push(round(accD, 4));
    }
    if (ip < partPoints.length - 1) {
      const last = pts[pts.length - 1].t;
      // Separator between parts: instant right after the end, with null values.
      seriesData.t_ms.push(last + stepMs);
      for (const k of Object.keys(seriesData) as (keyof typeof seriesData)[]) {
        if (k !== "t_ms") seriesData[k].push(null);
      }
    }
  });

  const slowT: (number | null)[] = [];
  const slowC: (number | null)[] = [];
  const tankT: (number | null)[] = [];
  const tankPct: (number | null)[] = [];
  const tankMedian: (number | null)[] = [];
  partPoints.forEach((pts, ip) => {
    const from = pts[0].t;
    const end = pts[pts.length - 1].t;
    const rnd = mulberry32(opts.parts[ip].seed + 99);
    const level: number[] = [];
    for (let t = from; t <= end; t += 5000) {
      slowT.push(t);
      slowC.push(Math.round(temperatureAt((t - opts.parts[0].startMs) / 1000 + (ip ? 600 : 0))));
    }
    for (let t = from; t <= end; t += 10000) {
      tankT.push(t);
      level.push(round(62 - ((t - from) / 1000) * 0.004 + (rnd() - 0.5) * 1.2, 1));
    }
    level.forEach((v, i) => {
      tankPct.push(v);
      const window = level.slice(Math.max(0, i - 3), i + 4);
      tankMedian.push(round(median(window) ?? v, 1));
    });
    if (ip < partPoints.length - 1) {
      slowT.push(end + 1000);
      slowC.push(null);
      tankT.push(end + 1000);
      tankPct.push(null);
      tankMedian.push(null);
    }
  });

  // ---- Engine ----------------------------------------------------------
  const t0 = opts.parts[0].startMs;
  const warmup = Math.ceil(90 * Math.log(42 / 10));
  const cold = all.filter(
    (p) => p.rpm >= 300 && temperatureAt((p.t - t0) / 1000) < 60 && (p.rpm > 3000 || p.pedal > 60),
  ).length;

  // ---- Quality ---------------------------------------------------------
  let gpsDistance = 0;
  let gaps = 0;
  for (let i = 0; i < positions.length - 1; i++) {
    const a = positions[i];
    const b = positions[i + 1];
    if (a.part !== b.part || b.t - a.t > 5000) gaps += a.part !== b.part ? 0 : 1;
    else gpsDistance += haversineKm(a.lat, a.lon, b.lat, b.lon);
  }
  const readings = Math.round(
    (duration / (stepMs / 1000)) * 3 + (duration / 0.3) * 2 + duration / 5 + duration / 10,
  );

  // ---- Trip ------------------------------------------------------------
  const tripParts: TripPart[] = opts.parts.map((p, i) => {
    const pts = partPoints[i];
    return {
      boot_generation: 0,
      boot_id: p.bootId,
      dados_inicio_t_ms: p.startMs - 2600,
      dados_fim_t_ms: pts[pts.length - 1].t + 400,
      inicio_t_ms: p.startMs,
      fim_t_ms: pts[pts.length - 1].t + stepMs,
    };
  });
  const iso = (ms: number) => new Date(ms).toISOString();
  const warnings: TripWarning[] = [];
  const pedalFraction = runningTime ? highPedal / runningTime : 0;
  if (pedalFraction > 0.1) warnings.push({ codigo: "consumo_pode_estar_subestimado" });

  const result: TripAnalysis = {
    versao_calculo: "1",

    viagem: {
      entrada: opts.input,
      motorista: opts.driver,
      categoria_viagem: opts.category,
      inicio_t_ms: tripStart,
      fim_t_ms: tripEnd,
      inicio_em: opts.offsetMs === null ? null : iso(opts.offsetMs + tripStart),
      fim_em: opts.offsetMs === null ? null : iso(opts.offsetMs + tripEnd),
      relogio:
        opts.offsetMs === null ? null : { deslocamento_ms: opts.offsetMs, incerteza_ms: 180 },
      partes: tripParts,
    },
    parametros: {
      combustivel: opts.fuel,
      afr,
      densidade_g_l: density,
      marcha_lenta_margem_rpm: 150,
      gps_falha_s: 5,
      marcha_lenta_reserva_rpm: 750,
      motor_ligado_rpm: 300,
      movimento_kmh: 3,
      velocidade_idade_max_ms: 2000,
      acel_brusca_ms2: 3.0,
      fren_brusca_ms2: -3.5,
      evento_min_ms: 500,
      marcha_lenta_longa_ms: 60000,
      parada_min_s: 5,
      taxa_min_km: 20,
      pedal_alto_pct: 80,
      pedal_alto_aviso_fracao: 0.1,
      cruzeiro_min_kmh: 40,
      aquecido_c: 80,
      frio_c: 60,
      serie_max_pontos: 5000,
    },
    avisos: warnings,
    resumo: {
      distancia_km: round(distance, 3),
      duracao_s: round(duration, 1),
      sem_dados_s: round(noData, 1),
      em_movimento_s: round(movingTime, 1),
      parado_s: round(stopped.length * dt, 1),
      velocidade_media_kmh: round(
        moving.reduce((s, p) => s + p.v, 0) / Math.max(1, moving.length),
        1,
      ),
      velocidade_maxima_kmh: Math.max(...all.map((p) => p.v)),
      motor_ligado_s: round(runningTime, 1),
      marcha_lenta_s: round(idling.length * dt, 1),
      marcha_lenta_pct: round(runningTime ? ((idling.length * dt) / runningTime) * 100 : 0, 1),
      combustivel_l: round(liters, 3),
      km_por_l: liters > 0.02 ? round(distance / liters, 2) : null,
      n_acel_bruscas: events.filter((e) => e.tipo === "aceleracao_brusca").length,
      n_fren_bruscas: events.filter((e) => e.tipo === "frenagem_brusca").length,
      acel_por_100km: null,
      fren_por_100km: null,
    },
    condutor: {
      pedal_medio_pct: round(
        moving.reduce((s, p) => s + p.pedal, 0) / Math.max(1, moving.length),
        1,
      ),
      pedal_alto_s: round(highPedal, 1),
      pedal_variacao_p95_pct_s: round(quantile(variations, 0.95) ?? 0, 1),
      freio_motor_pct: round(moving.length ? (engineBrake / moving.length) * 100 : 0, 1),
      aceleracao_rms_ms2: rms === null ? null : round(rms, 2),
      cruzeiro_s: round(cruise, 1),
    },
    motor: {
      aquecimento_s: warmup,
      temperatura_maxima_c: Math.max(...(slowC.filter((c) => c !== null) as number[])),
      esforco_frio_s: round(cold * dt, 1),
    },
    faixas_velocidade: bands,
    rota: {
      t_ms: routeT,
      lat: positions.map((p) => p.lat),
      lon: positions.map((p) => p.lon),
      segmentos: { velocidade_kmh: segSpeed, faixa: segBand },
    },
    paradas: stops,
    eventos: events,
    marchas: {
      marcha_lenta: { rpm: Math.round(idleRpm ?? 750), origem: "estimada", margem_rpm: 150 },
      patamares: plateaus,
      ponto_morto_embreagem_s: round(neutral, 1),
      trocas: shifts,
      subida_rpm: {
        mediana: upshifts.length ? Math.round(median(upshifts) as number) : null,
        p25: upshifts.length ? Math.round(quantile(upshifts, 0.25) as number) : null,
        p75: upshifts.length ? Math.round(quantile(upshifts, 0.75) as number) : null,
        n: upshifts.length,
      },
      reducao_rpm: {
        mediana: downshifts.length ? Math.round(median(downshifts) as number) : null,
        n: downshifts.length,
      },
    },
    distribuicoes: {
      rotacao: rpmBins,
      pedal: pedalBins,
      rotacao_pedal: {
        rpm_bin: 500,
        pedal_bin: 10,
        tempo_s: grid.map((l) => l.map((v) => round(v, 2))),
      },
    },
    series: {
      passo_ms: stepMs,
      exposicao_passo_ms: stepMs,
      ...seriesData,
    },
    series_lentas: {
      temperatura: { t_ms: slowT, c: slowC },
      nivel_tanque: { t_ms: tankT, pct: tankPct, mediana_movel_pct: tankMedian },
    },
    qualidade: {
      leituras_total: readings,
      leituras_por_s: round(readings / Math.max(1, duration), 2),
      periodos_mediana_ms: {
        "0x0C": stepMs,
        "0x0D": stepMs,
        "0x49": stepMs,
        "0x04": 300,
        "0x10": 300,
        "0x05": 5000,
        "0x2F": 10000,
      },
      combustivel_cobertura_pct: 99.8,
      gps: {
        periodo_mediana_s: 1,
        cobertura_pct: 100,
        n_falhas: gaps,
        maior_falha_s: 0,
        distancia_km: round(gpsDistance, 3),
        diferenca_distancia_pct:
          distance > 0 ? round(((gpsDistance - distance) / distance) * 100, 2) : null,
      },
    },
  };
  return result;
}

function smooth(f: number): number {
  const x = clamp(f, 0, 1);
  return x * x * (3 - 2 * x);
}
