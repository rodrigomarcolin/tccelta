import type { Analise, CombustivelId, Entrada } from "@/contrato/tipos";
import type { CenarioId } from "./cenarios";
import { gerarAnalise, type ParteSpec } from "./gerador";

export type EntradaAnalise = {
  entrada: Entrada;
  motorista: { id: string; nome: string };
  categoria: string;
  combustivel: CombustivelId;
};

const BOOT_INICIO_MS = 33_481;
const BOOT_HORA = Date.parse("2026-10-28T14:03:22-03:00");

function base(e: EntradaAnalise, semHora = false): Analise {
  const boot = e.entrada.tipo === "boot" ? e.entrada.boot_id : 8;
  return gerarAnalise({
    ...e,
    partes: [
      { bootId: boot, inicioMs: BOOT_INICIO_MS, perfilDe: 0, perfilAte: 500, semente: 1000 + boot },
    ],
    deslocamentoMs: semHora ? null : BOOT_HORA - BOOT_INICIO_MS,
  });
}

function intervalo(e: EntradaAnalise): Analise {
  const t0 = e.entrada.tipo === "intervalo" ? Date.parse(e.entrada.t0) : BOOT_HORA;
  const t1 = e.entrada.tipo === "intervalo" ? Date.parse(e.entrada.t1) : t0 + 36_000_000;
  const vao = t1 - t0;
  const umaParte = 260_000;
  const inicio1 = Math.round(vao * 0.1);
  const inicio2 = Math.max(Math.round(vao * 0.6), inicio1 + umaParte + 30_000);
  const partes: ParteSpec[] = [
    { bootId: 8, inicioMs: inicio1, perfilDe: 0, perfilAte: 260, semente: 2008 },
    { bootId: 9, inicioMs: inicio2, perfilDe: 300, perfilAte: 500, semente: 2009 },
  ];
  return gerarAnalise({ ...e, partes, deslocamentoMs: t0 });
}

/** Esvazia a análise: nada foi lido. Segue a regra "ausência é null". */
function vazia(a: Analise, avisos: Analise["avisos"]): Analise {
  const nulos = <T extends object>(o: T): T =>
    Object.fromEntries(Object.keys(o).map((k) => [k, null])) as T;
  return {
    ...a,
    avisos,
    viagem: {
      ...a.viagem,
      inicio_t_ms: null,
      fim_t_ms: null,
      inicio_em: null,
      fim_em: null,
      partes: [],
    },
    resumo: nulos(a.resumo),
    condutor: nulos(a.condutor),
    motor: nulos(a.motor),
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

function semGps(a: Analise): Analise {
  const nulaPos = <T extends { lat: number | null; lon: number | null }>(x: T): T => ({
    ...x,
    lat: null,
    lon: null,
  });
  return {
    ...a,
    avisos: [{ codigo: "sem_gps" }, ...a.avisos],
    rota: null,
    paradas: a.paradas.map(nulaPos),
    eventos: a.eventos.map(nulaPos),
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

/** Remove posições em duas janelas (8 s e 13 s), criando falhas de GPS. */
function gpsComFalhas(a: Analise): Analise {
  const rota = a.rota;
  if (!rota) return a;
  const ini = a.viagem.inicio_t_ms ?? 0;
  const janelas: [number, number][] = [
    [ini + 62_000, ini + 70_000],
    [ini + 300_000, ini + 313_000],
  ];
  const mantidos: number[] = [];
  rota.t_ms.forEach((t, i) => {
    if (t === null || !janelas.some(([x, y]) => t >= x && t <= y)) mantidos.push(i);
  });
  const faixa: Analise["rota"] extends infer R
    ? R extends { segmentos: infer S }
      ? S
      : never
    : never = {
    velocidade_kmh: [],
    faixa: [],
  };
  for (let k = 0; k < mantidos.length - 1; k++) {
    const i = mantidos[k];
    if (mantidos[k + 1] === i + 1) {
      faixa.velocidade_kmh.push(rota.segmentos.velocidade_kmh[i]);
      faixa.faixa.push(rota.segmentos.faixa[i]);
    } else {
      faixa.velocidade_kmh.push(null);
      faixa.faixa.push("falha_gps");
    }
  }
  const perda = janelas.reduce((s, [x, y]) => s + (y - x), 0) / 1000;
  const total = (a.resumo.duracao_s ?? 1) || 1;
  return {
    ...a,
    rota: {
      ...rota,
      t_ms: mantidos.map((i) => rota.t_ms[i]),
      lat: mantidos.map((i) => rota.lat[i]),
      lon: mantidos.map((i) => rota.lon[i]),
      segmentos: faixa,
    },
    qualidade: {
      ...a.qualidade,
      gps: {
        ...a.qualidade.gps,
        cobertura_pct: Math.round((1 - perda / total) * 1000) / 10,
        n_falhas: 2,
        maior_falha_s: 14,
      },
    },
  };
}

function semHoraAbsoluta(a: Analise): Analise {
  return {
    ...a,
    avisos: [{ codigo: "sem_hora_absoluta" }, ...a.avisos],
    viagem: { ...a.viagem, inicio_em: null, fim_em: null, relogio: null },
  };
}

function pidAusente(a: Analise): Analise {
  const nulo = <T>(xs: T[]): null[] => xs.map(() => null);
  return {
    ...a,
    avisos: [{ codigo: "cobertura_maf_baixa" }, ...a.avisos],
    resumo: { ...a.resumo, combustivel_l: null, km_por_l: null },
    series: {
      ...a.series,
      consumo_l_h: nulo(a.series.consumo_l_h),
      combustivel_acumulado_l: nulo(a.series.combustivel_acumulado_l),
    },
    series_lentas: {
      ...a.series_lentas,
      nivel_tanque: {
        t_ms: a.series_lentas.nivel_tanque.t_ms,
        pct: nulo(a.series_lentas.nivel_tanque.pct),
        mediana_movel_pct: nulo(a.series_lentas.nivel_tanque.mediana_movel_pct),
      },
    },
    qualidade: { ...a.qualidade, combustivel_cobertura_pct: 0 },
  };
}

/**
 * Monta a análise de um cenário. `completa` indica que a viagem já passou da
 * fase provisória (30 s no mock).
 */
export function construirAnalise(
  cenario: CenarioId,
  e: EntradaAnalise,
  completa: boolean,
): Analise {
  switch (cenario) {
    case "sem_gps":
      return semGps(base(e));
    case "gps_falhas":
      return gpsComFalhas(base(e));
    case "sem_hora":
      return semHoraAbsoluta(base(e, true));
    case "sem_leituras":
      return vazia(base(e), [{ codigo: "sem_leituras" }]);
    case "provisoria_aguardando":
      return completa ? base(e) : vazia(base(e), [{ codigo: "sem_leituras" }]);
    case "pid_ausente":
      return pidAusente(base(e));
    case "dados_parciais": {
      const a = base(e);
      return completa ? a : { ...a, avisos: [{ codigo: "dados_parciais" }, ...a.avisos] };
    }
    case "intervalo":
      return intervalo(e);
    case "normal":
    default:
      return base(e);
  }
}
