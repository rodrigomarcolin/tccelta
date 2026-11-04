import type {
  Analise,
  Aviso,
  CombustivelId,
  Entrada,
  Evento,
  FaixaId,
  FaixaSegmento,
  FaixaVelocidade,
  Parada,
  Parte,
  Troca,
} from "@/contrato/tipos";
import { faixaDeVelocidade, ORDEM_FAIXAS } from "@/formato/faixas";

/**
 * Gerador de percurso sintético e determinístico. Não faz parte do produto:
 * só alimenta o mock do FE1. Os números seguem as regras da spec 02 de forma
 * simplificada, mas coerente (arrays paralelos, faixas somando 100%).
 */

export const PRESETS: Record<CombustivelId, { afr: number; densidade: number }> = {
  gasolina_e30: { afr: 13.0, densidade: 750 },
  gasolina_e32: { afr: 13.0, densidade: 750 },
  gasolina_e27: { afr: 13.2, densidade: 754 },
  gasolina_pura: { afr: 14.7, densidade: 745 },
  etanol_hidratado: { afr: 9.0, densidade: 809 },
  diesel: { afr: 14.5, densidade: 832 },
};

const LAT0 = -25.43;
const LON0 = -49.27;
const IDLE = 800;
const K_MARCHA = [100, 57, 38, 28, 21]; // rpm por km/h de cada marcha
const SOBE_KMH = [0, 14, 28, 48, 70];

/** Perfil de velocidade (s, km/h, tipo do trecho que termina neste ponto). */
const PERFIL: [number, number, "l" | "s"][] = [
  [0, 0, "s"],
  [22, 0, "s"],
  [40, 35, "s"],
  [70, 50, "s"],
  [100, 50, "s"],
  [103, 95, "l"], // aceleração brusca
  [130, 95, "s"],
  [150, 112, "s"],
  [175, 112, "s"],
  [177.2, 70, "l"], // frenagem brusca
  [200, 60, "s"],
  [215, 0, "s"],
  [235, 0, "s"], // parada
  [255, 40, "s"],
  [290, 60, "s"],
  [330, 85, "s"],
  [380, 85, "s"],
  [400, 45, "s"],
  [420, 20, "s"],
  [440, 0, "s"],
  [500, 0, "s"], // marcha lenta longa
];

const EMBREAGEM: [number, number][] = [
  [205, 213],
  [410, 418],
];

export const DURACAO_PERFIL_S = 500;

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

function velocidadePerfil(s: number): number {
  if (s <= 0) return 0;
  for (let i = 1; i < PERFIL.length; i++) {
    const [t1, v1, tipo] = PERFIL[i];
    const [t0, v0] = PERFIL[i - 1];
    if (s <= t1) {
      const f = (s - t0) / (t1 - t0);
      const g = tipo === "l" ? f : f * f * (3 - 2 * f);
      return v0 + (v1 - v0) * g;
    }
  }
  return PERFIL[PERFIL.length - 1][1];
}

const arred = (v: number, c: number) => Math.round(v * 10 ** c) / 10 ** c;
const clamp = (v: number, a: number, b: number) => Math.min(b, Math.max(a, v));

function mediana(xs: number[]): number | null {
  if (xs.length === 0) return null;
  const s = [...xs].sort((a, b) => a - b);
  const m = s.length >> 1;
  return s.length % 2 ? s[m] : (s[m - 1] + s[m]) / 2;
}

function quantil(xs: number[], q: number): number | null {
  if (xs.length === 0) return null;
  const s = [...xs].sort((a, b) => a - b);
  const pos = (s.length - 1) * q;
  const lo = Math.floor(pos);
  const hi = Math.ceil(pos);
  return s[lo] + (s[hi] - s[lo]) * (pos - lo);
}

export type ParteSpec = {
  bootId: number;
  /** Início da janela da parte no eixo t_ms da viagem. */
  inicioMs: number;
  /** Trecho do perfil (s) usado nesta parte. */
  perfilDe: number;
  perfilAte: number;
  semente: number;
};

export type OpcoesGeracao = {
  entrada: Entrada;
  motorista: { id: string; nome: string };
  categoria: string;
  combustivel: CombustivelId;
  partes: ParteSpec[];
  /** Hora absoluta (epoch ms) correspondente a t_ms = 0; null = sem hora. */
  deslocamentoMs: number | null;
  passoMs?: number;
};

type Ponto = {
  t: number;
  parte: number;
  v: number;
  rpm: number;
  marcha: number | null;
  pedal: number;
  carga: number;
  lh: number;
  a: number | null;
};

/** Vetor de pontos de uma parte (grade única). */
function gerarParte(spec: ParteSpec, indice: number, passo: number): Ponto[] {
  const rnd = mulberry32(spec.semente);
  const n = Math.floor(((spec.perfilAte - spec.perfilDe) * 1000) / passo) + 1;
  const pts: Ponto[] = [];
  let marchaAtual = 1;
  let ultimaTroca = -1e9;
  for (let k = 0; k < n; k++) {
    const sPerfil = spec.perfilDe + (k * passo) / 1000;
    let v = velocidadePerfil(sPerfil);
    if (v > 3) v += (rnd() - 0.5) * 0.9;
    v = Math.max(0, Math.round(v));
    const t = spec.inicioMs + k * passo;
    const embr = EMBREAGEM.some(([a, b]) => sPerfil >= a && sPerfil <= b);

    // Marcha com histerese.
    let alvo = 1;
    for (let g = 0; g < SOBE_KMH.length; g++) {
      if (v >= SOBE_KMH[g]) alvo = g + 1;
    }
    if (alvo < marchaAtual && v >= SOBE_KMH[marchaAtual - 1] - 4) alvo = marchaAtual;
    let marcha: number | null = marchaAtual;
    if (alvo !== marchaAtual) {
      marchaAtual = alvo;
      ultimaTroca = t;
    }
    marcha = marchaAtual;
    let rpm = Math.max(IDLE, K_MARCHA[marchaAtual - 1] * v) * (1 + (rnd() - 0.5) * 0.03);
    if (t - ultimaTroca < 2 * passo && t >= ultimaTroca) {
      marcha = null; // troca em curso: marcha indefinida
      rpm = Math.max(IDLE, rpm * 0.82);
    }
    if (v < 8) {
      marcha = v < 3 ? null : marcha;
    }
    if (embr && v >= 8) {
      marcha = 0;
      rpm = IDLE + 60 + rnd() * 50;
    }
    if (v < 3) rpm = IDLE + (rnd() - 0.5) * 30;
    pts.push({
      t,
      parte: indice,
      v,
      rpm: Math.round(rpm),
      marcha,
      pedal: 0,
      carga: 0,
      lh: 0,
      a: null,
    });
  }

  // Aceleração (diferença central de ±0,5 s).
  const h = Math.max(1, Math.round(500 / passo));
  const dt = passo / 1000;
  for (let i = 0; i < pts.length; i++) {
    if (i - h >= 0 && i + h < pts.length) {
      pts[i].a = arred((pts[i + h].v - pts[i - h].v) / 3.6 / (2 * h * dt), 3);
    }
  }
  for (const p of pts) {
    const a = p.a ?? 0;
    let pedal = p.v < 1 ? 0 : 6 + 0.17 * p.v + 16 * Math.max(a, 0) + (rnd() - 0.5) * 3;
    if (a < -0.3) pedal = 0;
    if (p.v < 1) pedal = 0;
    p.pedal = arred(clamp(pedal, 0, 100), 1);
    p.carga = arred(clamp(14 + 0.65 * p.pedal + p.rpm / 400, 8, 100), 1);
    let maf = 1.5 + (p.rpm / 1000) * (p.carga / 100) * 14;
    if (p.pedal === 0 && p.rpm > 1200 && p.v > 15) maf = 1.5;
    p.lh = maf; // provisório: g/s; convertido depois
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

function temperaturaEm(sDesdeInicio: number): number {
  return 90 - 42 * Math.exp(-sDesdeInicio / 90);
}

export function gerarAnalise(op: OpcoesGeracao): Analise {
  const passo = op.passoMs ?? 250;
  const dt = passo / 1000;
  const { afr, densidade } = PRESETS[op.combustivel];

  const partesPts = op.partes.map((p, i) => gerarParte(p, i, passo));
  for (const pts of partesPts)
    for (const p of pts) p.lh = arred((p.lh / afr / densidade) * 3600, 3);
  const todos = partesPts.flat();

  // ---- Resumo -----------------------------------------------------------
  const distancia = todos.reduce((s, p) => s + (p.v * dt) / 3600, 0);
  const duracao = todos.length * dt;
  const inicioViagem = op.partes[0].inicioMs;
  const ultimaParte = partesPts[partesPts.length - 1];
  const fimViagem = ultimaParte[ultimaParte.length - 1].t + passo;
  const semDados = (fimViagem - inicioViagem) / 1000 - duracao;
  const mov = todos.filter((p) => p.v >= 3);
  const parados = todos.filter((p) => p.v < 3);
  const ligados = todos.filter((p) => p.rpm >= 300);
  const lenta = todos.filter((p) => p.v < 3 && p.rpm >= 300);
  const litros = todos.reduce((s, p) => s + (p.lh * dt) / 3600, 0);

  // Faixas.
  const acum: Record<FaixaId, { t: number; d: number }> = {
    parado: { t: 0, d: 0 },
    ate_30: { t: 0, d: 0 },
    "30_60": { t: 0, d: 0 },
    "60_80": { t: 0, d: 0 },
    "80_110": { t: 0, d: 0 },
    acima_110: { t: 0, d: 0 },
  };
  for (const p of todos) {
    const f = faixaDeVelocidade(p.v);
    acum[f].t += dt;
    acum[f].d += (p.v * dt) / 3600;
  }
  const distMov = ORDEM_FAIXAS.filter((f) => f !== "parado").reduce((s, f) => s + acum[f].d, 0);
  const limites: Record<FaixaId, [number, number | null]> = {
    parado: [0, 3],
    ate_30: [3, 30],
    "30_60": [30, 60],
    "60_80": [60, 80],
    "80_110": [80, 110],
    acima_110: [110, null],
  };
  const faixas: FaixaVelocidade[] = ORDEM_FAIXAS.map((id) => ({
    id,
    de_kmh: limites[id][0],
    ate_kmh: limites[id][1],
    tempo_s: arred(acum[id].t, 2),
    distancia_km: arred(acum[id].d, 4),
    distancia_pct: id === "parado" || distMov === 0 ? null : arred((acum[id].d / distMov) * 100, 2),
  }));

  // ---- Eventos ----------------------------------------------------------
  const eventos: Evento[] = [];
  const paradas: Parada[] = [];
  const posicaoMaisProxima = (t: number, rota: { t: number; lat: number; lon: number }[]) => {
    let melhor: { t: number; lat: number; lon: number } | null = null;
    let d = Infinity;
    for (const r of rota) {
      const x = Math.abs(r.t - t);
      if (x < d) {
        d = x;
        melhor = r;
      }
    }
    return melhor && d <= 5000 ? melhor : null;
  };

  // Posições de GPS (1 Hz), integradas da velocidade.
  const posicoes: { t: number; lat: number; lon: number; parte: number }[] = [];
  {
    let x = 0;
    let y = 0;
    partesPts.forEach((pts, ip) => {
      if (ip > 0) {
        x += 350;
        y -= 120;
      }
      let proximo = pts[0].t;
      for (let i = 0; i < pts.length; i++) {
        const p = pts[i];
        const s = (p.t - pts[0].t) / 1000 + op.partes[ip].perfilDe;
        const rumo =
          0.6 +
          0.35 * Math.sin(s / 45) +
          (s > 60 ? 0.9 * smooth((s - 60) / 8) : 0) -
          (s > 250 ? 1.2 * smooth((s - 250) / 10) : 0) +
          (s > 340 ? 0.8 * smooth((s - 340) / 10) : 0);
        const d = (p.v / 3.6) * dt;
        x += d * Math.cos(rumo);
        y += d * Math.sin(rumo);
        if (p.t >= proximo) {
          proximo += 1000;
          posicoes.push({
            t: p.t,
            lat: arred(LAT0 + y / 111320, 6),
            lon: arred(LON0 + x / (111320 * Math.cos((LAT0 * Math.PI) / 180)), 6),
            parte: ip,
          });
        }
      }
    });
  }

  const emParte = (ip: number) => partesPts[ip];
  partesPts.forEach((_, ip) => {
    const pts = emParte(ip);
    const detectar = (
      tipo: "aceleracao_brusca" | "frenagem_brusca",
      cond: (a: number) => boolean,
      pico: (xs: number[]) => number,
    ) => {
      let i = 0;
      while (i < pts.length) {
        if (pts[i].a !== null && cond(pts[i].a as number)) {
          let j = i;
          while (j + 1 < pts.length && pts[j + 1].a !== null && cond(pts[j + 1].a as number)) j++;
          const dur = (pts[j].t - pts[i].t + passo) / 1000;
          if (dur * 1000 >= 500) {
            const pos = posicaoMaisProxima(pts[i].t, posicoes);
            eventos.push({
              tipo,
              t_inicio_ms: pts[i].t,
              duracao_s: arred(dur, 2),
              pico: arred(pico(pts.slice(i, j + 1).map((p) => p.a as number)), 2),
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
    detectar(
      "aceleracao_brusca",
      (a) => a >= 3.0,
      (xs) => Math.max(...xs),
    );
    detectar(
      "frenagem_brusca",
      (a) => a <= -3.5,
      (xs) => Math.min(...xs),
    );

    // Paradas e marcha lenta longa.
    let i = 0;
    while (i < pts.length) {
      if (pts[i].v < 3) {
        let j = i;
        while (j + 1 < pts.length && pts[j + 1].v < 3) j++;
        const dur = (pts[j].t - pts[i].t + passo) / 1000;
        const ligado = pts.slice(i, j + 1).every((p) => p.rpm >= 300);
        if (dur >= 5) {
          const pos = posicaoMaisProxima(pts[i].t, posicoes);
          paradas.push({
            t_inicio_ms: pts[i].t,
            duracao_s: arred(dur, 2),
            motor_ligado: ligado,
            lat: pos?.lat ?? null,
            lon: pos?.lon ?? null,
          });
          if (ligado && dur * 1000 >= 60000) {
            eventos.push({
              tipo: "marcha_lenta_longa",
              t_inicio_ms: pts[i].t,
              duracao_s: arred(dur, 2),
              pico: arred(dur, 2),
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
  eventos.sort((a, b) => a.t_inicio_ms - b.t_inicio_ms);
  paradas.sort((a, b) => a.t_inicio_ms - b.t_inicio_ms);

  // ---- Rota -------------------------------------------------------------
  const rotaT = posicoes.map((p) => p.t);
  const velNoIntervalo = (t0: number, t1: number, ip: number): number | null => {
    const xs = emParte(ip)
      .filter((p) => p.t >= t0 && p.t < t1)
      .map((p) => p.v);
    return xs.length ? xs.reduce((a, b) => a + b, 0) / xs.length : null;
  };
  const segVel: (number | null)[] = [];
  const segFaixa: FaixaSegmento[] = [];
  for (let i = 0; i < posicoes.length - 1; i++) {
    const a = posicoes[i];
    const b = posicoes[i + 1];
    if (a.parte !== b.parte || b.t - a.t > 5000) {
      segVel.push(null);
      segFaixa.push("falha_gps");
      continue;
    }
    const v = velNoIntervalo(a.t, b.t, a.parte);
    segVel.push(v === null ? null : arred(v, 1));
    segFaixa.push(v === null ? "sem_velocidade" : faixaDeVelocidade(v));
  }

  // ---- Marchas ----------------------------------------------------------
  const trocas: Troca[] = [];
  partesPts.forEach((pts) => {
    let ultimo: { g: number; fimT: number; rpm: number; v: number } | null = null;
    for (let i = 0; i < pts.length; i++) {
      const g = pts[i].marcha;
      if (g === null || g === 0) continue;
      if (ultimo && g !== ultimo.g && pts[i].t - ultimo.fimT <= 4000) {
        trocas.push({
          t_ms: ultimo.fimT,
          direcao: g > ultimo.g ? "subida" : "reducao",
          de: ultimo.g,
          para: g,
          rpm_antes: ultimo.rpm,
          rpm_depois: pts[i].rpm,
          velocidade_kmh: ultimo.v,
        });
      }
      ultimo = { g, fimT: pts[i].t, rpm: pts[i].rpm, v: pts[i].v };
    }
  });
  const subidas = trocas.filter((t) => t.direcao === "subida").map((t) => t.rpm_antes);
  const reducoes = trocas.filter((t) => t.direcao === "reducao").map((t) => t.rpm_antes);
  const gears = [...new Set(todos.map((p) => p.marcha).filter((g): g is number => !!g))].sort(
    (a, b) => a - b,
  );
  const tempoMov = mov.length * dt;
  const patamares = gears.map((g) => {
    const ps = mov.filter((p) => p.marcha === g);
    return {
      marcha: g,
      rpm_por_kmh: arred(mediana(ps.map((p) => p.rpm / Math.max(1, p.v))) ?? 0, 1),
      tempo_s: arred(ps.length * dt, 2),
      tempo_pct: arred(tempoMov ? ((ps.length * dt) / tempoMov) * 100 : 0, 2),
    };
  });
  const neutro = todos.filter((p) => p.marcha === 0).length * dt;
  const rpmLenta = mediana(lenta.map((p) => p.rpm));

  // ---- Distribuições ----------------------------------------------------
  const tempoLigado = ligados.length * dt;
  const cortesRpm = [0, 1000, 1500, 2000, 2500, 3000, 3500, 4000, 5000];
  const rotacao = cortesRpm.map((de, i) => {
    const ate = i + 1 < cortesRpm.length ? cortesRpm[i + 1] : null;
    const n = ligados.filter((p) => p.rpm >= de && (ate === null || p.rpm < ate)).length;
    return {
      de,
      ate,
      tempo_s: arred(n * dt, 2),
      pct: arred(tempoLigado ? ((n * dt) / tempoLigado) * 100 : 0, 2),
    };
  });
  const cortesPedal = [0, 5, 20, 40, 60, 80];
  const pedal = cortesPedal.map((de, i) => {
    const ate = i + 1 < cortesPedal.length ? cortesPedal[i + 1] : 100;
    const n = ligados.filter((p) => p.pedal >= de && (ate === 100 ? true : p.pedal < ate)).length;
    return {
      de,
      ate,
      ...(i === 0 ? { rotulo: "Solto" } : {}),
      tempo_s: arred(n * dt, 2),
      pct: arred(tempoLigado ? ((n * dt) / tempoLigado) * 100 : 0, 2),
    };
  });
  // Linhas = faixas de rotação (500 rpm); colunas = faixas de pedal (10%).
  const nRpm = 12;
  const nPedal = 10;
  const grade: number[][] = Array.from({ length: nRpm }, () => Array<number>(nPedal).fill(0));
  for (const p of ligados) {
    const r = Math.min(nRpm - 1, Math.floor(p.rpm / 500));
    const c = Math.min(nPedal - 1, Math.floor(p.pedal / 10));
    grade[r][c] += dt;
  }

  // ---- Condutor ---------------------------------------------------------
  const variacoes: number[] = [];
  partesPts.forEach((pts) => {
    for (let i = 1; i < pts.length; i++)
      variacoes.push(Math.abs(pts[i].pedal - pts[i - 1].pedal) / dt);
  });
  const freio = mov.filter((p) => p.pedal <= 3 && p.rpm >= 1200 && p.v >= 15).length;
  const comAcel = mov.filter((p) => p.a !== null).map((p) => p.a as number);
  const rms = comAcel.length
    ? Math.sqrt(comAcel.reduce((s, a) => s + a * a, 0) / comAcel.length)
    : null;
  let cruzeiro = 0;
  partesPts.forEach((pts) => {
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
      const dur = (pts[j].t - pts[i].t + passo) / 1000;
      if (dur >= 30) {
        cruzeiro += dur;
        i = j + 1;
      } else i++;
    }
  });
  const pedalAlto = ligados.filter((p) => p.pedal >= 80).length * dt;

  // ---- Séries -----------------------------------------------------------
  const serie = {
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
  partesPts.forEach((pts, ip) => {
    for (const p of pts) {
      accL += (p.lh * dt) / 3600;
      accD += (p.v * dt) / 3600;
      serie.t_ms.push(p.t);
      serie.velocidade_kmh.push(p.v);
      serie.rpm.push(p.rpm);
      serie.rpm_por_kmh.push(p.v >= 8 ? arred(p.rpm / p.v, 2) : null);
      serie.marcha.push(p.marcha);
      serie.pedal_pct.push(p.pedal);
      serie.aceleracao_ms2.push(p.a);
      serie.carga_pct.push(p.carga);
      serie.consumo_l_h.push(p.lh);
      serie.combustivel_acumulado_l.push(arred(accL, 5));
      serie.distancia_acumulada_km.push(arred(accD, 4));
    }
    if (ip < partesPts.length - 1) {
      const ultimo = pts[pts.length - 1].t;
      // Separador entre partes: instante logo após o fim e valores null.
      serie.t_ms.push(ultimo + passo);
      for (const k of Object.keys(serie) as (keyof typeof serie)[]) {
        if (k !== "t_ms") serie[k].push(null);
      }
    }
  });

  const lentoT: (number | null)[] = [];
  const lentoC: (number | null)[] = [];
  const tanqueT: (number | null)[] = [];
  const tanquePct: (number | null)[] = [];
  const tanqueMed: (number | null)[] = [];
  partesPts.forEach((pts, ip) => {
    const ini = pts[0].t;
    const fim = pts[pts.length - 1].t;
    const rnd = mulberry32(op.partes[ip].semente + 99);
    const nivel: number[] = [];
    for (let t = ini; t <= fim; t += 5000) {
      lentoT.push(t);
      lentoC.push(Math.round(temperaturaEm((t - op.partes[0].inicioMs) / 1000 + (ip ? 600 : 0))));
    }
    for (let t = ini; t <= fim; t += 10000) {
      tanqueT.push(t);
      nivel.push(arred(62 - ((t - ini) / 1000) * 0.004 + (rnd() - 0.5) * 1.2, 1));
    }
    nivel.forEach((v, i) => {
      tanquePct.push(v);
      const janela = nivel.slice(Math.max(0, i - 3), i + 4);
      tanqueMed.push(arred(mediana(janela) ?? v, 1));
    });
    if (ip < partesPts.length - 1) {
      lentoT.push(fim + 1000);
      lentoC.push(null);
      tanqueT.push(fim + 1000);
      tanquePct.push(null);
      tanqueMed.push(null);
    }
  });

  // ---- Motor ------------------------------------------------------------
  const t0 = op.partes[0].inicioMs;
  const aquecimento = Math.ceil(90 * Math.log(42 / 10));
  const frio = todos.filter(
    (p) => p.rpm >= 300 && temperaturaEm((p.t - t0) / 1000) < 60 && (p.rpm > 3000 || p.pedal > 60),
  ).length;

  // ---- Qualidade --------------------------------------------------------
  let distGps = 0;
  let falhas = 0;
  for (let i = 0; i < posicoes.length - 1; i++) {
    const a = posicoes[i];
    const b = posicoes[i + 1];
    if (a.parte !== b.parte || b.t - a.t > 5000) falhas += a.parte !== b.parte ? 0 : 1;
    else distGps += haversineKm(a.lat, a.lon, b.lat, b.lon);
  }
  const leituras = Math.round(
    (duracao / (passo / 1000)) * 3 + (duracao / 0.3) * 2 + duracao / 5 + duracao / 10,
  );

  // ---- Viagem -----------------------------------------------------------
  const partes: Parte[] = op.partes.map((p, i) => {
    const pts = partesPts[i];
    return {
      boot_generation: 0,
      boot_id: p.bootId,
      dados_inicio_t_ms: p.inicioMs - 2600,
      dados_fim_t_ms: pts[pts.length - 1].t + 400,
      inicio_t_ms: p.inicioMs,
      fim_t_ms: pts[pts.length - 1].t + passo,
    };
  });
  const iso = (ms: number) => new Date(ms).toISOString();
  const avisos: Aviso[] = [];
  const pedalFracao = tempoLigado ? pedalAlto / tempoLigado : 0;
  if (pedalFracao > 0.1) avisos.push({ codigo: "consumo_pode_estar_subestimado" });

  const analise: Analise = {
    versao_calculo: "1",

    viagem: {
      entrada: op.entrada,
      motorista: op.motorista,
      categoria_viagem: op.categoria,
      inicio_t_ms: inicioViagem,
      fim_t_ms: fimViagem,
      inicio_em: op.deslocamentoMs === null ? null : iso(op.deslocamentoMs + inicioViagem),
      fim_em: op.deslocamentoMs === null ? null : iso(op.deslocamentoMs + fimViagem),
      relogio:
        op.deslocamentoMs === null
          ? null
          : { deslocamento_ms: op.deslocamentoMs, incerteza_ms: 180 },
      partes,
    },
    parametros: {
      combustivel: op.combustivel,
      afr,
      densidade_g_l: densidade,
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
    avisos,
    resumo: {
      distancia_km: arred(distancia, 3),
      duracao_s: arred(duracao, 1),
      sem_dados_s: arred(semDados, 1),
      em_movimento_s: arred(tempoMov, 1),
      parado_s: arred(parados.length * dt, 1),
      velocidade_media_kmh: arred(mov.reduce((s, p) => s + p.v, 0) / Math.max(1, mov.length), 1),
      velocidade_maxima_kmh: Math.max(...todos.map((p) => p.v)),
      motor_ligado_s: arred(tempoLigado, 1),
      marcha_lenta_s: arred(lenta.length * dt, 1),
      marcha_lenta_pct: arred(tempoLigado ? ((lenta.length * dt) / tempoLigado) * 100 : 0, 1),
      combustivel_l: arred(litros, 3),
      km_por_l: litros > 0.02 ? arred(distancia / litros, 2) : null,
      n_acel_bruscas: eventos.filter((e) => e.tipo === "aceleracao_brusca").length,
      n_fren_bruscas: eventos.filter((e) => e.tipo === "frenagem_brusca").length,
      acel_por_100km: null,
      fren_por_100km: null,
    },
    condutor: {
      pedal_medio_pct: arred(mov.reduce((s, p) => s + p.pedal, 0) / Math.max(1, mov.length), 1),
      pedal_alto_s: arred(pedalAlto, 1),
      pedal_variacao_p95_pct_s: arred(quantil(variacoes, 0.95) ?? 0, 1),
      freio_motor_pct: arred(mov.length ? (freio / mov.length) * 100 : 0, 1),
      aceleracao_rms_ms2: rms === null ? null : arred(rms, 2),
      cruzeiro_s: arred(cruzeiro, 1),
    },
    motor: {
      aquecimento_s: aquecimento,
      temperatura_maxima_c: Math.max(...(lentoC.filter((c) => c !== null) as number[])),
      esforco_frio_s: arred(frio * dt, 1),
    },
    faixas_velocidade: faixas,
    rota: {
      t_ms: rotaT,
      lat: posicoes.map((p) => p.lat),
      lon: posicoes.map((p) => p.lon),
      segmentos: { velocidade_kmh: segVel, faixa: segFaixa },
    },
    paradas,
    eventos,
    marchas: {
      marcha_lenta: { rpm: Math.round(rpmLenta ?? 750), origem: "estimada", margem_rpm: 150 },
      patamares,
      ponto_morto_embreagem_s: arred(neutro, 1),
      trocas,
      subida_rpm: {
        mediana: subidas.length ? Math.round(mediana(subidas) as number) : null,
        p25: subidas.length ? Math.round(quantil(subidas, 0.25) as number) : null,
        p75: subidas.length ? Math.round(quantil(subidas, 0.75) as number) : null,
        n: subidas.length,
      },
      reducao_rpm: {
        mediana: reducoes.length ? Math.round(mediana(reducoes) as number) : null,
        n: reducoes.length,
      },
    },
    distribuicoes: {
      rotacao,
      pedal,
      rotacao_pedal: {
        rpm_bin: 500,
        pedal_bin: 10,
        tempo_s: grade.map((l) => l.map((v) => arred(v, 2))),
      },
    },
    series: {
      passo_ms: passo,
      exposicao_passo_ms: passo,
      ...serie,
    },
    series_lentas: {
      temperatura: { t_ms: lentoT, c: lentoC },
      nivel_tanque: { t_ms: tanqueT, pct: tanquePct, mediana_movel_pct: tanqueMed },
    },
    qualidade: {
      leituras_total: leituras,
      leituras_por_s: arred(leituras / Math.max(1, duracao), 2),
      periodos_mediana_ms: {
        "0x0C": passo,
        "0x0D": passo,
        "0x49": passo,
        "0x04": 300,
        "0x10": 300,
        "0x05": 5000,
        "0x2F": 10000,
      },
      combustivel_cobertura_pct: 99.8,
      gps: {
        periodo_mediana_s: 1,
        cobertura_pct: 100,
        n_falhas: falhas,
        maior_falha_s: 0,
        distancia_km: arred(distGps, 3),
        diferenca_distancia_pct:
          distancia > 0 ? arred(((distGps - distancia) / distancia) * 100, 2) : null,
      },
    },
  };
  return analise;
}

function smooth(f: number): number {
  const x = clamp(f, 0, 1);
  return x * x * (3 - 2 * x);
}
