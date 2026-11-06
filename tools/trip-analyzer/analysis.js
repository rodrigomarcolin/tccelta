/* Análise de viagem a partir das leituras OBD-II de um boot (tabela `reading`).
 * Funções puras: sem DOM, carregáveis no navegador (window.TripAnalysis) ou no Node (require). */
(function (root) {
'use strict';

// Espelho da tabela `pid` (migration 00005): usado só quando o CSV traz raw_value sem value.
const PID = { LOAD: 4, COOLANT: 5, RPM: 12, SPEED: 13, MAF: 16, FUEL: 47, PEDAL: 73 };
const PID_INFO = {
  4:  { name: 'Carga do motor',         unit: '%',    factor: 100 / 255, offset: 0,   min: 0,   max: 100 },
  5:  { name: 'Temperatura do líquido', unit: '°C',   factor: 1,         offset: -40, min: -40, max: 215 },
  12: { name: 'Rotação',                unit: 'RPM',  factor: 0.25,      offset: 0,   min: 0,   max: 16383.75 },
  13: { name: 'Velocidade',             unit: 'km/h', factor: 1,         offset: 0,   min: 0,   max: 255 },
  16: { name: 'MAF',                    unit: 'g/s',  factor: 0.01,      offset: 0,   min: 0,   max: 655.35 },
  47: { name: 'Nível de combustível',   unit: '%',    factor: 100 / 255, offset: 0,   min: 0,   max: 100 },
  73: { name: 'Pedal do acelerador',    unit: '%',    factor: 100 / 255, offset: 0,   min: 0,   max: 100 },
};

const FUEL_PRESETS = [
  { id: 'gasoline', label: 'Gasolina pura',     afr: 14.7, density: 745 },
  { id: 'gasoline_c', label: 'Gasolina C (E27)', afr: 13.2, density: 754 },
  { id: 'ethanol',  label: 'Etanol hidratado',  afr: 9.0,  density: 809 },
  { id: 'diesel',   label: 'Diesel',            afr: 14.5, density: 832 },
];

// Limiares das heurísticas. Velocidade OBD é inteira (km/h), por isso as janelas de ~1 s.
const CFG = {
  engineOnRpm: 300,
  movingKmh: 3,
  gearMinKmh: 8,           // abaixo disso RPM/velocidade é dominado por embreagem e arrancada
  gearMinRpm: 500,
  gearTolLn: 0.06,         // amostra pertence ao patamar se |ln k − centro| ≤ 6 %
  gearMinSepLn: 0.09,      // patamares a menos de ~9 % são a mesma marcha
  gearMinDwellMs: 1000,    // trechos menores que isso não contam como marcha
  gearFillMs: 1500,        // buraco curto entre a mesma marcha é preenchido
  shiftMaxGapMs: 4000,     // troca: nova marcha começa até 4 s depois do fim da anterior
  harshAccel: 3.0,         // m/s² (~0,3 g)
  harshBrake: -3.5,        // m/s² (~0,36 g)
  harshMinMs: 500,
  engineBrakeRpm: 1200,
  engineBrakeKmh: 15,
  engineBrakePedal: 3,
  warmC: 80,
  coldC: 60,
  coldRpm: 3000,
  coldPedal: 60,
  longIdleMs: 60000,
};

// ---------- CSV ----------

function splitCsv(text, delim) {
  const rows = [];
  let row = [], field = '', q = false;
  for (let i = 0; i < text.length; i++) {
    const c = text[i];
    if (q) {
      if (c === '"') { if (text[i + 1] === '"') { field += '"'; i++; } else q = false; }
      else field += c;
    } else if (c === '"') q = true;
    else if (c === delim) { row.push(field); field = ''; }
    else if (c === '\n' || c === '\r') {
      if (c === '\r' && text[i + 1] === '\n') i++;
      row.push(field); field = '';
      if (row.length > 1 || row[0] !== '') rows.push(row);
      row = [];
    } else field += c;
  }
  if (field !== '' || row.length) { row.push(field); rows.push(row); }
  return rows;
}

function detectDelimiter(firstLine) {
  let best = ',', n = -1;
  for (const d of [',', ';', '\t', '|']) {
    const c = firstLine.split(d).length;
    if (c > n) { n = c; best = d; }
  }
  return best;
}

/** Número do CSV. Com decimalComma (exportação pt-BR), '.' é milhar e ',' é decimal: "1.560" = 1560, "23,13" = 23.13. */
function parseNumber(s, decimalComma) {
  if (s == null) return NaN;
  s = String(s).trim();
  if (s === '' || /^(null|\\n|nan)$/i.test(s)) return NaN;
  if (/^0x[0-9a-f]+$/i.test(s)) return parseInt(s, 16);
  if (decimalComma) return Number(s.replace(/\./g, '').replace(',', '.'));
  return Number(s);
}

/** Exportações em pt-BR (Excel, DBeaver) usam ';' e vírgula decimal; com ',' como separador não há como haver vírgula decimal sem aspas. */
function detectDecimalComma(table, delim) {
  if (delim === ';') return true;
  for (let r = 1; r < Math.min(table.length, 500); r++) {
    for (const f of table[r]) if (/^-?\d+,\d+$/.test(f.trim())) return true;
  }
  return false;
}

/** CSV → { boots: Map<key, {generation, bootId, rows:[{t, pid, v}]}>, warnings } */
function parseReadingsCsv(text) {
  text = text.replace(/^﻿/, '');
  const nl = text.search(/\r?\n/);
  const delim = detectDelimiter(nl < 0 ? text : text.slice(0, nl));
  const table = splitCsv(text, delim);
  if (!table.length) throw new Error('CSV vazio.');
  const header = table[0].map(h => h.trim().toLowerCase().replace(/^"|"$/g, ''));
  const col = name => header.indexOf(name);
  const iT = col('t_ms'), iPid = col('pid'), iV = col('value'), iRaw = col('raw_value');
  const iBoot = col('boot_id'), iGen = col('boot_generation');
  if (iT < 0 || iPid < 0 || (iV < 0 && iRaw < 0)) {
    throw new Error('O cabeçalho precisa das colunas t_ms, pid e value (ou raw_value). Encontrado: ' + header.join(', '));
  }
  const dc = detectDecimalComma(table, delim);
  const num = x => parseNumber(x, dc);
  const boots = new Map();
  const warnings = [];
  if (dc) warnings.push('CSV em formato pt-BR (vírgula decimal, ponto de milhar).');
  let skippedNull = 0, skippedBad = 0, unknownPid = 0;
  for (let r = 1; r < table.length; r++) {
    const row = table[r];
    const t = num(row[iT]), pid = num(row[iPid]);
    if (!Number.isFinite(t) || !Number.isFinite(pid)) { skippedBad++; continue; }
    let v = iV >= 0 ? num(row[iV]) : NaN;
    if (!Number.isFinite(v) && iV < 0 && iRaw >= 0) {
      const info = PID_INFO[pid], raw = num(row[iRaw]);
      if (info && Number.isFinite(raw)) {
        v = raw * info.factor + info.offset;
        if (v < info.min || v > info.max) v = NaN;
      }
    }
    if (!Number.isFinite(v)) { skippedNull++; continue; }
    if (!PID_INFO[pid]) unknownPid++;
    const gen = iGen >= 0 ? num(row[iGen]) : NaN;
    const boot = iBoot >= 0 ? num(row[iBoot]) : NaN;
    const key = (Number.isFinite(gen) ? gen : '?') + ':' + (Number.isFinite(boot) ? boot : '?');
    let b = boots.get(key);
    if (!b) boots.set(key, b = { key, generation: Number.isFinite(gen) ? gen : null, bootId: Number.isFinite(boot) ? boot : null, rows: [] });
    b.rows.push({ t, pid, v });
  }
  if (skippedNull) warnings.push(`${skippedNull} leituras sem valor (NULL/fora da faixa) ignoradas.`);
  if (skippedBad) warnings.push(`${skippedBad} linhas malformadas ignoradas.`);
  if (unknownPid) warnings.push(`${unknownPid} leituras de PIDs fora da tabela pid.`);
  if (!boots.size) throw new Error('Nenhuma leitura válida no CSV.');
  return { boots, warnings };
}

// ---------- séries ----------

/** rows → { [pid]: {t: number[], v: number[]} } ordenado por t, sem (t, pid) duplicado. */
function toSeries(rows) {
  const by = {};
  for (const r of rows) (by[r.pid] || (by[r.pid] = [])).push(r);
  const out = {};
  for (const pid in by) {
    const arr = by[pid].sort((a, b) => a.t - b.t);
    const t = [], v = [];
    for (const r of arr) {
      if (t.length && t[t.length - 1] === r.t) continue;
      t.push(r.t); v.push(r.v);
    }
    out[pid] = { t, v };
  }
  return out;
}

function maxOf(a) { let m = -Infinity; for (const x of a) if (x != null && x > m) m = x; return m; }
function minOf(a) { let m = Infinity; for (const x of a) if (x != null && x < m) m = x; return m; }

function median(a) {
  if (!a.length) return NaN;
  const s = Float64Array.from(a).sort();
  const m = s.length >> 1;
  return s.length % 2 ? s[m] : (s[m - 1] + s[m]) / 2;
}

function quantile(a, q) {
  if (!a.length) return NaN;
  const s = Float64Array.from(a).sort();
  const p = (s.length - 1) * q, lo = Math.floor(p), hi = Math.ceil(p);
  return s[lo] + (s[hi] - s[lo]) * (p - lo);
}

function medianInterval(ts) {
  const d = [];
  for (let i = 1; i < ts.length; i++) if (ts[i] > ts[i - 1]) d.push(ts[i] - ts[i - 1]);
  return median(d);
}

/** Último valor em ou antes de cada ponto da grade, se não for mais velho que maxAge. */
function resample(s, grid, maxAge) {
  const out = new Array(grid.length).fill(null);
  if (!s) return out;
  let j = 0;
  for (let i = 0; i < grid.length; i++) {
    const g = grid[i];
    while (j + 1 < s.t.length && s.t[j + 1] <= g) j++;
    if (s.t[j] <= g && g - s.t[j] <= maxAge) out[i] = s.v[j];
  }
  return out;
}

// ---------- marchas ----------

function detectGears(grid, step, rpm, speed) {
  const n = grid.length;
  const lnk = new Array(n).fill(null);
  for (let i = 0; i < n; i++) {
    if (speed[i] != null && rpm[i] != null && speed[i] >= CFG.gearMinKmh && rpm[i] >= CFG.gearMinRpm) {
      lnk[i] = Math.log(rpm[i] / speed[i]);
    }
  }
  // a velocidade OBD é inteira: ±1 km/h a 15 km/h já são ~7 % em k, então a tolerância cresce em baixa velocidade
  const quant = i => 1 / speed[i];
  // amostras estáveis: vizinhança de ±0,5 s com ln k dentro de 4 % (+ quantização)
  const h = Math.max(1, Math.round(500 / step));
  const stable = [];
  for (let i = h; i < n - h; i++) {
    if (lnk[i] == null) continue;
    let ok = true;
    const tol = 0.04 + quant(i);
    for (let j = i - h; j <= i + h && ok; j++) ok = lnk[j] != null && Math.abs(lnk[j] - lnk[i]) <= tol;
    if (ok) stable.push(lnk[i]);
  }
  const result = { centers: [], gear: new Array(n).fill(null), lnk, stableCount: stable.length };
  if (stable.length < 20) return result;

  // histograma de ln k suavizado; picos viram patamares
  const bw = 0.005;
  const lo = minOf(stable) - 0.05, hi = maxOf(stable) + 0.05;
  const nb = Math.ceil((hi - lo) / bw) + 1;
  const hist = new Float64Array(nb);
  for (const x of stable) hist[Math.round((x - lo) / bw)]++;
  const sm = new Float64Array(nb), sig = 3;
  for (let b = 0; b < nb; b++) {
    let acc = 0;
    for (let d = -3 * sig; d <= 3 * sig; d++) {
      const k = b + d;
      if (k >= 0 && k < nb) acc += hist[k] * Math.exp(-(d * d) / (2 * sig * sig));
    }
    sm[b] = acc;
  }
  const peak = maxOf(sm);
  const minMass = Math.round(1500 / step);
  const cands = [];
  for (let b = 1; b < nb - 1; b++) {
    if (sm[b] < sm[b - 1] || sm[b] < sm[b + 1] || sm[b] < peak * 0.002) continue;
    const c = lo + b * bw;
    const near = stable.filter(x => Math.abs(x - c) <= 0.05);
    if (near.length >= minMass) cands.push({ c, h: sm[b], near });
  }
  cands.sort((a, b) => b.h - a.h);
  const kept = [];
  for (const cd of cands) if (kept.every(k => Math.abs(k.c - cd.c) >= CFG.gearMinSepLn)) kept.push(cd);
  // marcha 1 = maior RPM por km/h
  const centers = kept.map(k => median(k.near)).sort((a, b) => b - a);
  result.centers = centers;
  if (!centers.length) return result;

  // atribuição por proximidade
  const raw = new Array(n).fill(null);
  for (let i = 0; i < n; i++) {
    if (lnk[i] == null) continue;
    let best = -1, bd = Infinity;
    centers.forEach((c, gi) => { const d = Math.abs(lnk[i] - c); if (d < bd) { bd = d; best = gi; } });
    if (bd <= CFG.gearTolLn + quant(i) / 2) raw[i] = best + 1;
  }
  // descarta trechos curtos e preenche buracos curtos entre a mesma marcha
  const runs = toRuns(raw);
  const minDwell = Math.ceil(CFG.gearMinDwellMs / step);
  for (const r of runs) if (r.v != null && r.len < minDwell) for (let i = r.i; i < r.i + r.len; i++) raw[i] = null;
  const runs2 = toRuns(raw);
  const maxFill = Math.floor(CFG.gearFillMs / step);
  for (let k = 1; k < runs2.length - 1; k++) {
    const r = runs2[k];
    if (r.v == null && r.len <= maxFill && runs2[k - 1].v != null && runs2[k - 1].v === runs2[k + 1].v) {
      for (let i = r.i; i < r.i + r.len; i++) raw[i] = runs2[k - 1].v;
    }
  }
  result.gear = raw;
  return result;
}

function toRuns(a) {
  const runs = [];
  for (let i = 0; i < a.length; i++) {
    const last = runs[runs.length - 1];
    if (last && last.v === a[i]) last.len++;
    else runs.push({ v: a[i], i, len: 1 });
  }
  return runs;
}

function detectShifts(grid, gear, rpm, speed) {
  const segs = toRuns(gear).filter(r => r.v != null);
  const shifts = [];
  for (let k = 1; k < segs.length; k++) {
    const a = segs[k - 1], b = segs[k];
    if (a.v === b.v) continue;
    const iEnd = a.i + a.len - 1, iStart = b.i;
    if (grid[iStart] - grid[iEnd] > CFG.shiftMaxGapMs) continue;
    shifts.push({
      dir: b.v > a.v ? 'up' : 'down',
      from: a.v, to: b.v,
      t: grid[iEnd], i: iEnd,
      tAfter: grid[iStart], iAfter: iStart,
      rpmBefore: rpm[iEnd], rpmAfter: rpm[iStart],
      speed: speed[iEnd],
    });
  }
  return shifts;
}

// ---------- eventos por limiar ----------

function thresholdEvents(grid, step, values, test, minMs) {
  const evs = [];
  let start = -1;
  for (let i = 0; i <= values.length; i++) {
    const hit = i < values.length && values[i] != null && test(values[i], i);
    if (hit && start < 0) start = i;
    if (!hit && start >= 0) {
      if ((i - start) * step >= minMs) evs.push({ i0: start, i1: i - 1, t0: grid[start], t1: grid[i - 1] });
      start = -1;
    }
  }
  return evs;
}

// ---------- análise ----------

function analyze(series, opts) {
  const afr = opts && opts.afr || 14.7;
  const density = opts && opts.density || 745;
  const pids = Object.keys(series).map(Number);
  let t0 = Infinity, t1 = -Infinity;
  for (const p of pids) { t0 = Math.min(t0, series[p].t[0]); t1 = Math.max(t1, series[p].t[series[p].t.length - 1]); }

  const fast = [PID.RPM, PID.SPEED, PID.PEDAL].filter(p => series[p]).map(p => medianInterval(series[p].t)).filter(Number.isFinite);
  const step = Math.min(1000, Math.max(100, Math.round((fast.length ? median(fast) : 1000) / 50) * 50 || 250));
  const n = Math.floor((t1 - t0) / step) + 1;
  const grid = new Array(n);
  for (let i = 0; i < n; i++) grid[i] = t0 + i * step;

  const interval = {}, rs = {};
  for (const p of pids) {
    interval[p] = medianInterval(series[p].t);
    const maxAge = Math.max(3 * (interval[p] || step), 2 * step);
    rs[p] = resample(series[p], grid, maxAge);
  }
  const nul = () => new Array(n).fill(null);
  const rpm = rs[PID.RPM] || nul(), speed = rs[PID.SPEED] || nul(), pedal = rs[PID.PEDAL] || nul();
  const load = rs[PID.LOAD] || nul(), maf = rs[PID.MAF] || nul(), coolant = rs[PID.COOLANT] || nul();
  const dt = step / 1000;

  // aceleração longitudinal (m/s²) por diferença central de ±0,5 s
  const h = Math.max(1, Math.round(500 / step));
  const accel = nul();
  for (let i = h; i < n - h; i++) {
    if (speed[i - h] != null && speed[i + h] != null) accel[i] = (speed[i + h] - speed[i - h]) / 3.6 / (2 * h * dt);
  }

  // combustível: MAF / AFR = g/s de combustível
  const lph = nul(), litersCum = nul(), distCum = nul(), kmlCum = nul();
  let liters = 0, dist = 0, fuelCovered = 0;
  for (let i = 0; i < n; i++) {
    if (maf[i] != null) { lph[i] = maf[i] / afr / density * 3600; liters += lph[i] * dt / 3600; fuelCovered++; }
    if (speed[i] != null) dist += speed[i] * dt / 3600;
    litersCum[i] = liters; distCum[i] = dist;
    if (liters > 0.02) kmlCum[i] = dist / liters;
  }

  // tempos
  let moving = 0, idle = 0, engineOn = 0, engineBrake = 0, cold = 0, speedSum = 0;
  for (let i = 0; i < n; i++) {
    const on = rpm[i] != null && rpm[i] >= CFG.engineOnRpm;
    if (on) engineOn++;
    if (speed[i] != null && speed[i] >= CFG.movingKmh) { moving++; speedSum += speed[i]; }
    else if (on && speed[i] != null) idle++;
    if (on && pedal[i] != null && speed[i] != null && pedal[i] <= CFG.engineBrakePedal &&
        rpm[i] >= CFG.engineBrakeRpm && speed[i] >= CFG.engineBrakeKmh) engineBrake++;
    if (on && coolant[i] != null && coolant[i] < CFG.coldC &&
        (rpm[i] > CFG.coldRpm || (pedal[i] != null && pedal[i] > CFG.coldPedal))) cold++;
  }

  // marchas e trocas
  const gears = detectGears(grid, step, rpm, speed);
  const shifts = gears.centers.length >= 2 ? detectShifts(grid, gears.gear, rpm, speed) : [];
  const gearTime = gears.centers.map(() => 0);
  for (const g of gears.gear) if (g != null) gearTime[g - 1] += dt;

  // distribuições
  const rpmBands = [[0, 1000], [1000, 1500], [1500, 2000], [2000, 2500], [2500, 3000], [3000, 3500], [3500, 4000], [4000, 5000], [5000, Infinity]]
    .map(([a, b]) => ({ a, b, s: 0 }));
  const pedalBands = [[0, 5, 'Solto'], [5, 20], [20, 40], [40, 60], [60, 80], [80, 101]].map(([a, b, l]) => ({ a, b, label: l, s: 0 }));
  let rpmMax = 0;
  for (let i = 0; i < n; i++) if (rpm[i] != null) rpmMax = Math.max(rpmMax, rpm[i]);
  const hmRpmBin = 500, hmPedalBin = 10;
  const hmRows = Math.max(1, Math.ceil((rpmMax + 1) / hmRpmBin));
  const heat = Array.from({ length: hmRows }, () => new Array(10).fill(0));
  for (let i = 0; i < n; i++) {
    const on = rpm[i] != null && rpm[i] >= CFG.engineOnRpm;
    if (!on) continue;
    for (const b of rpmBands) if (rpm[i] >= b.a && rpm[i] < b.b) { b.s += dt; break; }
    if (pedal[i] != null) {
      for (const b of pedalBands) if (pedal[i] >= b.a && pedal[i] < b.b) { b.s += dt; break; }
      heat[Math.min(hmRows - 1, Math.floor(rpm[i] / hmRpmBin))][Math.min(9, Math.floor(pedal[i] / hmPedalBin))] += dt;
    }
  }

  // taxa de variação do pedal (%/s)
  const pedalRates = [];
  const ps = series[PID.PEDAL];
  if (ps) for (let i = 1; i < ps.t.length; i++) {
    const d = ps.t[i] - ps.t[i - 1];
    if (d > 0 && d <= 3 * (interval[PID.PEDAL] || 1000)) pedalRates.push(Math.abs(ps.v[i] - ps.v[i - 1]) / (d / 1000));
  }

  // eventos
  const harshAccel = thresholdEvents(grid, step, accel, a => a >= CFG.harshAccel, CFG.harshMinMs)
    .map(e => ({ ...e, type: 'accel', peak: maxOf(accel.slice(e.i0, e.i1 + 1)) }));
  const harshBrake = thresholdEvents(grid, step, accel, a => a <= CFG.harshBrake, CFG.harshMinMs)
    .map(e => ({ ...e, type: 'brake', peak: minOf(accel.slice(e.i0, e.i1 + 1)) }));
  const longIdle = thresholdEvents(grid, step, speed, (s, i) => s < CFG.movingKmh && rpm[i] != null && rpm[i] >= CFG.engineOnRpm, CFG.longIdleMs)
    .map(e => ({ ...e, type: 'idle', peak: (e.t1 - e.t0) / 1000 }));
  const events = [...harshAccel, ...harshBrake, ...longIdle].sort((a, b) => a.t0 - b.t0);

  // aquecimento
  let warmAt = null;
  const cs = series[PID.COOLANT];
  if (cs) for (let i = 0; i < cs.t.length; i++) if (cs.v[i] >= CFG.warmC) { warmAt = cs.t[i]; break; }

  const ups = shifts.filter(s => s.dir === 'up' && s.rpmBefore != null).map(s => s.rpmBefore);
  const downs = shifts.filter(s => s.dir === 'down' && s.rpmBefore != null).map(s => s.rpmBefore);

  return {
    t0, t1, step, n, grid, interval, pids,
    rpm, speed, pedal, load, maf, coolant, accel, lph, litersCum, distCum, kmlCum,
    fuelLevel: series[PID.FUEL] || null,
    coolantSeries: cs || null,
    gears, shifts, gearTime,
    rpmBands, pedalBands, heat, hmRpmBin, hmPedalBin,
    events,
    stats: {
      durationS: (t1 - t0) / 1000,
      movingS: moving * dt, idleS: idle * dt, engineOnS: engineOn * dt,
      engineBrakeS: engineBrake * dt, coldAggressiveS: cold * dt,
      avgMovingKmh: moving ? speedSum / moving : null,
      maxKmh: series[PID.SPEED] ? maxOf(series[PID.SPEED].v) : null,
      maxRpm: rpmMax || null,
      distanceKm: dist,
      liters, fuelCoverage: fuelCovered / n,
      kmPerL: liters > 0.02 ? dist / liters : null,
      upshiftRpmMedian: ups.length ? median(ups) : null,
      upshiftRpmP25: ups.length ? quantile(ups, 0.25) : null,
      upshiftRpmP75: ups.length ? quantile(ups, 0.75) : null,
      downshiftRpmMedian: downs.length ? median(downs) : null,
      nUp: ups.length, nDown: downs.length,
      pedalRateP95: pedalRates.length ? quantile(pedalRates, 0.95) : null,
      pedalMean: (() => { const a = pedal.filter((p, i) => p != null && speed[i] != null && speed[i] >= CFG.movingKmh); return a.length ? a.reduce((x, y) => x + y, 0) / a.length : null; })(),
      kickdownS: pedalBands[pedalBands.length - 1].s,
      warmupS: warmAt != null ? (warmAt - t0) / 1000 : null,
      maxCoolant: cs ? maxOf(cs.v) : null,
      nHarshAccel: harshAccel.length, nHarshBrake: harshBrake.length, nLongIdle: longIdle.length,
    },
  };
}

// ---------- âncora de tempo ----------

/** Texto de timestamp (formato do Postgres, ISO ou epoch) → ms desde a época (com fração). */
function parseTimestamp(str, assumeUtc) {
  if (str == null) return null;
  const s = String(str).trim();
  if (!s) return null;
  if (/^\d{10}(\.\d+)?$/.test(s)) return Number(s) * 1000;
  if (/^\d{13}(\.\d+)?$/.test(s)) return Number(s);
  const m = s.match(/^(\d{4})-(\d{2})-(\d{2})[ T](\d{1,2}):(\d{2})(?::(\d{2})(?:[.,](\d{1,9}))?)?\s*(Z|UTC|[+-]\d{2}(?::?\d{2})?)?$/i);
  if (!m) return null;
  const [, Y, Mo, D, H, Mi, S = '0', F = '', tz] = m;
  const frac = F ? Number('0.' + F) * 1000 : 0;
  let ms;
  if (tz || assumeUtc) {
    ms = Date.UTC(+Y, +Mo - 1, +D, +H, +Mi, +S) + frac;
    if (tz && !/^(z|utc)$/i.test(tz)) {
      const sign = tz[0] === '-' ? -1 : 1;
      const digits = tz.slice(1).replace(':', '');
      const off = (+digits.slice(0, 2)) * 60 + (+(digits.slice(2, 4) || 0));
      ms -= sign * off * 60000;
    }
  } else {
    ms = new Date(+Y, +Mo - 1, +D, +H, +Mi, +S).getTime() + frac;
  }
  return Number.isFinite(ms) ? ms : null;
}

/** Âncora (t_ms, s1, s2) → deslocamento o tal que hora_real = o + t_ms, com a meia-largura da incerteza. */
function anchorOffset(tMs, s1Ms, s2Ms) {
  if (!Number.isFinite(tMs) || !Number.isFinite(s1Ms)) return null;
  if (!Number.isFinite(s2Ms)) s2Ms = s1Ms;
  if (s2Ms < s1Ms) return { error: 's2 é anterior a s1.' };
  return { offsetMs: (s1Ms + s2Ms) / 2 - tMs, uncertaintyMs: (s2Ms - s1Ms) / 2 };
}

const api = { PID, PID_INFO, CFG, FUEL_PRESETS, parseReadingsCsv, toSeries, analyze, parseTimestamp, anchorOffset, median, quantile };
if (typeof module !== 'undefined' && module.exports) module.exports = api;
else root.TripAnalysis = api;
})(this);
