#!/usr/bin/env node
'use strict';
/*
 * Impacto da escrita na flash sobre a coleta, a partir de um log de eventos
 * (docs/debug-events.md). Uso: node flash-impact.js sessao.log [outra.log ...]
 *
 * Por boot: prazo dos blocos, como os blocos fecharam, quanto tempo a flash
 * ficou ocupada, e as leituras que coincidiram com uma escrita comparadas com
 * as demais (duração, respostas perdidas, atraso das decisões).
 */
const fs = require('fs');
const path = require('path');
const vm = require('vm');

const ctx = {console};
vm.createContext(ctx);
vm.runInContext(fs.readFileSync(path.join(__dirname, 'core.js'), 'utf8') + '\nthis.api = {parseLog, build};', ctx);
const {parseLog, build} = ctx.api;

const num = v => (v == null || v === '-') ? null : Number(v);
const pct = (a, b) => b ? (100 * a / b).toFixed(1) + '%' : '—';
const quantile = (xs, q) => {
  if (!xs.length) return null;
  const s = [...xs].sort((a, b) => a - b);
  return s[Math.min(s.length - 1, Math.ceil(q * s.length) - 1)];
};
const mean = xs => xs.length ? xs.reduce((a, b) => a + b, 0) / xs.length : null;
const fmt = v => v == null ? '—' : (Number.isInteger(v) ? String(v) : v.toFixed(1));

function readStats(reads) {
  return {
    n: reads.length,
    lost: pct(reads.filter(r => !r.ok).length, reads.length),
    durMean: fmt(mean(reads.map(r => r.dur))),
    durP99: fmt(quantile(reads.map(r => r.dur), 0.99)),
    lateP99: fmt(quantile(reads.filter(r => r.late != null).map(r => r.late), 0.99)),
  };
}

function analyze(model, seg) {
  const evs = model.evs.slice(seg.start, seg.end + 1);
  const span = seg.lastT - seg.firstT;
  const config = evs.find(e => e.type === 'CONFIG');
  const flushes = evs.filter(e => e.type === 'FLUSH')
    .map(e => ({start: e.t - num(e.f.dur), end: e.t, dur: num(e.f.dur), len: num(e.f.len)}));
  const closes = evs.filter(e => e.type === 'BLK_CLOSE')
    .map(e => ({reason: e.f.reason, len: num(e.f.len), recs: num(e.f.recs), age: num(e.f.age)}));

  // Cada READ, com o atraso da decisão que o originou e se coincidiu com uma escrita.
  let late = null;
  const reads = [];
  for (const e of evs) {
    if (e.type === 'DECIDE') late = num(e.f.late);
    if (e.type !== 'READ') continue;
    const start = num(e.f.start), end = e.t;
    const overlaps = flushes.some(f => f.start < end && start < f.end);
    reads.push({ok: e.f.ok === '1', dur: num(e.f.dur), late, overlaps});
    late = null;
  }

  const byReason = {};
  for (const c of closes) byReason[c.reason] = (byReason[c.reason] || 0) + 1;
  const busy = flushes.reduce((a, f) => a + f.dur, 0);

  return {
    boot: seg.boot,
    duration_s: (span / 1000).toFixed(1),
    flush_interval_ms: config ? (num(config.f.flush) ?? 10000) : '—',
    blocks: closes.length,
    close_reasons: Object.entries(byReason).map(([k, v]) => `${k} ${v}`).join(', ') || '—',
    block_len_mean_B: fmt(mean(closes.map(c => c.len))),
    block_age_mean_ms: fmt(mean(closes.map(c => c.age))),
    writes_per_min: span ? (flushes.length * 60000 / span).toFixed(1) : '—',
    flash_busy: pct(busy, span),
    write_ms_p50: fmt(quantile(flushes.map(f => f.dur), 0.5)),
    write_ms_p99: fmt(quantile(flushes.map(f => f.dur), 0.99)),
    write_ms_max: fmt(flushes.length ? Math.max(...flushes.map(f => f.dur)) : null),
    records_dropped: evs.filter(e => e.type === 'REC_DROP').length,
    reads_during_write: readStats(reads.filter(r => r.overlaps)),
    reads_otherwise: readStats(reads.filter(r => !r.overlaps)),
  };
}

function print(file, r) {
  const rows = [
    ['boot', r.boot], ['duração', r.duration_s + ' s'], ['prazo dos blocos', r.flush_interval_ms + ' ms'],
    ['blocos fechados', r.blocks], ['motivo do fechamento', r.close_reasons],
    ['tamanho médio do bloco', r.block_len_mean_B + ' B'], ['idade média ao fechar', r.block_age_mean_ms + ' ms'],
    ['escritas por minuto', r.writes_per_min], ['flash ocupada', r.flash_busy],
    ['escrita p50 / p99 / máx', `${r.write_ms_p50} / ${r.write_ms_p99} / ${r.write_ms_max} ms`],
    ['registros descartados', r.records_dropped],
  ];
  console.log(`\n${file}`);
  for (const [k, v] of rows) console.log(`  ${k.padEnd(26)} ${v}`);
  console.log(`\n  ${'leituras'.padEnd(26)} ${'durante escrita'.padStart(16)} ${'demais'.padStart(10)}`);
  const a = r.reads_during_write, b = r.reads_otherwise;
  for (const [k, ka] of [['quantidade', 'n'], ['sem resposta', 'lost'], ['duração média (ms)', 'durMean'],
                         ['duração p99 (ms)', 'durP99'], ['atraso da decisão p99 (ms)', 'lateP99']]) {
    console.log(`  ${k.padEnd(26)} ${String(a[ka]).padStart(16)} ${String(b[ka]).padStart(10)}`);
  }
}

const files = process.argv.slice(2);
if (!files.length) {
  console.error('uso: node flash-impact.js sessao.log [outra.log ...]');
  process.exit(1);
}
for (const file of files) {
  const model = build(parseLog(fs.readFileSync(file, 'utf8')));
  for (const seg of model.segs) print(path.basename(file) + (model.segs.length > 1 ? ` · boot ${seg.boot}` : ''), analyze(model, seg));
}
