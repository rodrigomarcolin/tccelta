/* Interface da análise de viagem: carrega o CSV, aplica a âncora de tempo e desenha os gráficos. */
(function () {
'use strict';
const A = window.TripAnalysis;
const $ = id => document.getElementById(id);

const S = {
  fileName: '', boots: null, warnings: [], bootKey: null, series: null, res: null,
  anchor: null,          // {offsetMs, uncertaintyMs}
  xRange: null,          // zoom atual [min, max] no eixo x (s)
  plots: [],
};

// ---------- formatação ----------

const nf = (x, d = 0) => x == null || !Number.isFinite(x) ? '—'
  : x.toLocaleString('pt-BR', { minimumFractionDigits: d, maximumFractionDigits: d });
const pad = (n, w = 2) => String(n).padStart(w, '0');

function fmtDur(s) {
  if (s == null || !Number.isFinite(s)) return '—';
  s = Math.round(s);
  if (s < 60) return `${s} s`;
  if (s < 3600) return `${Math.floor(s / 60)} min ${pad(s % 60)} s`;
  return `${Math.floor(s / 3600)} h ${pad(Math.floor(s % 3600 / 60))} min`;
}

/** x do gráfico (s): hora real (época) com âncora, senão segundos desde o boot. */
const xOf = tMs => S.anchor ? (S.anchor.offsetMs + tMs) / 1000 : tMs / 1000;

function fmtX(x, precise) {
  if (x == null) return '—';
  if (S.anchor) {
    const d = new Date(x * 1000);
    const base = `${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())}`;
    return precise ? `${base},${Math.floor(d.getMilliseconds() / 100)}` : base;
  }
  const neg = x < 0; let s = Math.abs(x);
  const h = Math.floor(s / 3600), m = Math.floor(s % 3600 / 60), sec = s % 60;
  const ss = precise ? sec.toFixed(1).replace('.', ',').padStart(4, '0') : pad(Math.floor(sec));
  return (neg ? '−' : '') + (h ? `${h}:${pad(m)}:${ss}` : `${m}:${ss}`);
}
const fmtT = (tMs, precise) => fmtX(xOf(tMs), precise);

function fmtDateTime(ms) {
  const d = new Date(ms);
  return `${pad(d.getDate())}/${pad(d.getMonth() + 1)}/${d.getFullYear()} ${pad(d.getHours())}:${pad(d.getMinutes())}:${pad(d.getSeconds())},${pad(Math.floor(d.getMilliseconds()), 3)}`;
}

// ---------- cores ----------

const css = name => getComputedStyle(document.documentElement).getPropertyValue(name).trim();
const ramp = name => css(name).split(',').map(s => s.trim()).filter(Boolean);
function gearColor(g, nG) {
  const r = ramp('--ord');
  if (nG <= 1) return r[4];
  return r[Math.round((g - 1) * (r.length - 1) / (nG - 1))];
}

// ---------- tooltip ----------

const tip = $('tip');
function showTip(text, ev) {
  tip.textContent = text;
  tip.hidden = false;
  const w = tip.offsetWidth, h = tip.offsetHeight;
  let x = ev.clientX + 14, y = ev.clientY + 14;
  if (x + w > innerWidth - 8) x = ev.clientX - w - 14;
  if (y + h > innerHeight - 8) y = ev.clientY - h - 14;
  tip.style.left = x + 'px'; tip.style.top = y + 'px';
}
const hideTip = () => { tip.hidden = true; };
document.addEventListener('mousemove', ev => {
  const el = ev.target.closest && ev.target.closest('[data-tip]');
  if (el) showTip(el.dataset.tip, ev);
  else if (!ev.target.closest || !ev.target.closest('.hover-own')) hideTip();
});

// ---------- carga do arquivo ----------

function loadText(text, name) {
  try {
    const { boots, warnings } = A.parseReadingsCsv(text);
    S.fileName = name; S.boots = boots; S.warnings = warnings;
    $('fileName').textContent = name;
    const keys = [...boots.keys()];
    const sel = $('bootSel');
    sel.innerHTML = '';
    for (const k of keys) {
      const b = boots.get(k);
      const o = document.createElement('option');
      o.value = k; o.textContent = `geração ${b.generation ?? '?'} · boot_id ${b.bootId ?? '?'} · ${nf(b.rows.length)} leituras`;
      sel.appendChild(o);
    }
    $('bootPick').hidden = keys.length < 2;
    $('loadError').hidden = true;
    $('empty').hidden = true;
    $('app').hidden = false;
    selectBoot(keys[0]);
  } catch (e) {
    $('loadError').textContent = e.message;
    $('loadError').hidden = false;
    $('empty').hidden = false;
    $('app').hidden = true;
  }
}

function openFile(file) {
  if (!file) return;
  const r = new FileReader();
  r.onload = () => loadText(String(r.result), file.name);
  r.readAsText(file);
}

$('openBtn').onclick = $('openBtn2').onclick = () => $('fileInput').click();
$('fileInput').onchange = e => { openFile(e.target.files[0]); e.target.value = ''; };
document.addEventListener('dragover', e => { e.preventDefault(); document.body.classList.add('drag'); });
document.addEventListener('dragleave', e => { if (!e.relatedTarget) document.body.classList.remove('drag'); });
document.addEventListener('drop', e => {
  e.preventDefault(); document.body.classList.remove('drag');
  openFile(e.dataTransfer.files[0]);
});

function selectBoot(key) {
  S.bootKey = key;
  $('bootSel').value = key;
  const b = S.boots.get(key);
  S.series = A.toSeries(b.rows);
  S.xRange = null;
  restoreAnchor();
  recompute();
}
$('bootSel').onchange = e => selectBoot(e.target.value);

// ---------- combustível ----------

(function initFuel() {
  const sel = $('fuelPreset');
  for (const p of A.FUEL_PRESETS) {
    const o = document.createElement('option');
    o.value = p.id; o.textContent = `${p.label} (AFR ${nf(p.afr, 1)})`;
    sel.appendChild(o);
  }
  const o = document.createElement('option'); o.value = 'custom'; o.textContent = 'Personalizado';
  sel.appendChild(o);
  const apply = p => { $('afr').value = p.afr; $('dens').value = p.density; };
  apply(A.FUEL_PRESETS[0]);
  sel.onchange = () => {
    const p = A.FUEL_PRESETS.find(x => x.id === sel.value);
    if (p) { apply(p); recompute(); }
  };
  for (const id of ['afr', 'dens']) $(id).addEventListener('change', () => { sel.value = 'custom'; recompute(); });
})();

function recompute() {
  if (!S.series) return;
  const afr = parseFloat($('afr').value) || 14.7, density = parseFloat($('dens').value) || 745;
  S.res = A.analyze(S.series, { afr, density });
  renderAll();
}

// ---------- âncora ----------

const anchorKey = () => 'trip-analyzer:anchor:' + S.bootKey;

function readAnchorInputs() {
  const tz = document.querySelector('input[name=tz]:checked').value;
  const tStr = $('aT').value.trim(), s1Str = $('aS1').value.trim(), s2Str = $('aS2').value.trim();
  const t = tStr === '' ? NaN : Number(tStr.replace(/\s/g, ''));
  const s1 = A.parseTimestamp(s1Str, tz === 'utc');
  const s2 = A.parseTimestamp(s2Str, tz === 'utc');
  $('aT').classList.toggle('bad', tStr !== '' && !Number.isInteger(t));
  $('aS1').classList.toggle('bad', s1Str !== '' && s1 == null);
  $('aS2').classList.toggle('bad', s2Str !== '' && s2 == null);
  return { tStr, s1Str, s2Str, tz, t, s1, s2 };
}

function updateAnchor(persist) {
  const v = readAnchorInputs();
  const st = $('anchorStatus');
  let next = null;
  st.className = 'status';
  if (!v.tStr && !v.s1Str && !v.s2Str) {
    st.textContent = 'Sem âncora: tempo relativo ao boot.';
  } else if (!Number.isInteger(v.t) || v.s1 == null || (v.s2Str && v.s2 == null)) {
    st.className = 'status bad';
    const missing = [];
    if (!Number.isInteger(v.t)) missing.push('t_ms (inteiro)');
    if (v.s1 == null) missing.push('s1');
    if (v.s2Str && v.s2 == null) missing.push('s2');
    st.textContent = `Preencha ${missing.join(', ')}. Formatos aceitos: 2026-10-28 14:03:22.123+00, 2026-10-28T14:03:22Z ou epoch.`;
  } else {
    const a = A.anchorOffset(v.t, v.s1, v.s2 ?? v.s1);
    if (a.error) {
      st.className = 'status bad'; st.textContent = a.error;
    } else {
      next = a;
      st.className = 'status ok';
      const r = S.res;
      const tz = Intl.DateTimeFormat().resolvedOptions().timeZone;
      let msg = `Boot iniciado em ${fmtDateTime(a.offsetMs)} (±${nf(a.uncertaintyMs / 1000, 3)} s).`;
      if (r) msg += ` Leituras de ${fmtDateTime(a.offsetMs + r.t0)} a ${fmtDateTime(a.offsetMs + r.t1)}.`;
      msg += ` Horários no fuso do navegador (${tz}).`;
      if (!v.s2Str) msg += ' Sem s2, a incerteza não é conhecida.';
      st.textContent = msg;
    }
  }
  if (persist) {
    try { localStorage.setItem(anchorKey(), JSON.stringify({ t: v.tStr, s1: v.s1Str, s2: v.s2Str, tz: v.tz })); } catch (e) { /* armazenamento indisponível */ }
  }
  const changed = JSON.stringify(next) !== JSON.stringify(S.anchor);
  if (changed) {
    // mantém o zoom no mesmo trecho do boot ao trocar de eixo
    if (S.xRange) {
      const toMs = x => S.anchor ? x * 1000 - S.anchor.offsetMs : x * 1000;
      const [a0, a1] = S.xRange.map(toMs);
      S.anchor = next;
      S.xRange = [xOf(a0), xOf(a1)];
    } else S.anchor = next;
  }
  return changed;
}

function restoreAnchor() {
  let saved = null;
  try { saved = JSON.parse(localStorage.getItem(anchorKey()) || 'null'); } catch (e) { /* ignora */ }
  $('aT').value = saved ? saved.t : '';
  $('aS1').value = saved ? saved.s1 : '';
  $('aS2').value = saved ? saved.s2 : '';
  document.querySelector(`input[name=tz][value=${saved && saved.tz === 'local' ? 'local' : 'utc'}]`).checked = true;
  S.anchor = null;
  updateAnchor(false);
}

let anchorTimer = 0;
function onAnchorInput() {
  clearTimeout(anchorTimer);
  anchorTimer = setTimeout(() => { if (updateAnchor(true) && S.res) renderAll(); }, 250);
}
for (const id of ['aT', 'aS1', 'aS2']) $(id).addEventListener('input', onAnchorInput);
for (const r of document.querySelectorAll('input[name=tz]')) r.addEventListener('change', onAnchorInput);
$('anchorClear').onclick = () => { $('aT').value = $('aS1').value = $('aS2').value = ''; onAnchorInput(); };

// ---------- tema ----------

$('themeBtn').onclick = () => {
  const dark = matchMedia('(prefers-color-scheme: dark)').matches;
  const cur = document.documentElement.dataset.theme || (dark ? 'dark' : 'light');
  document.documentElement.dataset.theme = cur === 'dark' ? 'light' : 'dark';
  if (S.res) renderAll();
};
matchMedia('(prefers-color-scheme: dark)').addEventListener('change', () => { if (S.res) renderAll(); });
let resizeTimer = 0;
addEventListener('resize', () => { clearTimeout(resizeTimer); resizeTimer = setTimeout(() => { if (S.res) renderAll(); }, 150); });

// ---------- render ----------

function renderAll() {
  for (const p of S.plots) p.destroy();
  S.plots = [];
  syncing = true;   // os painéis nascem com faixas diferentes; o zoom só é sincronizado no fim
  updateAnchor(false);
  renderInfo();
  renderKpis();
  renderTimeline();
  renderScatter();
  renderGearTime();
  renderStrips();
  renderBands();
  renderHeatmap();
  renderFuel();
  renderTemp();
  renderEvents();
  syncing = false;
  applyRange(S.xRange || fullRange());
}

function renderInfo() {
  const r = S.res, b = S.boots.get(S.bootKey);
  const pidNames = r.pids.map(p => (A.PID_INFO[p] ? A.PID_INFO[p].name : 'PID ' + p) +
    ` (${nf(r.interval[p] / 1000, r.interval[p] < 1000 ? 2 : 0)} s)`);
  $('bootInfo').textContent = `geração ${b.generation ?? '?'} · boot_id ${b.bootId ?? '?'} · ${nf(b.rows.length)} leituras · ` +
    `t_ms ${nf(r.t0)} a ${nf(r.t1)} · grade de ${r.step} ms`;
  const w = [...S.warnings];
  const missing = [A.PID.RPM, A.PID.SPEED, A.PID.PEDAL, A.PID.MAF].filter(p => !S.series[p]).map(p => A.PID_INFO[p].name);
  if (missing.length) w.push(`PIDs ausentes: ${missing.join(', ')}. Os gráficos que dependem deles ficam vazios.`);
  if (r.gears.centers.length < 2 && S.series[A.PID.RPM] && S.series[A.PID.SPEED]) w.push('Não foi possível separar marchas (poucos trechos em movimento estável).');
  $('warnings').innerHTML = '';
  const intervals = document.createElement('div');
  intervals.className = 'info';
  intervals.textContent = 'Período mediano: ' + pidNames.join(' · ');
  $('warnings').appendChild(intervals);
  for (const m of w) { const d = document.createElement('div'); d.textContent = m; $('warnings').appendChild(d); }
}

function renderKpis() {
  const s = S.res.stats;
  const k = (label, value, unit, detail) =>
    `<div class="kpi"><div class="k">${label}</div><div class="v">${value}${unit ? `<small>${unit}</small>` : ''}</div>${detail ? `<div class="d">${detail}</div>` : ''}</div>`;
  const pct = (a, b) => b ? nf(a / b * 100) + ' %' : '—';
  $('kpis').innerHTML = [
    k('Duração', fmtDur(s.durationS), '', `em movimento ${fmtDur(s.movingS)}`),
    k('Distância', nf(s.distanceKm, 1), 'km', 'integral da velocidade'),
    k('Velocidade média', nf(s.avgMovingKmh), 'km/h', `em movimento · máx. ${nf(s.maxKmh)} km/h`),
    k('Combustível', nf(s.liters, 2), 'L', s.fuelCoverage < 0.9 ? `estimado · MAF em ${nf(s.fuelCoverage * 100)} % do tempo` : 'estimado pelo MAF'),
    k('Consumo médio', nf(s.kmPerL, 1), 'km/L', 'estimado'),
    k('RPM ao subir marcha', nf(s.upshiftRpmMedian), 'rpm', s.nUp ? `mediana de ${s.nUp} · IQR ${nf(s.upshiftRpmP25)}–${nf(s.upshiftRpmP75)}` : 'nenhuma subida detectada'),
    k('RPM ao reduzir', nf(s.downshiftRpmMedian), 'rpm', s.nDown ? `mediana de ${s.nDown}` : 'nenhuma redução detectada'),
    k('Marcha lenta', pct(s.idleS, s.engineOnS), '', `${fmtDur(s.idleS)} parado com motor ligado`),
    k('Pedal médio', nf(s.pedalMean), '%', `em movimento · ${fmtDur(s.kickdownS)} acima de 80 %`),
    k('Freio-motor', pct(s.engineBrakeS, s.movingS), '', 'do tempo em movimento, pedal solto'),
    k('Acelerações bruscas', nf(s.nHarshAccel), '', '≥ 3,0 m/s²'),
    k('Frenagens bruscas', nf(s.nHarshBrake), '', '≤ −3,5 m/s²'),
  ].join('');
}

// ---------- painéis de série temporal (uPlot) ----------

const SYNC_KEY = 'trip';
const X_INCRS = [1, 2, 5, 10, 15, 30, 60, 120, 300, 600, 900, 1800, 3600, 7200, 14400];
let syncing = false;

function fullRange() { return [xOf(S.res.t0), xOf(S.res.t1)]; }

function applyRange(range) {
  syncing = true;
  for (const p of S.plots) p.setScale('x', { min: range[0], max: range[1] });
  syncing = false;
}

function resetZoom() { S.xRange = null; applyRange(fullRange()); }
$('zoomReset').onclick = resetZoom;

function zoomTo(tMs, halfS = 20) {
  const x = xOf(tMs);
  S.xRange = [x - halfS, x + halfS];
  applyRange(S.xRange);
  $('timeline').scrollIntoView({ behavior: 'smooth', block: 'start' });
}

function makePanel(container, cfg) {
  const wrap = document.createElement('div');
  wrap.className = 'panel';
  wrap.innerHTML = `<div class="ptitle">${cfg.title}${cfg.subtitle ? ` <span>${cfg.subtitle}</span>` : ''}</div>`;
  const host = document.createElement('div');
  wrap.appendChild(host);
  container.appendChild(wrap);

  const ink2 = css('--ink-2'), line = css('--line'), panel = css('--panel');
  const axis = { stroke: ink2, grid: { stroke: line, width: 1 }, ticks: { show: false }, font: '11px ' + css('--sans') };
  const series = [{ label: S.anchor ? 'Horário' : 'Tempo desde o boot', value: (u, v) => fmtX(v, true) }];
  for (const s of cfg.series) {
    const o = {
      label: s.label, stroke: s.color, width: s.width || 2, spanGaps: false,
      points: { show: false },
      value: (u, v) => v == null ? '—' : (s.fmt ? s.fmt(v) : nf(v, s.dec || 0) + (cfg.unit ? ' ' + cfg.unit : '')),
    };
    if (s.kind === 'step') o.paths = uPlot.paths.stepped({ align: 1 });
    if (s.kind === 'points') {
      o.paths = () => null;
      o.points = { show: true, size: s.size || 9, width: 2, stroke: panel, fill: s.color };
    }
    series.push(o);
  }
  const yAxis = Object.assign({}, axis, { size: 56, values: cfg.yValues || ((u, vals) => vals.map(v => nf(v))) });
  if (cfg.ySplits) yAxis.splits = cfg.ySplits;
  if (cfg.yLog) yAxis.filter = (u, splits) => splits;   // o padrão do log esconde o que não é 1/2/5 × 10ⁿ
  const opts = {
    width: Math.max(280, container.clientWidth), height: cfg.height || 140,
    series,
    scales: { x: { time: false }, y: Object.assign(cfg.yLog ? { distr: 3, log: 10 } : {}, cfg.yRange ? { range: cfg.yRange } : {}) },
    axes: [
      Object.assign({}, axis, { incrs: X_INCRS, space: 80, values: (u, vals) => vals.map(v => fmtX(v)) }),
      yAxis,
    ],
    cursor: {
      sync: { key: SYNC_KEY, setSeries: false },
      drag: { x: true, y: false },
      points: { size: 7 },
      bind: { dblclick: () => () => { resetZoom(); return null; } },
    },
    legend: { live: true },
    hooks: {
      setScale: [(u, key) => {
        if (key !== 'x' || syncing) return;
        const { min, max } = u.scales.x;
        const [f0, f1] = fullRange();
        S.xRange = (min <= f0 && max >= f1) ? null : [min, max];
        syncing = true;
        for (const p of S.plots) if (p !== u) p.setScale('x', { min, max });
        syncing = false;
      }],
      draw: cfg.draw ? [cfg.draw] : [],
    },
  };
  const plot = new uPlot(opts, cfg.data, host);
  S.plots.push(plot);
  return plot;
}

function gridX() { return S.res.grid.map(t => xOf(t)); }

function renderTimeline() {
  const r = S.res, el = $('timeline');
  el.innerHTML = '';
  const x = gridX();
  const c1 = css('--s1');
  if (!S.series[A.PID.SPEED] && !S.series[A.PID.RPM]) { el.innerHTML = '<div class="empty-chart">Sem velocidade nem rotação no CSV.</div>'; return; }

  makePanel(el, { title: 'Velocidade', unit: 'km/h', height: 140, data: [x, r.speed],
    series: [{ label: 'Velocidade', color: c1 }], yRange: (u, mn, mx) => [0, Math.max(20, Math.ceil(mx / 20) * 20)] });

  const up = new Array(r.n).fill(null), down = new Array(r.n).fill(null);
  for (const s of r.shifts) (s.dir === 'up' ? up : down)[s.i] = s.rpmBefore;
  makePanel(el, { title: 'Rotação', unit: 'rpm', height: 160, data: [x, r.rpm, up, down],
    series: [
      { label: 'Rotação', color: c1 },
      { label: 'Subida de marcha', color: css('--s2'), kind: 'points' },
      { label: 'Redução', color: css('--s7'), kind: 'points' },
    ], yRange: (u, mn, mx) => [0, Math.max(1000, Math.ceil(mx / 1000) * 1000)] });

  const nG = r.gears.centers.length;
  if (S.series[A.PID.RPM] && S.series[A.PID.SPEED]) {
    // abaixo de 8 km/h a razão explode (embreagem, arrancada) e fica de fora
    const k = r.gears.lnk.map(v => v == null ? null : Math.exp(v));
    const ks = r.gears.centers.map(c => Math.exp(c));
    // escala log: as relações de marcha são quase geométricas, então os degraus ficam igualmente espaçados
    let kMin = ks.length ? Math.min(...ks) * 0.75 : Infinity, kMax = ks.length ? Math.max(...ks) * 1.35 : 0;
    if (!ks.length) for (const v of k) if (v != null && v < 300) { kMax = Math.max(kMax, v); kMin = Math.min(kMin, v); }
    kMin = Math.max(1, Number.isFinite(kMin) ? kMin : 10); kMax = Math.max(kMin * 2, kMax);
    const kTicks = [5, 7, 10, 15, 20, 30, 40, 50, 70, 100, 150, 200, 300].filter(v => v >= kMin && v <= kMax);
    makePanel(el, { title: 'Rotação ÷ velocidade', subtitle: 'cada degrau é uma marcha · escala logarítmica · só com velocidade ≥ 8 km/h', unit: 'rpm por km/h', height: 200,
      data: [x, k], series: [{ label: 'RPM/velocidade', color: c1, dec: 1 }],
      yLog: true, yRange: () => [kMin, kMax], ySplits: () => kTicks,
      draw: u => {
        const ctx = u.ctx, dpr = devicePixelRatio || 1;
        ctx.save();
        ctx.font = `600 ${11 * dpr}px ${css('--sans')}`; ctx.textAlign = 'right'; ctx.textBaseline = 'bottom';
        ks.forEach((kc, gi) => {
          const y = Math.round(u.valToPos(kc, 'y', true)) + .5;
          ctx.strokeStyle = css('--muted'); ctx.lineWidth = dpr; ctx.setLineDash([5 * dpr, 4 * dpr]);
          ctx.beginPath(); ctx.moveTo(u.bbox.left, y); ctx.lineTo(u.bbox.left + u.bbox.width, y); ctx.stroke();
          ctx.setLineDash([]); ctx.fillStyle = css('--ink-2');
          ctx.fillText(`${gi + 1}ª · ${nf(kc, 1)}`, u.bbox.left + u.bbox.width - 4 * dpr, y - 2 * dpr);
        });
        ctx.restore();
      } });
  }
  if (nG >= 2) {
    makePanel(el, { title: 'Marcha estimada', subtitle: 'vazio = parado, embreagem ou indefinido', height: 30 + nG * 22,
      data: [x, r.gears.gear], series: [{ label: 'Marcha', color: c1, kind: 'step', fmt: v => v + 'ª' }],
      yRange: () => [0.5, nG + 0.5],
      ySplits: () => Array.from({ length: nG }, (_, i) => i + 1),
      yValues: (u, vals) => vals.map(v => v + 'ª') });
  }
  if (S.series[A.PID.PEDAL]) {
    makePanel(el, { title: 'Pedal do acelerador', unit: '%', height: 120, data: [x, r.pedal],
      series: [{ label: 'Pedal', color: c1 }], yRange: () => [0, 100] });
  }
  if (S.series[A.PID.LOAD]) {
    makePanel(el, { title: 'Carga do motor', unit: '%', height: 120, data: [x, r.load],
      series: [{ label: 'Carga', color: c1 }], yRange: () => [0, 100] });
  }
  if (S.series[A.PID.MAF]) {
    makePanel(el, { title: 'Consumo instantâneo', subtitle: 'estimado pelo MAF', unit: 'L/h', height: 120, data: [x, r.lph],
      series: [{ label: 'Consumo', color: c1, dec: 1 }], yRange: (u, mn, mx) => [0, Math.max(2, Math.ceil(mx / 2) * 2)] });
  }
}

// ---------- dispersão rotação × velocidade (canvas) ----------

function niceMax(v, step) { return Math.max(step, Math.ceil(v / step) * step); }

function renderScatter() {
  const r = S.res, el = $('scatter');
  el.innerHTML = ''; $('scatterLegend').innerHTML = '';
  if (!S.series[A.PID.SPEED] || !S.series[A.PID.RPM]) { el.innerHTML = '<div class="empty-chart">Precisa de rotação e velocidade.</div>'; return; }
  const W = el.clientWidth, H = 320, m = { l: 66, r: 16, t: 10, b: 34 };
  const dpr = devicePixelRatio || 1;
  const cv = document.createElement('canvas');
  cv.width = W * dpr; cv.height = H * dpr; cv.style.height = H + 'px';
  cv.className = 'hover-own';
  el.appendChild(cv);
  const ctx = cv.getContext('2d');
  ctx.scale(dpr, dpr);
  const xMax = niceMax(r.stats.maxKmh || 0, 20), yMax = niceMax(r.stats.maxRpm || 0, 1000);
  const px = v => m.l + v / xMax * (W - m.l - m.r), py = v => H - m.b - v / yMax * (H - m.t - m.b);
  const ink2 = css('--ink-2'), line = css('--line'), muted = css('--muted'), panel = css('--panel');
  ctx.font = '11px ' + css('--sans');
  ctx.strokeStyle = line; ctx.fillStyle = ink2; ctx.lineWidth = 1;
  for (let v = 0; v <= yMax; v += yMax > 4000 ? 1000 : 500) {
    ctx.beginPath(); ctx.moveTo(m.l, py(v) + .5); ctx.lineTo(W - m.r, py(v) + .5); ctx.stroke();
    ctx.textAlign = 'right'; ctx.textBaseline = 'middle'; ctx.fillText(nf(v), m.l - 6, py(v));
  }
  for (let v = 0; v <= xMax; v += xMax > 120 ? 40 : 20) {
    ctx.beginPath(); ctx.moveTo(px(v) + .5, m.t); ctx.lineTo(px(v) + .5, H - m.b); ctx.stroke();
    ctx.textAlign = 'center'; ctx.textBaseline = 'top'; ctx.fillText(nf(v), px(v), H - m.b + 6);
  }
  ctx.textAlign = 'center'; ctx.fillText('velocidade (km/h)', m.l + (W - m.l - m.r) / 2, H - 14);
  ctx.save(); ctx.translate(12, m.t + (H - m.t - m.b) / 2); ctx.rotate(-Math.PI / 2); ctx.fillText('rotação (rpm)', 0, 0); ctx.restore();

  const nG = r.gears.centers.length;
  const pts = [];
  for (let i = 0; i < r.n; i++) {
    if (r.speed[i] == null || r.rpm[i] == null || r.speed[i] < 1) continue;
    pts.push({ i, x: px(r.speed[i]), y: py(r.rpm[i]), g: r.gears.gear[i] });
  }
  // retas de cada marcha
  ctx.save();
  ctx.beginPath(); ctx.rect(m.l, m.t, W - m.l - m.r, H - m.t - m.b); ctx.clip();
  ctx.setLineDash([4, 4]); ctx.lineWidth = 1;
  r.gears.centers.forEach((c, gi) => {
    const k = Math.exp(c);
    ctx.strokeStyle = gearColor(gi + 1, nG);
    ctx.globalAlpha = 0.7;
    ctx.beginPath(); ctx.moveTo(px(0), py(0)); ctx.lineTo(px(xMax), py(k * xMax)); ctx.stroke();
  });
  ctx.setLineDash([]); ctx.globalAlpha = 1;
  // pontos sem marcha por baixo
  ctx.fillStyle = muted; ctx.globalAlpha = 0.3;
  for (const p of pts) if (p.g == null) { ctx.beginPath(); ctx.arc(p.x, p.y, 1.6, 0, 7); ctx.fill(); }
  ctx.globalAlpha = 0.75;
  for (const p of pts) if (p.g != null) { ctx.fillStyle = gearColor(p.g, nG); ctx.beginPath(); ctx.arc(p.x, p.y, 2.2, 0, 7); ctx.fill(); }
  ctx.globalAlpha = 1;
  ctx.restore();
  // rótulo direto de cada marcha, na ponta da nuvem
  ctx.font = '600 12px ' + css('--sans');
  r.gears.centers.forEach((c, gi) => {
    const k = Math.exp(c);
    let vMax = 0;
    for (let i = 0; i < r.n; i++) if (r.gears.gear[i] === gi + 1 && r.speed[i] > vMax) vMax = r.speed[i];
    const v = Math.min(vMax, xMax * 0.95, (yMax * 0.95) / k);
    const x = px(v) + 8, y = py(k * v);
    ctx.lineWidth = 3; ctx.strokeStyle = panel; ctx.fillStyle = css('--ink');
    ctx.textAlign = 'left'; ctx.textBaseline = 'middle';
    ctx.strokeText(`${gi + 1}ª`, x, y); ctx.fillText(`${gi + 1}ª`, x, y);
  });

  cv.addEventListener('mousemove', ev => {
    const b = cv.getBoundingClientRect(), mx = ev.clientX - b.left, my = ev.clientY - b.top;
    let best = null, bd = 100;
    for (const p of pts) { const d = (p.x - mx) ** 2 + (p.y - my) ** 2; if (d < bd) { bd = d; best = p; } }
    if (!best) return hideTip();
    const i = best.i;
    showTip(`${fmtT(r.grid[i], true)}\n${nf(r.speed[i])} km/h · ${nf(r.rpm[i])} rpm\n${best.g ? best.g + 'ª marcha' : 'sem marcha identificada'}`, ev);
  });
  cv.addEventListener('mouseleave', hideTip);

  const leg = $('scatterLegend');
  r.gears.centers.forEach((c, gi) => {
    leg.insertAdjacentHTML('beforeend', `<span><i style="background:${gearColor(gi + 1, nG)}"></i>${gi + 1}ª · ${nf(Math.exp(c), 1)} rpm por km/h</span>`);
  });
  leg.insertAdjacentHTML('beforeend', `<span><i style="background:${muted};opacity:.5"></i>sem marcha</span>`);
}

// ---------- barras horizontais (SVG) ----------

const SVGNS = 'http://www.w3.org/2000/svg';
function svgEl(tag, attrs, parent) {
  const e = document.createElementNS(SVGNS, tag);
  for (const k in attrs) e.setAttribute(k, attrs[k]);
  if (parent) parent.appendChild(e);
  return e;
}

function hbars(el, items, opts = {}) {
  el.innerHTML = '';
  if (!items.length || items.every(it => !it.value)) { el.innerHTML = `<div class="empty-chart">${opts.empty || 'Sem dados.'}</div>`; return; }
  const W = el.clientWidth, rowH = 28, labelW = opts.labelW || 96, valueW = 110;
  const H = items.length * rowH + 4;
  const svg = svgEl('svg', { viewBox: `0 0 ${W} ${H}`, height: H, role: 'img', 'aria-label': opts.aria || '' }, el);
  const max = Math.max(...items.map(it => it.value));
  const barW = W - labelW - valueW;
  const ink = css('--ink'), ink2 = css('--ink-2');
  items.forEach((it, k) => {
    const y = k * rowH + 4, w = max ? Math.max(it.value > 0 ? 3 : 0, it.value / max * barW) : 0;
    const g = svgEl('g', { 'data-tip': it.tip || `${it.label}: ${it.display}` }, svg);
    svgEl('rect', { x: 0, y: y - 2, width: W, height: rowH, fill: 'transparent' }, g);
    const lt = svgEl('text', { x: labelW - 10, y: y + rowH / 2 - 2, 'text-anchor': 'end', 'dominant-baseline': 'middle', fill: ink2, 'font-size': 12 }, g);
    lt.textContent = it.label;
    // barra com ponta arredondada só no fim (ancorada à linha de base)
    const h = rowH - 10, x0 = labelW, r = Math.min(4, w / 2, h / 2);
    if (w > 0) svgEl('path', { d: `M${x0},${y + 3}h${w - r}a${r},${r} 0 0 1 ${r},${r}v${h - 2 * r}a${r},${r} 0 0 1 -${r},${r}h-${w - r}z`, fill: it.color || css('--s1') }, g);
    const vt = svgEl('text', { x: labelW + w + 8, y: y + rowH / 2 - 2, 'dominant-baseline': 'middle', fill: ink, 'font-size': 12 }, g);
    vt.textContent = it.display;
  });
  svgEl('line', { x1: labelW + .5, x2: labelW + .5, y1: 0, y2: H, stroke: css('--line-2') }, svg);
}

function pctOf(v, total) { return total ? nf(v / total * 100) + ' %' : '—'; }

function renderGearTime() {
  const r = S.res, nG = r.gears.centers.length;
  const total = r.gearTime.reduce((a, b) => a + b, 0);
  hbars($('gearTime'), nG >= 2 ? r.gearTime.map((s, gi) => ({
    label: `${gi + 1}ª`, value: s, color: gearColor(gi + 1, nG),
    display: `${fmtDur(s)} · ${pctOf(s, total)}`,
  })) : [], { empty: 'Marchas não identificadas.', labelW: 56 });
}

function renderBands() {
  const r = S.res;
  const pTotal = r.pedalBands.reduce((a, b) => a + b.s, 0);
  hbars($('pedalBands'), S.series[A.PID.PEDAL] ? r.pedalBands.map(b => ({
    label: b.label ? `${b.label} (<${b.b} %)` : `${b.a}–${Math.min(100, b.b)} %`, value: b.s,
    display: `${pctOf(b.s, pTotal)} · ${fmtDur(b.s)}`,
  })) : [], { empty: 'Sem leituras do pedal.', labelW: 104 });
  const st = r.stats;
  $('pedalNote').textContent = st.pedalRateP95 != null
    ? `Variação do pedal (p95): ${nf(st.pedalRateP95)} %/s. Valores altos indicam pisadas e soltadas bruscas.` : '';
  const rTotal = r.rpmBands.reduce((a, b) => a + b.s, 0);
  hbars($('rpmBands'), S.series[A.PID.RPM] ? r.rpmBands.filter(b => b.a < (st.maxRpm || 0) + 1).map(b => ({
    label: b.b === Infinity ? `≥ ${nf(b.a)}` : `${nf(b.a)}–${nf(b.b)}`, value: b.s,
    display: `${pctOf(b.s, rTotal)} · ${fmtDur(b.s)}`,
  })) : [], { empty: 'Sem leituras de rotação.', labelW: 104 });
}

// ---------- trocas (strip plot SVG) ----------

function strip(el, shifts, color) {
  el.innerHTML = '';
  if (!shifts.length) { el.innerHTML = '<div class="empty-chart">Nenhuma troca detectada.</div>'; return; }
  const groups = new Map();
  for (const s of shifts) {
    const k = `${s.from}ª → ${s.to}ª`;
    if (!groups.has(k)) groups.set(k, { from: s.from, to: s.to, list: [] });
    groups.get(k).list.push(s);
  }
  const rows = [...groups.entries()].sort((a, b) => a[1].from - b[1].from || a[1].to - b[1].to);
  const vals = shifts.map(s => s.rpmBefore);
  const lo = Math.floor(Math.min(...vals) / 500) * 500, hi = Math.ceil(Math.max(...vals) / 500) * 500 || 1000;
  const x0 = Math.max(0, lo - 500), x1 = hi + 500;
  const W = el.clientWidth, labelW = 70, rowH = 34, axisH = 24, padR = 70;
  const H = rows.length * rowH + axisH;
  const px = v => labelW + (v - x0) / (x1 - x0) * (W - labelW - padR);
  const svg = svgEl('svg', { viewBox: `0 0 ${W} ${H}`, height: H }, el);
  const line = css('--line'), ink2 = css('--ink-2'), panel = css('--panel'), ink = css('--ink');
  const tick = (x1 - x0) > 3000 ? 1000 : 500;
  for (let v = Math.ceil(x0 / tick) * tick; v <= x1; v += tick) {
    svgEl('line', { x1: px(v), x2: px(v), y1: 0, y2: H - axisH, stroke: line }, svg);
    const t = svgEl('text', { x: px(v), y: H - 8, 'text-anchor': 'middle', fill: ink2, 'font-size': 11 }, svg);
    t.textContent = nf(v);
  }
  rows.forEach(([label, g], k) => {
    const yc = k * rowH + rowH / 2;
    const lt = svgEl('text', { x: labelW - 10, y: yc, 'text-anchor': 'end', 'dominant-baseline': 'middle', fill: ink2, 'font-size': 12 }, svg);
    lt.textContent = label;
    const med = A.median(g.list.map(s => s.rpmBefore));
    g.list.forEach((s, j) => {
      const jitter = g.list.length > 1 ? ((j * 7919) % 11 - 5) * 1.3 : 0;
      const c = svgEl('circle', {
        cx: px(s.rpmBefore), cy: yc + jitter, r: 5, fill: color, stroke: panel, 'stroke-width': 2, style: 'cursor:pointer',
        'data-tip': `${label} às ${fmtT(s.t)}\n${nf(s.rpmBefore)} → ${nf(s.rpmAfter)} rpm a ${nf(s.speed)} km/h`,
      }, svg);
      c.addEventListener('click', () => zoomTo(s.t));
    });
    svgEl('line', { x1: px(med), x2: px(med), y1: yc - 11, y2: yc + 11, stroke: ink, 'stroke-width': 2 }, svg);
    const mt = svgEl('text', { x: W - padR + 8, y: yc, 'dominant-baseline': 'middle', fill: ink, 'font-size': 12 }, svg);
    mt.textContent = `${nf(med)} rpm`;
    const nt = svgEl('title', {}, mt); nt.textContent = `mediana de ${g.list.length}`;
  });
}

function renderStrips() {
  const r = S.res;
  strip($('stripUp'), r.shifts.filter(s => s.dir === 'up' && s.rpmBefore != null), css('--s2'));
  strip($('stripDown'), r.shifts.filter(s => s.dir === 'down' && s.rpmBefore != null), css('--s7'));
  const rows = r.shifts.map((s, k) => `<tr><td>${k + 1}</td><td>${s.dir === 'up' ? 'subida' : 'redução'}</td><td>${s.from}ª → ${s.to}ª</td>
    <td class="mono">${fmtT(s.t, true)}</td><td class="num">${nf(s.rpmBefore)}</td><td class="num">${nf(s.rpmAfter)}</td>
    <td class="num">${nf(s.speed)}</td><td><a class="go" data-t="${s.t}">ver</a></td></tr>`).join('');
  $('shiftTable').innerHTML = r.shifts.length
    ? `<table><thead><tr><th>#</th><th>Tipo</th><th>Marchas</th><th>Horário</th><th class="num">RPM antes</th><th class="num">RPM depois</th><th class="num">km/h</th><th></th></tr></thead><tbody>${rows}</tbody></table>`
    : '<div class="empty-chart">Nenhuma troca detectada.</div>';
}

// ---------- mapa de calor rotação × pedal ----------

function renderHeatmap() {
  const r = S.res, el = $('heatmap');
  el.innerHTML = '';
  if (!S.series[A.PID.PEDAL] || !S.series[A.PID.RPM]) { el.innerHTML = '<div class="empty-chart">Precisa de rotação e pedal.</div>'; return; }
  const heat = r.heat;
  let max = 0;
  for (const row of heat) for (const v of row) max = Math.max(max, v);
  const seq = ramp('--seq');
  const W = el.clientWidth, labelW = 96, cellH = 22, axisH = 34, legendH = 30;
  const cols = 10, cellW = (W - labelW) / cols;
  const rows = heat.length;
  const H = rows * cellH + axisH + legendH;
  const svg = svgEl('svg', { viewBox: `0 0 ${W} ${H}`, height: H }, el);
  const ink2 = css('--ink-2'), empty = css('--panel-2'), panel = css('--panel');
  const colorOf = v => v <= 0 ? empty : seq[Math.min(seq.length - 1, Math.floor(Math.sqrt(v / max) * (seq.length - 1) + 0.5))];
  for (let ri = 0; ri < rows; ri++) {
    const y = (rows - 1 - ri) * cellH;
    const rpmLo = ri * r.hmRpmBin, rpmHi = rpmLo + r.hmRpmBin;
    const lt = svgEl('text', { x: labelW - 8, y: y + cellH / 2, 'text-anchor': 'end', 'dominant-baseline': 'middle', fill: ink2, 'font-size': 11 }, svg);
    lt.textContent = `${nf(rpmLo)}–${nf(rpmHi)}`;
    for (let c = 0; c < cols; c++) {
      const v = heat[ri][c];
      svgEl('rect', {
        x: labelW + c * cellW + 1, y: y + 1, width: Math.max(0, cellW - 2), height: cellH - 2, rx: 3, fill: colorOf(v),
        'data-tip': `${nf(rpmLo)}–${nf(rpmHi)} rpm · pedal ${c * 10}–${c * 10 + 10} %\n${fmtDur(v)}`,
      }, svg);
    }
  }
  for (let c = 0; c <= cols; c += 2) {
    const t = svgEl('text', { x: labelW + c * cellW, y: rows * cellH + 14, 'text-anchor': 'middle', fill: ink2, 'font-size': 11 }, svg);
    t.textContent = c * 10 + '%';
  }
  const at = svgEl('text', { x: labelW + (W - labelW) / 2, y: rows * cellH + 30, 'text-anchor': 'middle', fill: ink2, 'font-size': 11 }, svg);
  at.textContent = 'pedal do acelerador';
  const yt = svgEl('text', { x: 0, y: -6, fill: ink2, 'font-size': 11 }, svg);
  yt.textContent = 'rpm';
  // legenda do gradiente
  const ly = rows * cellH + axisH + 8, lx = labelW, sw = Math.min(26, (W - labelW - 140) / seq.length);
  const l1 = svgEl('text', { x: lx - 8, y: ly + 7, 'text-anchor': 'end', fill: ink2, 'font-size': 11, 'dominant-baseline': 'middle' }, svg);
  l1.textContent = 'menos tempo';
  seq.forEach((c, k) => svgEl('rect', { x: lx + k * sw, y: ly, width: sw - 1, height: 14, fill: c, stroke: panel, 'stroke-width': 0 }, svg));
  const l2 = svgEl('text', { x: lx + seq.length * sw + 8, y: ly + 7, fill: ink2, 'font-size': 11, 'dominant-baseline': 'middle' }, svg);
  l2.textContent = `mais tempo (máx. ${fmtDur(max)})`;
}

// ---------- combustível e temperatura ----------

function movingMedian(v, w) {
  const out = new Array(v.length);
  for (let i = 0; i < v.length; i++) {
    out[i] = A.median(v.slice(Math.max(0, i - w), Math.min(v.length, i + w + 1)));
  }
  return out;
}

function renderFuel() {
  const r = S.res, el = $('fuelPanels');
  el.innerHTML = '';
  const st = r.stats;
  $('fuelSub').textContent = S.series[A.PID.MAF]
    ? `AFR ${$('afr').value} · ${$('dens').value} g/L. Total estimado: ${nf(st.liters, 2)} L em ${nf(st.distanceKm, 1)} km.`
    : 'Sem leituras de MAF: consumo não estimado.';
  const x = gridX(), c1 = css('--s1');
  if (S.series[A.PID.MAF]) {
    makePanel(el, { title: 'Combustível acumulado', unit: 'L', height: 130, data: [x, r.litersCum],
      series: [{ label: 'Acumulado', color: c1, dec: 2 }], yRange: (u, mn, mx) => [0, Math.max(0.1, mx * 1.1)],
      yValues: (u, vals) => vals.map(v => nf(v, 2)) });
    makePanel(el, { title: 'Consumo médio acumulado', subtitle: 'distância ÷ litros desde o início', unit: 'km/L', height: 130, data: [x, r.kmlCum],
      series: [{ label: 'Média', color: c1, dec: 1 }], yRange: (u, mn, mx) => [0, Math.max(5, Math.ceil(Math.min(mx, 40) / 5) * 5)],
      yValues: (u, vals) => vals.map(v => nf(v, 0)) });
  }
  const fl = r.fuelLevel;
  if (fl && fl.t.length) {
    const xs = fl.t.map(t => xOf(t));
    makePanel(el, { title: 'Nível do tanque', subtitle: 'sensor da boia, oscila com curvas e frenagens', unit: '%', height: 130,
      data: [xs, fl.v, movingMedian(fl.v, 3)],
      series: [
        { label: 'Leitura', color: css('--muted'), kind: 'points', size: 5, dec: 1 },
        { label: 'Mediana móvel', color: c1, dec: 1 },
      ],
      yRange: (u, mn, mx) => [Math.max(0, Math.floor(mn / 5) * 5 - 5), Math.min(100, Math.ceil(mx / 5) * 5 + 5)] });
  }
  if (!el.children.length) el.innerHTML = '<div class="empty-chart">Sem MAF nem nível de combustível no CSV.</div>';
}

function renderTemp() {
  const r = S.res, el = $('tempPanels');
  el.innerHTML = '';
  const st = r.stats;
  if (!r.coolantSeries) { $('tempSub').textContent = ''; el.innerHTML = '<div class="empty-chart">Sem leituras de temperatura.</div>'; return; }
  $('tempSub').textContent = (st.warmupS != null ? `Chegou a 80 °C em ${fmtDur(st.warmupS)}.` : 'Não chegou a 80 °C.') +
    ` Máxima ${nf(st.maxCoolant)} °C. Rotação > 3.000 rpm ou pedal > 60 % com motor abaixo de 60 °C: ${fmtDur(st.coldAggressiveS)}.`;
  const cs = r.coolantSeries;
  const warm = A.CFG.warmC;
  makePanel(el, { title: 'Temperatura', unit: '°C', height: 150, data: [cs.t.map(t => xOf(t)), cs.v],
    series: [{ label: 'Temperatura', color: css('--s1') }],
    yRange: (u, mn, mx) => [Math.min(0, Math.floor(mn / 20) * 20), Math.max(100, Math.ceil(mx / 20) * 20)],
    draw: u => {
      const ctx = u.ctx, y = Math.round(u.valToPos(warm, 'y', true)) + .5;
      ctx.save();
      ctx.strokeStyle = css('--muted'); ctx.setLineDash([5, 4]); ctx.lineWidth = devicePixelRatio || 1;
      ctx.beginPath(); ctx.moveTo(u.bbox.left, y); ctx.lineTo(u.bbox.left + u.bbox.width, y); ctx.stroke();
      ctx.setLineDash([]); ctx.fillStyle = css('--ink-2');
      ctx.font = `${11 * (devicePixelRatio || 1)}px ${css('--sans')}`; ctx.textAlign = 'right'; ctx.textBaseline = 'bottom';
      ctx.fillText('80 °C', u.bbox.left + u.bbox.width - 4, y - 3);
      ctx.restore();
    } });
}

// ---------- eventos ----------

function renderEvents() {
  const r = S.res;
  const name = { accel: 'Aceleração brusca', brake: 'Frenagem brusca', idle: 'Marcha lenta longa' };
  const peak = e => e.type === 'idle' ? fmtDur(e.peak) : `${nf(e.peak, 1)} m/s²`;
  const rows = r.events.map(e => `<tr><td>${name[e.type]}</td><td class="mono">${fmtT(e.t0, true)}</td>
    <td class="num">${fmtDur((e.t1 - e.t0) / 1000 + r.step / 1000)}</td><td class="num">${peak(e)}</td>
    <td class="num">${nf(r.speed[e.i0])} → ${nf(r.speed[e.i1])} km/h</td><td><a class="go" data-t="${e.t0}">ver</a></td></tr>`).join('');
  $('eventTable').innerHTML = r.events.length
    ? `<table><thead><tr><th>Evento</th><th>Início</th><th class="num">Duração</th><th class="num">Pico</th><th class="num">Velocidade</th><th></th></tr></thead><tbody>${rows}</tbody></table>`
    : '<div class="empty-chart">Nenhum evento.</div>';
}

document.addEventListener('click', e => {
  const a = e.target.closest('a.go');
  if (a) zoomTo(Number(a.dataset.t));
});
})();
