import type { TripAnalysis, FuelId, TripInput } from "@/domain/trip/types";
import type { ScenarioId } from "./scenarios";
import { generateAnalysis, type PartSpec } from "./generator";

export type AnalysisInput = {
  input: TripInput;
  driver: { id: string; nome: string };
  category: string;
  fuel: FuelId;
};

const BOOT_START_MS = 33_481;
const BOOT_TIME = Date.parse("2026-10-28T14:03:22-03:00");

function base(e: AnalysisInput, noTime = false): TripAnalysis {
  const boot = e.input.tipo === "boot" ? e.input.boot_id : 8;
  return generateAnalysis({
    ...e,
    parts: [
      {
        bootId: boot,
        startMs: BOOT_START_MS,
        profileFrom: 0,
        profileUntil: 500,
        seed: 1000 + boot,
      },
    ],
    offsetMs: noTime ? null : BOOT_TIME - BOOT_START_MS,
  });
}

function intervalScenario(e: AnalysisInput): TripAnalysis {
  const t0 = e.input.tipo === "intervalo" ? Date.parse(e.input.t0) : BOOT_TIME;
  const t1 = e.input.tipo === "intervalo" ? Date.parse(e.input.t1) : t0 + 36_000_000;
  const span = t1 - t0;
  const onePart = 260_000;
  const start1 = Math.round(span * 0.1);
  const start2 = Math.max(Math.round(span * 0.6), start1 + onePart + 30_000);
  const parts: PartSpec[] = [
    { bootId: 8, startMs: start1, profileFrom: 0, profileUntil: 260, seed: 2008 },
    { bootId: 9, startMs: start2, profileFrom: 300, profileUntil: 500, seed: 2009 },
  ];
  return generateAnalysis({ ...e, parts, offsetMs: t0 });
}

/** Empties the analysis: nothing was read. Follows the "absence is null" rule. */
function emptied(a: TripAnalysis, warnings: TripAnalysis["avisos"]): TripAnalysis {
  const nulled = <T extends object>(o: T): T =>
    Object.fromEntries(Object.keys(o).map((k) => [k, null])) as T;
  return {
    ...a,
    avisos: warnings,
    viagem: {
      ...a.viagem,
      inicio_t_ms: null,
      fim_t_ms: null,
      inicio_em: null,
      fim_em: null,
      partes: [],
    },
    resumo: nulled(a.resumo),
    condutor: nulled(a.condutor),
    motor: nulled(a.motor),
    faixas_velocidade: [],
    rota: null,
    paradas: [],
    eventos: [],
    marchas: {
      marcha_lenta: null,
      patamares: [],
      ponto_morto_embreagem_s: null,
      trocas: [],
      subida_rpm: { mediana: null, p25: null, p75: null, n: 0 },
      reducao_rpm: { mediana: null, n: 0 },
    },
    distribuicoes: {
      rotacao: [],
      pedal: [],
      rotacao_pedal: { rpm_bin: 500, pedal_bin: 10, tempo_s: [] },
    },
    series: {
      passo_ms: 1000,
      exposicao_passo_ms: 1000,
      t_ms: [],
      velocidade_kmh: [],
      rpm: [],
      rpm_por_kmh: [],
      marcha: [],
      pedal_pct: [],
      aceleracao_ms2: [],
      carga_pct: [],
      consumo_l_h: [],
      combustivel_acumulado_l: [],
      distancia_acumulada_km: [],
    },
    series_lentas: {
      temperatura: { t_ms: [], c: [] },
      nivel_tanque: { t_ms: [], pct: [], mediana_movel_pct: [] },
    },
    qualidade: {
      leituras_total: 0,
      leituras_por_s: null,
      periodos_mediana_ms: {},
      combustivel_cobertura_pct: null,
      gps: {
        periodo_mediana_s: null,
        cobertura_pct: 0,
        n_falhas: null,
        maior_falha_s: null,
        distancia_km: null,
        diferenca_distancia_pct: null,
      },
    },
  };
}

function withoutGps(a: TripAnalysis): TripAnalysis {
  const nullPos = <T extends { lat: number | null; lon: number | null }>(x: T): T => ({
    ...x,
    lat: null,
    lon: null,
  });
  return {
    ...a,
    avisos: [{ codigo: "sem_gps" }, ...a.avisos],
    rota: null,
    paradas: a.paradas.map(nullPos),
    eventos: a.eventos.map(nullPos),
    qualidade: {
      ...a.qualidade,
      gps: {
        periodo_mediana_s: null,
        cobertura_pct: 0,
        n_falhas: null,
        maior_falha_s: null,
        distancia_km: null,
        diferenca_distancia_pct: null,
      },
    },
  };
}

/** Removes positions in two windows (8 s and 13 s), creating GPS gaps. */
function gpsWithGaps(a: TripAnalysis): TripAnalysis {
  const route = a.rota;
  if (!route) return a;
  const start = a.viagem.inicio_t_ms ?? 0;
  const windows: [number, number][] = [
    [start + 62_000, start + 70_000],
    [start + 300_000, start + 313_000],
  ];
  const kept: number[] = [];
  route.t_ms.forEach((t, i) => {
    if (t === null || !windows.some(([x, y]) => t >= x && t <= y)) kept.push(i);
  });
  const segs: TripAnalysis["rota"] extends infer R
    ? R extends { segmentos: infer S }
      ? S
      : never
    : never = {
    velocidade_kmh: [],
    faixa: [],
  };
  for (let k = 0; k < kept.length - 1; k++) {
    const i = kept[k];
    if (kept[k + 1] === i + 1) {
      segs.velocidade_kmh.push(route.segmentos.velocidade_kmh[i]);
      segs.faixa.push(route.segmentos.faixa[i]);
    } else {
      segs.velocidade_kmh.push(null);
      segs.faixa.push("falha_gps");
    }
  }
  const lost = windows.reduce((s, [x, y]) => s + (y - x), 0) / 1000;
  const total = (a.resumo.duracao_s ?? 1) || 1;
  return {
    ...a,
    rota: {
      ...route,
      t_ms: kept.map((i) => route.t_ms[i]),
      lat: kept.map((i) => route.lat[i]),
      lon: kept.map((i) => route.lon[i]),
      segmentos: segs,
    },
    qualidade: {
      ...a.qualidade,
      gps: {
        ...a.qualidade.gps,
        cobertura_pct: Math.round((1 - lost / total) * 1000) / 10,
        n_falhas: 2,
        maior_falha_s: 14,
      },
    },
  };
}

function withoutAbsoluteTime(a: TripAnalysis): TripAnalysis {
  return {
    ...a,
    avisos: [{ codigo: "sem_hora_absoluta" }, ...a.avisos],
    viagem: { ...a.viagem, inicio_em: null, fim_em: null, relogio: null },
  };
}

function pidMissing(a: TripAnalysis): TripAnalysis {
  const nullArray = <T>(xs: T[]): null[] => xs.map(() => null);
  return {
    ...a,
    avisos: [{ codigo: "cobertura_maf_baixa" }, ...a.avisos],
    resumo: { ...a.resumo, combustivel_l: null, km_por_l: null },
    series: {
      ...a.series,
      consumo_l_h: nullArray(a.series.consumo_l_h),
      combustivel_acumulado_l: nullArray(a.series.combustivel_acumulado_l),
    },
    series_lentas: {
      ...a.series_lentas,
      nivel_tanque: {
        t_ms: a.series_lentas.nivel_tanque.t_ms,
        pct: nullArray(a.series_lentas.nivel_tanque.pct),
        mediana_movel_pct: nullArray(a.series_lentas.nivel_tanque.mediana_movel_pct),
      },
    },
    qualidade: { ...a.qualidade, combustivel_cobertura_pct: 0 },
  };
}

/**
 * Builds the analysis for a scenario. `complete` means the trip is past the
 * provisional phase (30 s in the mock).
 */
export function buildAnalysis(
  scenario: ScenarioId,
  e: AnalysisInput,
  complete: boolean,
): TripAnalysis {
  switch (scenario) {
    case "sem_gps":
      return withoutGps(base(e));
    case "gps_falhas":
      return gpsWithGaps(base(e));
    case "sem_hora":
      return withoutAbsoluteTime(base(e, true));
    case "sem_leituras":
      return emptied(base(e), [{ codigo: "sem_leituras" }]);
    case "provisoria_aguardando":
      return complete ? base(e) : emptied(base(e), [{ codigo: "sem_leituras" }]);
    case "pid_ausente":
      return pidMissing(base(e));
    case "dados_parciais": {
      const a = base(e);
      return complete ? a : { ...a, avisos: [{ codigo: "dados_parciais" }, ...a.avisos] };
    }
    case "intervalo":
      return intervalScenario(e);
    case "normal":
    default:
      return base(e);
  }
}
