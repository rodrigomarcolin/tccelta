'use strict';
/* Shared by index.html and simple.html: parsing, state reconstruction, checks, demo generator (docs/debug-events.md v1). */
/* ================= constants & helpers ================= */
const ROW_H = 26, CP_K = 256, BLOCK_MAX = 1024, HDR = 12, DEADLINE = 10000, ACK_TIMEOUT = 5000, HEALTH_EVERY = 60000;
const CATS = {
  sched:{label:'Scheduler', c:'var(--s1)', types:['DECIDE','READ','BUS']},
  rec:{label:'Records', c:'var(--s3)', types:['REC','REC_DROP','GPS']},
  block:{label:'Blocks', c:'var(--s7)', types:['BLK_OPEN','BLK_CLOSE','BLK_RELEASE']},
  flash:{label:'Flash', c:'var(--s2)', types:['FLUSH','SEG_DROP']},
  cmd:{label:'Commands', c:'var(--s5)', types:['CMD','RESP']},
  link:{label:'Link', c:'var(--s6)', types:['CONN','NET','UP_SEND','UP_ACK','UP_REWIND','TIME_REQ','TIME_RESP','TIME_REPORT']},
  sys:{label:'System', c:'var(--ink-2)', types:['FORMAT','BOOT','CONFIG','CVT','MODE','TMAX_START','TMAX_SAMPLE','TMAX_DONE']},
  health:{label:'Health', c:'var(--muted)', types:['STACK','HEAP','LOG_DROP']},
  other:{label:'Other', c:'var(--line-2)', types:[]},
};
const TYPE_CAT = {};
for (const [k, c] of Object.entries(CATS)) for (const t of c.types) TYPE_CAT[t] = k;
const catOf = t => TYPE_CAT[t] || 'other';
const PROTO = {6:'CAN 11-bit 500 kbps', 7:'CAN 29-bit 500 kbps', 8:'CAN 11-bit 250 kbps', 9:'CAN 29-bit 250 kbps'};
const REASON_C = {full:'var(--s1)', deadline:'var(--s3)', config:'var(--s7)', mode:'var(--s5)', stop:'var(--s2)', released:'var(--muted)'};

// name, unit, decode(A,B), decimals
const PIDS = {
  '04':['Engine load','%',a=>a*100/255,1], '05':['Coolant temp','°C',a=>a-40,0],
  '0A':['Fuel pressure','kPa',a=>a*3,0], '0B':['Intake MAP','kPa',a=>a,0],
  '0C':['Engine RPM','rpm',(a,b)=>(a*256+b)/4,0], '0D':['Vehicle speed','km/h',a=>a,0],
  '0E':['Timing advance','°',a=>a/2-64,1], '0F':['Intake air temp','°C',a=>a-40,0],
  '10':['MAF rate','g/s',(a,b)=>(a*256+b)/100,2], '11':['Throttle','%',a=>a*100/255,1],
  '1F':['Run time','s',(a,b)=>a*256+b,0], '21':['Dist. with MIL','km',(a,b)=>a*256+b,0],
  '2F':['Fuel level','%',a=>a*100/255,1], '31':['Dist. since clear','km',(a,b)=>a*256+b,0],
  '33':['Baro pressure','kPa',a=>a,0], '42':['Module voltage','V',(a,b)=>(a*256+b)/1000,2],
  '43':['Absolute load','%',(a,b)=>(a*256+b)*100/255,1], '45':['Rel. throttle','%',a=>a*100/255,1],
  '46':['Ambient temp','°C',a=>a-40,0], '47':['Abs. throttle B','%',a=>a*100/255,1],
  '49':['Accel pedal D','%',a=>a*100/255,1], '4A':['Accel pedal E','%',a=>a*100/255,1],
  '4C':['Cmd throttle','%',a=>a*100/255,1], '5C':['Oil temp','°C',a=>a-40,0],
  '5E':['Fuel rate','L/h',(a,b)=>(a*256+b)/20,2],
};
const pidName = p => PIDS[p] ? PIDS[p][0] : 'PID ' + p;
function decode(pid, hex){
  if (!hex || hex === '-') return null;
  const a = parseInt(hex.slice(0,2),16), b = hex.length >= 4 ? parseInt(hex.slice(2,4),16) : 0;
  const d = PIDS[pid];
  if (!d) return {text:hex, unit:''};
  return {v:d[2](a,b), text:d[2](a,b).toFixed(d[3]), unit:d[1]};
}
const N = v => (v == null || v === '-') ? null : Number(v);
const B = v => v === '1';
const L = v => (!v || v === '-') ? [] : v.split(',');
const esc = s => String(s ?? '').replace(/[&<>"]/g, c => ({'&':'&amp;','<':'&lt;','>':'&gt;','"':'&quot;'}[c]));
const clamp = (v,a,b) => Math.max(a, Math.min(b, v));
const fmtT = ms => ms == null ? '—' : (ms/1000).toFixed(3) + ' s';
function fmtD(ms){
  if (ms == null || isNaN(ms)) return '—';
  const a = Math.abs(ms);
  if (a < 1000) return Math.round(ms) + ' ms';
  if (a < 60000) return (ms/1000).toFixed(a < 10000 ? 2 : 1) + ' s';
  return Math.floor(ms/60000) + ' min ' + Math.round((a%60000)/1000) + ' s';
}
const pct = (a,b) => b ? Math.round(a*100/b) + '%' : '—';
const avg = a => a.length ? a.reduce((s,x)=>s+x,0)/a.length : null;
const hx = (v,n) => (v>>>0).toString(16).toUpperCase().padStart(n*2,'0').slice(-n*2);
const evLink = (i, label) => `<a class="ev" data-i="${i}">${esc(label ?? '#' + M.evs[i].n + ' ' + M.evs[i].type)}</a>`;

/* ================= parsing ================= */
function parseLog(text){
  const lines = text.split(/\r?\n/);
  const evs = []; let ignored = 0, bad = 0;
  const head = /^EV (\d+) (\d+) ([A-Z_]+)(.*)$/;
  const fre = /\s([a-z0-9_]+)=("(?:[^"\\]|\\.)*"|[^\s"]+)/g;
  for (let li = 0; li < lines.length; li++){
    const s = lines[li].replace(/\s+$/, '');
    if (!s.startsWith('EV ')){ if (s.trim()) ignored++; continue; }
    const m = head.exec(s);
    if (!m){ bad++; continue; }
    const f = {};
    fre.lastIndex = 0; let fm;
    while ((fm = fre.exec(m[4]))){
      let v = fm[2];
      if (v[0] === '"') v = v.slice(1,-1).replace(/\\(["\\])/g, '$1');
      f[fm[1]] = v;
    }
    evs.push({i:evs.length, n:+m[1], t:+m[2], type:m[3], f, raw:s, line:li+1, d:null});
  }
  return {evs, ignored, bad};
}

/* ================= state reconstruction ================= */
function newState(seg){
  return {seg, idx:-1, t:0, boot:null, config:null, rows:{}, order:[],
    bufs:[{st:'free'},{st:'free'}], openBuf:-1, mode:null, pending:null, conn:null, net:null,
    up:{lastAck:null, sends:{}, inflight:[], oldest:null, rewind:null, lastSend:null, ackT:null},
    health:{stack:{}, heap:null, heapT:null}, time:{}, tmaxRun:null, seqNext:null, cmd:null, lastBus:null,
    cnt:{reads:0, ok:0, early:0, recs:0, drops:0, flushOk:0, flushFail:0, segDrop:0, lost:0, diag:0}};
}
const clone = o => JSON.parse(JSON.stringify(o));
function ensureBuf(st, b){ while (st.bufs.length <= b) st.bufs.push({st:'free'}); }
function newRow(p){ return {pid:p, p:null, pmin:null, due:null, last:null, val:null, valT:null, reads:0, ok:0, early:0, lastStart:null, lastDur:null, lastOk:null, lastRead:null}; }

function apply(st, ev){
  const f = ev.f; st.idx = ev.i; st.t = ev.t;
  switch (ev.type){
    case 'BOOT':
      st.boot = {boot:N(f.boot), seq_next:N(f.seq_next), nvs_last:N(f.nvs_last), log_oldest:N(f.log_oldest), log_newest:N(f.log_newest), lost:N(f.lost)};
      st.seqNext = st.boot.seq_next; break;
    case 'CONFIG':
      st.config = {reason:f.reason, proto:N(f.proto), tmax:N(f.tmax), u:N(f.u), pids:L(f.pids), t:ev.t, i:ev.i};
      st.rows = {}; st.order = []; st.pending = null; break;
    case 'CVT': {
      const p = f.pid; if (!st.rows[p]) st.order.push(p);
      const r = st.rows[p] = newRow(p);
      r.p = N(f.p); r.pmin = N(f.pmin); r.due = N(f.due); r.last = N(f.last); r.val = f.val === '-' ? null : f.val;
      break; }
    case 'DECIDE': st.pending = {pids:L(f.pids), left:L(f.pids), t:ev.t, rule:f.rule, i:ev.i}; break;
    case 'READ': {
      const p = f.pid; let r = st.rows[p];
      if (!r){ r = st.rows[p] = newRow(p); st.order.push(p); }
      const ok = B(f.ok), early = B(f.early);
      r.last = ev.t; r.due = N(f.next_due);
      if (ok && f.val && f.val !== '-'){ r.val = f.val; r.valT = ev.t; }
      r.reads++; if (ok) r.ok++; if (early) r.early++;
      r.lastStart = N(f.start); r.lastDur = N(f.dur); r.lastOk = ok; r.lastRead = ev.i;
      st.cnt.reads++; if (ok) st.cnt.ok++; if (early) st.cnt.early++;
      if (st.pending){ st.pending.left = st.pending.left.filter(x => x !== p); if (!st.pending.left.length) st.pending = null; }
      break; }
    case 'BUS': st.cnt.diag++; st.lastBus = {start:N(f.start), dur:N(f.dur), ok:B(f.ok), t:ev.t}; break;
    case 'BLK_OPEN': { const b = N(f.buf); ensureBuf(st,b);
      st.bufs[b] = {st:'fill', t0:N(f.t0), len:HDR, recs:0, openT:ev.t, openI:ev.i}; st.openBuf = b; break; }
    case 'REC': { const b = st.bufs[st.openBuf]; if (b && b.st === 'fill'){ b.len = N(f.len); b.recs++; } st.cnt.recs++; break; }
    case 'REC_DROP': st.cnt.drops++; break;
    case 'BLK_CLOSE': { const b = N(f.buf); ensureBuf(st,b); const o = st.bufs[b];
      st.bufs[b] = {st:'write', t0:o.t0, len:N(f.len), recs:N(f.recs), reason:f.reason, closeT:ev.t, closeI:ev.i, openI:o.openI};
      if (st.openBuf === b) st.openBuf = -1; break; }
    case 'BLK_RELEASE': { const b = N(f.buf); ensureBuf(st,b); st.bufs[b] = {st:'free', lastT:ev.t}; if (st.openBuf === b) st.openBuf = -1; break; }
    case 'FLUSH': { const b = N(f.buf); ensureBuf(st,b);
      st.bufs[b] = {st:'free', lastSeq:N(f.seq), lastOk:B(f.ok), lastT:ev.t};
      if (B(f.ok)){ st.cnt.flushOk++; st.seqNext = N(f.seq) + 1; } else st.cnt.flushFail++;
      break; }
    case 'SEG_DROP': st.cnt.segDrop += N(f.count) || 0; break;
    case 'MODE': st.mode = {diag:B(f.diag), running:B(f.running), t:ev.t}; if (!st.mode.running) st.pending = null; break;
    case 'CONN': st.conn = {up:B(f.up), t:ev.t}; break;
    case 'NET': st.net = {up:B(f.up), t:ev.t}; break;
    case 'UP_SEND': { const s = N(f.seq); st.up.sends[s] = ev.t; st.up.lastSend = s; st.up.oldest = N(f.oldest);
      if (!st.up.inflight.includes(s)) st.up.inflight.push(s); break; }
    case 'UP_ACK': if (B(f.advanced)){ const s = N(f.seq); st.up.lastAck = s; st.up.ackT = ev.t;
      st.up.inflight = st.up.inflight.filter(x => x > s);
      for (const k in st.up.sends) if (+k <= s) delete st.up.sends[k]; } break;
    case 'UP_REWIND': st.up.rewind = {reason:f.reason, from:N(f.from), t:ev.t}; st.up.inflight = []; st.up.sends = {}; break;
    case 'TIME_REQ': st.time.req = {boot:N(f.boot), t1:N(f.t1), t:ev.t}; break;
    case 'TIME_RESP': st.time.resp = {ok:B(f.ok), t:ev.t}; break;
    case 'TIME_REPORT': st.time.report = {t1:N(f.t1), t2:N(f.t2), t:ev.t}; break;
    case 'STACK': st.health.stack[f.task] = N(f.free); break;
    case 'HEAP': st.health.heap = {free:N(f.free), min:N(f.min)}; st.health.heapT = ev.t; break;
    case 'LOG_DROP': st.cnt.lost += N(f.count) || 0; break;
    case 'TMAX_START': st.tmaxRun = {pids:L(f.pids), samples:N(f.samples), list:[], t:ev.t, done:null}; break;
    case 'TMAX_SAMPLE': if (st.tmaxRun) st.tmaxRun.list.push({pid:f.pid, ok:B(f.ok), lat:N(f.lat)}); break;
    case 'TMAX_DONE': if (st.tmaxRun) st.tmaxRun.done = N(f.tmax); break;
    case 'CMD': st.cmd = {txt:f.txt, t:ev.t, i:ev.i}; break;
  }
}

// Re-run the scheduler rules (spec §1) on the reconstructed table and compare with the DECIDE.
function checkDecide(st, ev){
  const c = st.config; if (!c) return null;
  const rows = st.order.map(p => st.rows[p]).filter(r => r && r.due != null);
  if (!rows.length) return null;
  const t = ev.t, f = ev.f, pids = L(f.pids), tmax = c.tmax;
  const cand = rows.map(r => ({pid:r.pid, dueIn:r.due - t, since:r.last == null ? null : t - r.last, pmin:r.pmin,
    isDue:t >= r.due, elig:r.last != null && t - r.last >= r.pmin}));
  const due = rows.filter(r => t >= r.due);
  let ok = true, msgs = [], expected;
  if (due.length){
    const minDue = Math.min(...due.map(r => r.due));
    const best = due.filter(r => r.due === minDue).map(r => r.pid);
    expected = `due ${best.join('/')}`;
    if (f.rule !== 'due'){ ok = false; msgs.push(`a PID was due (${best.join('/')}) but rule=${f.rule}`); }
    else {
      if (!best.includes(pids[0])){ ok = false; msgs.push(`most overdue is ${best.join('/')}, chose ${pids[0]}`); }
      const nd = pids.filter(p => !due.find(r => r.pid === p));
      if (nd.length){ ok = false; msgs.push(`not due: ${nd.join(',')}`); }
      const late = t - Math.min(...pids.map(p => st.rows[p]?.due ?? Infinity));
      if (N(f.late) != null && N(f.late) !== late){ ok = false; msgs.push(`late=${f.late}, table says ${late}`); }
    }
  } else {
    const e = Math.min(...rows.map(r => r.due));
    const elig = rows.filter(r => r.last != null && t - r.last >= r.pmin);
    if (e - t > tmax && elig.length){
      const md = Math.min(...elig.map(r => r.due));
      const best = elig.filter(r => r.due === md).map(r => r.pid);
      expected = `early ${best.join('/')}`;
      if (f.rule !== 'early'){ ok = false; msgs.push(`nothing due, expected an early read`); }
      else {
        if (!best.includes(pids[0])){ ok = false; msgs.push(`eligible with earliest due is ${best.join('/')}, chose ${pids[0]}`); }
        if (N(f.slack) != null && N(f.slack) !== e - t){ ok = false; msgs.push(`slack=${f.slack}, table says ${e - t}`); }
      }
    } else {
      expected = 'wait';
      ok = false;
      msgs.push(elig.length ? `earliest due is ${e - t} ms away (≤ Tmax ${tmax}): should wait` : 'no PID is due or early-eligible: should wait');
    }
  }
  return {ok, expected, msg:msgs.join('; '), cand};
}

/* ================= block decoding ================= */
function decodeBlock(hex){
  const bytes = [];
  for (let k = 0; k + 1 < hex.length; k += 2) bytes.push(parseInt(hex.substr(k,2),16));
  const u16 = o => bytes[o] | bytes[o+1] << 8;
  const u32 = o => (bytes[o] | bytes[o+1] << 8 | bytes[o+2] << 16 | bytes[o+3] << 24) >>> 0;
  const i32 = o => u32(o) | 0;
  if (bytes.length < HDR) return {bytes, hdr:null, recs:[], err:`only ${bytes.length} bytes, shorter than the header`};
  const hdr = {seq:u32(0), boot:u16(4), t0:u32(6), len:u16(10)};
  const recs = []; let err = null;
  const end = Math.min(hdr.len, bytes.length);
  let o = HDR;
  while (o < end){
    const type = bytes[o], size = type === 0xFF ? 15 : 5;
    if (o + size > end){ err = `truncated record at offset ${o}`; break; }
    const r = {k:recs.length, off:o, size, dt:u16(o+1)};
    if (type === 0xFF){ r.kind = 'gps'; r.lat = i32(o+3); r.lon = i32(o+7); r.alt = i32(o+11); }
    else { r.kind = 'pid'; r.pid = hx(type,1); r.val = hx(bytes[o+3],1) + hx(bytes[o+4],1); }
    recs.push(r); o += size;
  }
  return {bytes, hdr, recs, err};
}
function recMatches(r, e){
  if (!e) return false;
  const f = e.f;
  if (f.kind === 'gps') return r.kind === 'gps' && r.dt === N(f.dt) && r.lat === N(f.lat) && r.lon === N(f.lon) && r.alt === N(f.alt);
  return r.kind === 'pid' && r.pid === f.pid && r.dt === N(f.dt) && r.val === f.val;
}
function checkBlock(blk, fl){
  const evs = M.evs, f = fl.f, d = blk.dec, len = N(f.len), c = [];
  const add = (ok, label, detail) => c.push({ok, label, detail});
  add(d.bytes.length === len, 'Hex size = FLUSH.len', `${d.bytes.length} B / ${len} B`);
  if (d.hdr){
    add(d.hdr.len === len, 'Header length = FLUSH.len', `${d.hdr.len} / ${len}`);
    add(d.hdr.seq === N(f.seq), 'Header seq = FLUSH.seq', `${d.hdr.seq} / ${f.seq}`);
    add(d.hdr.boot === N(f.boot), 'Header boot_id = FLUSH.boot', `${d.hdr.boot} / ${f.boot}`);
    if (blk.openIdx != null) add(d.hdr.t0 === blk.t0, 'Header t_ms_start = BLK_OPEN.t0', `${d.hdr.t0} / ${blk.t0}`);
  }
  if (d.err) add(false, 'Records decode cleanly', d.err);
  if (blk.closeIdx != null && !blk.released){
    const cl = evs[blk.closeIdx].f;
    add(N(cl.len) === len, 'BLK_CLOSE.len = FLUSH.len', `${cl.len} / ${len}`);
    add(N(cl.recs) === d.recs.length, 'BLK_CLOSE.recs = decoded records', `${cl.recs} / ${d.recs.length}`);
  }
  if (blk.openIdx != null){
    let mism = 0;
    d.recs.forEach((r, k) => { const e = evs[blk.recIdxs[k]]; r.ev = e ? e.i : null; r.match = recMatches(r, e); if (!r.match) mism++; });
    add(d.recs.length === blk.recIdxs.length && mism === 0, 'Records = REC events (order, kind, pid, dt, value)',
      `${d.recs.length} decoded / ${blk.recIdxs.length} REC · ${mism} mismatched`);
    const pidRecs = blk.recIdxs.filter(i => evs[i].f.kind === 'pid');
    const linked = pidRecs.filter(i => evs[i].d && evs[i].d.read != null).length;
    add(linked === pidRecs.length, 'Each PID record matches its READ (pid, val, t0+dt)', `${linked} / ${pidRecs.length}`);
  } else add(false, 'Block opening seen in log', 'BLK_OPEN not in log — cannot cross-check records');
  if (B(f.ok) && blk.expectedSeq != null) add(N(f.seq) === blk.expectedSeq, 'seq continuity', `seq ${f.seq}, expected ${blk.expectedSeq}`);
  return c;
}

/* ================= model build (single pass) ================= */
let M = null;
function build(parsed){
  const evs = parsed.evs;
  const m = {evs, segs:[], gaps:[], blocks:[], issues:[], cps:[], ignored:parsed.ignored, bad:parsed.bad, lost:0};
  M = m;
  let st = null, seg = null, prev = null, openBlk = {}, pendBlk = {}, lastOkSeq = null, lastGps = null;
  const finishSeg = () => {
    if (!seg) return;
    for (const b of Object.values(openBlk)) seg.intervals.push({k:'blk', buf:b.buf, s:b.t0, e:seg.lastT, i:b.openIdx, open:true});
    for (const b of Object.values(pendBlk)) seg.intervals.push({k:'wait', buf:b.buf, s:b.closeT, e:seg.lastT, i:b.closeIdx});
  };
  const startSeg = (ev, implicit) => {
    finishSeg();
    seg = {id:m.segs.length, start:ev.i, end:ev.i, implicit, pids:[], slot:{}, reads:[], intervals:[], blocks:[],
      cmds:[], firstT:ev.t, lastT:ev.t, gapLost:0, logDrop:0, mismatches:[], blockIssues:[], configs:[], boot:null};
    m.segs.push(seg); st = newState(seg.id); openBlk = {}; pendBlk = {}; lastOkSeq = null;
  };
  const seePid = p => { if (p && !(p in seg.slot)){ seg.slot[p] = seg.pids.length; seg.pids.push(p); } };
  for (const ev of evs){
    const f = ev.f;
    if (!seg || ev.type === 'FORMAT') startSeg(ev, ev.type !== 'FORMAT');
    else if (ev.n <= prev.n){
      startSeg(ev, true);
      m.issues.push({i:ev.i, msg:`Counter went back (n ${prev.n} → ${ev.n}) without FORMAT: treated as a new boot`});
    } else if (ev.n > prev.n + 1){
      const missing = ev.n - prev.n - 1;
      ev.gap = {from:prev.n, to:ev.n, missing}; m.gaps.push(ev.i); seg.gapLost += missing; m.lost += missing;
    }
    ev.seg = seg.id; seg.end = ev.i; seg.lastT = ev.t;
    const d = ev.d = {};
    switch (ev.type){
      case 'BOOT': seg.boot = N(f.boot); break;
      case 'CONFIG': L(f.pids).forEach(seePid); seg.configs.push(ev.i); break;
      case 'CVT': seePid(f.pid); break;
      case 'DECIDE': d.check = checkDecide(st, ev); d.reads = []; if (d.check && !d.check.ok) seg.mismatches.push(ev.i); break;
      case 'READ': {
        seePid(f.pid);
        const row = st.rows[f.pid];
        d.before = row ? {...row} : null; d.tmax = st.config ? st.config.tmax : null;
        d.decide = st.pending ? st.pending.i : null;
        d.interval = row && row.lastStart != null ? N(f.start) - row.lastStart : null;
        if (d.decide != null) evs[d.decide].d.reads.push(ev.i);
        seg.intervals.push({k:'read', pid:f.pid, s:N(f.start), e:ev.t, ok:B(f.ok), early:B(f.early), i:ev.i});
        seg.reads.push(ev.i);
        break; }
      case 'BUS': seg.intervals.push({k:'diag', s:N(f.start), e:N(f.start) + N(f.dur), ok:B(f.ok), i:ev.i}); break;
      case 'TMAX_SAMPLE': seePid(f.pid); seg.intervals.push({k:'tmax', pid:f.pid, s:ev.t - N(f.lat), e:ev.t, ok:B(f.ok), i:ev.i}); break;
      case 'BLK_OPEN': {
        const b = N(f.buf);
        const blk = {id:m.blocks.length, seg:seg.id, buf:b, t0:N(f.t0), openIdx:ev.i, recIdxs:[], closeIdx:null, flushIdx:null};
        m.blocks.push(blk); seg.blocks.push(blk.id); openBlk[b] = blk; d.block = blk.id; break; }
      case 'REC': {
        seePid(f.pid === '-' ? null : f.pid);
        const blk = openBlk[st.openBuf];
        if (blk){ blk.recIdxs.push(ev.i); d.block = blk.id; d.recNo = blk.recIdxs.length - 1; }
        if (f.kind === 'pid'){
          const row = st.rows[f.pid];
          if (row && row.lastRead != null && blk){
            const r = evs[row.lastRead], rv = r.f.val.length === 2 ? r.f.val + '00' : r.f.val;
            if (r.t === blk.t0 + N(f.dt) && rv === f.val && !r.d.rec){ d.read = r.i; r.d.rec = ev.i; }
          }
        } else if (lastGps != null){ d.gps = lastGps; evs[lastGps].d.rec = ev.i; lastGps = null; }
        break; }
      case 'REC_DROP': if (f.kind === 'gps' && lastGps != null){ evs[lastGps].d.drop = ev.i; lastGps = null; }
        else if (st.rows[f.pid] && st.rows[f.pid].lastRead != null) evs[st.rows[f.pid].lastRead].d.drop = ev.i;
        break;
      case 'GPS': lastGps = ev.i; break;
      case 'BLK_CLOSE': {
        const b = N(f.buf), blk = openBlk[b];
        if (blk){
          blk.closeIdx = ev.i; blk.closeT = ev.t; blk.reason = f.reason; delete openBlk[b]; pendBlk[b] = blk; d.block = blk.id;
          seg.intervals.push({k:'blk', buf:b, s:blk.t0, e:ev.t, i:blk.openIdx});
        }
        break; }
      case 'BLK_RELEASE': {
        const b = N(f.buf), blk = openBlk[b];
        if (blk){ blk.closeIdx = ev.i; blk.closeT = ev.t; blk.released = true; blk.reason = 'released'; delete openBlk[b]; d.block = blk.id; }
        break; }
      case 'FLUSH': {
        const b = N(f.buf);
        let blk = pendBlk[b];
        if (!blk){ blk = {id:m.blocks.length, seg:seg.id, buf:b, t0:null, openIdx:null, recIdxs:[], closeIdx:null, orphan:true}; m.blocks.push(blk); seg.blocks.push(blk.id); }
        delete pendBlk[b];
        blk.flushIdx = ev.i; d.block = blk.id;
        blk.dec = decodeBlock(f.hex || '');
        if (blk.orphan && blk.dec.hdr) blk.t0 = blk.dec.hdr.t0;
        blk.expectedSeq = lastOkSeq != null ? lastOkSeq + 1 : (st.boot ? st.boot.seq_next : null);
        if (B(f.ok)) lastOkSeq = N(f.seq);
        blk.checks = checkBlock(blk, ev);
        if (blk.checks.some(c => !c.ok)) seg.blockIssues.push(blk.id);
        if (blk.closeT != null && ev.t - N(f.dur) > blk.closeT) seg.intervals.push({k:'wait', buf:b, s:blk.closeT, e:ev.t - N(f.dur), i:blk.closeIdx});
        seg.intervals.push({k:'flash', buf:b, s:ev.t - N(f.dur), e:ev.t, ok:B(f.ok), i:ev.i});
        break; }
      case 'CMD': seg.cmds.push({cmd:ev.i, resp:null}); break;
      case 'RESP': { const c = seg.cmds[seg.cmds.length - 1]; if (c && c.resp == null){ c.resp = ev.i; d.cmd = c.cmd; M.evs[c.cmd].d.resp = ev.i; } break; }
      case 'LOG_DROP': seg.logDrop += N(f.count) || 0; break;
    }
    apply(st, ev);
    if (ev.type === 'READ') d.after = {...st.rows[f.pid]};
    if (ev.i % CP_K === 0) m.cps[ev.i / CP_K] = clone(st);
    prev = ev;
  }
  finishSeg();
  for (const s of m.segs) s.intervals.sort((a,b) => a.s - b.s);
  return m;
}
function stateAt(i){
  const evs = M.evs, seg = M.segs[evs[i].seg];
  let k = Math.floor(i / CP_K), st, from;
  const cp = M.cps[k];
  if (cp && cp.seg === seg.id && k * CP_K >= seg.start){ st = clone(cp); from = k * CP_K + 1; }
  else { st = newState(seg.id); from = seg.start; }
  for (let j = from; j <= i; j++) apply(st, evs[j]);
  return st;
}
let stCache = {i:-1, st:null};
function curState(){ if (stCache.i !== sel) stCache = {i:sel, st:stateAt(sel)}; return stCache.st; }

/* ================= timers ================= */
function timers(st, now){
  const T = {}, c = st.config;
  T.running = st.mode ? st.mode.running : !!c;
  T.diag = st.mode ? st.mode.diag : false;
  if (c){
    const rows = st.order.map(p => st.rows[p]).filter(r => r && r.due != null);
    if (st.pending) T.reading = {pids:st.pending.pids, since:now - st.pending.t, left:st.pending.t + c.tmax - now, tmax:c.tmax, rule:st.pending.rule};
    if (rows.length){
      let e = rows[0]; for (const r of rows) if (r.due < e.due) e = r;
      T.earliest = {pid:e.pid, in:e.due - now};
      if (T.running && !st.pending){
        if (e.due <= now) T.wake = {in:0, rule:'due', pid:e.pid};
        else {
          let best = null;
          for (const r of rows){
            if (r.last == null) continue;
            const at = Math.max(now, r.last + r.pmin);
            if (at < e.due - c.tmax && (!best || at < best)) best = at;
          }
          if (best != null){
            const elig = rows.filter(r => r.last != null && best - r.last >= r.pmin);
            let pick = elig[0]; for (const r of elig) if (r.due < pick.due) pick = r;
            T.wake = {in:best - now, rule:'early', pid:pick.pid};
          } else T.wake = {in:e.due - now, rule:'due', pid:e.pid};
        }
      }
    }
  }
  const ob = st.bufs[st.openBuf];
  if (ob && ob.st === 'fill') T.block = {buf:st.openBuf, age:now - ob.t0, left:ob.t0 + DEADLINE - now, len:ob.len, recs:ob.recs};
  T.writing = st.bufs.map((b,k) => b.st === 'write' ? {buf:k, wait:now - b.closeT, len:b.len, reason:b.reason} : null).filter(Boolean);
  const sends = Object.entries(st.up.sends).map(([s,t]) => ({seq:+s, t})).sort((a,b) => a.t - b.t);
  if (sends.length) T.ack = {seq:sends[0].seq, left:sends[0].t + ACK_TIMEOUT - now, n:sends.length};
  if (st.health.heapT != null) T.health = {left:st.health.heapT + HEALTH_EVERY - now};
  return T;
}

/* ================= demo generator ================= */
function genDemo(){
  let seed = 20261019;
  const rnd = () => (seed = (seed * 1664525 + 1013904223) >>> 0) / 4294967296;
  const ri = (a,b) => a + Math.floor(rnd() * (b - a + 1));
  const le = (v,n) => Array.from({length:n}, (_,k) => (v >> (8*k)) & 255);
  const out = []; const E = (t, s) => out.push({t:Math.round(t), s});
  const T0 = 812, END = T0 + 95000, BOOTN = 3;
  let tmax = 50, seq = 128, rows = [];
  const PLAN1 = [['0C',250,125],['0D',250,125],['49',500,250],['04',500,250],['10',1000,500],['05',2000,1000],['2F',5000,2500]];
  const PLAN2 = [['0C',250,125],['0D',250,125],['49',500,250],['11',500,250],['04',500,250],['10',1000,500],['05',2000,1000],['2F',5000,2500]];
  const config = (t, reason, plan) => {
    const u = Math.round(plan.reduce((s,[,p]) => s + tmax/p, 0) * 1000);
    E(t, `CONFIG reason=${reason} proto=6 tmax=${tmax} u=${u} pids=${plan.map(x => x[0]).join(',')}`);
    rows = plan.map(([pid,p,pmin]) => ({pid, p, pmin, due:t, last:t - pmin}));
    for (const r of rows) E(t, `CVT pid=${r.pid} p=${r.p} pmin=${r.pmin} due=${r.due} last=${r.last} val=-`);
  };
  const car = (pid, t) => {
    const rel = (t - T0) / 1000;
    const rpm = 850 + 2300 * Math.max(0, Math.sin(rel / 9)) * (0.75 + 0.25 * Math.sin(rel * 1.7)) + ri(-12, 12);
    const pedal = clamp((rpm - 850) / 25, 0, 100), speedv = Math.max(0, Math.round(rpm / 30 - 28));
    const one = v => hx(clamp(Math.round(v),0,255),1), two = v => hx(clamp(Math.round(v),0,65535),2);
    switch (pid){
      case '0C': return two(rpm * 4); case '0D': return one(speedv); case '49': return one(pedal * 2.55);
      case '11': return one((pedal * 0.8 + 12) * 2.55); case '04': return one(Math.min(100, 20 + pedal * 0.7) * 2.55);
      case '10': return two((2 + rpm / 250) * 100); case '05': return one(Math.min(90, 45 + rel * 1.1) + 40);
      case '2F': return one((62 - rel * 0.01) * 2.55); default: return one(0);
    }
  };
  const engineOn = t => !(t - T0 >= 30000 && t - T0 < 43000);
  let bufSt = ['free','free'], bufFreeAt = [0,0], blk = null, flashBusy = 0, writeNo = 0, conn = false;
  const uploads = [];
  const closeBlock = (t, reason) => {
    if (!blk) return; const b = blk.buf;
    if (blk.recs === 0){ E(t, `BLK_RELEASE buf=${b}`); bufSt[b] = 'free'; blk = null; return; }
    E(t, `BLK_CLOSE buf=${b} reason=${reason} len=${blk.len} recs=${blk.recs} age=${t - blk.t0}`);
    bufSt[b] = 'write'; writeNo++;
    const start = Math.max(t + ri(0, 3), flashBusy);
    let dur = ri(14, 38), attempts = 1, ok = 1;
    if (writeNo === 3){ dur = 9500; attempts = 2; }       // slow flash -> both buffers busy -> drops
    if (writeNo === 9){ dur = ri(90, 130); attempts = 2; ok = 0; }
    const end = start + dur; flashBusy = end; bufFreeAt[b] = end;
    const bytes = [...le(seq,4), ...le(BOOTN,2), ...le(blk.t0,4), ...le(blk.len,2), ...blk.bytes];
    E(end, `FLUSH buf=${b} seq=${seq} boot=${BOOTN} len=${blk.len} ok=${ok} attempts=${attempts} dur=${dur} hex=${bytes.map(x => hx(x,1)).join('')}`);
    if (ok){ uploads.push({seq, t:end, len:blk.len}); seq++; }
    blk = null;
  };
  const freeBufs = t => { for (let b = 0; b < 2; b++) if (bufSt[b] === 'write' && bufFreeAt[b] <= t) bufSt[b] = 'free'; };
  const addRec = (t, rec) => {
    freeBufs(t);
    const size = rec.gps ? 15 : 5;
    if (blk && blk.len + size > BLOCK_MAX) closeBlock(t, 'full');
    if (!blk){
      const b = bufSt.indexOf('free');
      if (b < 0){ E(t, `REC_DROP kind=${rec.gps ? 'gps' : 'pid'} pid=${rec.gps ? '-' : rec.pid} reason=no_buffer`); return; }
      bufSt[b] = 'fill'; blk = {buf:b, t0:t, len:HDR, recs:0, bytes:[]}; E(t, `BLK_OPEN buf=${b} t0=${t}`);
    }
    const dt = t - blk.t0;
    if (rec.gps){
      blk.bytes.push(0xFF, ...le(dt,2), ...le(rec.lat,4), ...le(rec.lon,4), ...le(rec.alt,4)); blk.len += 15; blk.recs++;
      E(t, `REC kind=gps pid=- dt=${dt} val=- lat=${rec.lat} lon=${rec.lon} alt=${rec.alt} len=${blk.len}`);
    } else {
      const v2 = rec.val.length === 2 ? rec.val + '00' : rec.val;
      blk.bytes.push(parseInt(rec.pid,16), ...le(dt,2), parseInt(v2.slice(0,2),16), parseInt(v2.slice(2,4),16)); blk.len += 5; blk.recs++;
      E(t, `REC kind=pid pid=${rec.pid} dt=${dt} val=${v2} lat=- lon=- alt=- len=${blk.len}`);
    }
  };
  let now = T0 + 1, running = true, nextGps = T0 + 3000, nextCmd = T0 + 4200;
  const acts = [
    {at:T0 + 1500, fn:t => { E(t,'CONN up=1'); E(t + 40,'NET up=1'); conn = true; }},
    {at:T0 + 2100, fn:t => { E(t,`TIME_REQ boot=${BOOTN} t1=${t}`); E(t + 180,'TIME_RESP ok=1'); E(t + 181,`TIME_REPORT t1=${t} t2=${t + 180}`); }},
    {at:T0 + 55000, fn:t => {
      closeBlock(t, 'mode'); E(t, 'MODE diag=1 running=0'); running = false;
      for (let k = 0, c = t + 300; k < 14; k++, c += ri(380, 520)){
        const pid = ['0C','0D','05'][k % 3], d = ri(9, 22);
        E(c, `CMD txt="01${pid}"`); E(c + 1, `BUS kind=diag start=${c + 1} dur=${d} ok=1`);
        E(c + d + 2, `RESP txt="41 ${pid} ${car(pid, c).match(/../g).join(' ')}\\r\\r>" dur=${d + 2}`);
      }
    }},
    {at:T0 + 62000, fn:t => { E(t, 'MODE diag=0 running=1'); running = true; }},
    {at:T0 + 60000, fn:t => { for (const [k,v] of [['acquisition',1840],['flush',2210],['elm327',1630],['session',2950],['upload',1720],['logger',880]]) E(t, `STACK task=${k} free=${v}`); E(t, 'HEAP free=187432 min=171208'); }},
    {at:T0 + 75000, fn:t => {
      E(t, 'MODE diag=0 running=0');
      const pids = PLAN2.map(x => x[0]); E(t, `TMAX_START pids=${pids.join(',')} samples=3`);
      let c = t, mx = 0;
      for (const p of pids) for (let k = 0; k < 3; k++){ const l = ri(9, 31); c += l + 1; mx = Math.max(mx, l); E(c, `TMAX_SAMPLE pid=${p} ok=1 lat=${l}`); }
      tmax = Math.ceil(mx * 1.3); E(c + 1, `TMAX_DONE tmax=${tmax}`);
      closeBlock(c + 2, 'config'); config(c + 2, 'apply', PLAN2); E(c + 2, 'MODE diag=0 running=1');
      return c + 3;
    }},
  ].sort((a,b) => a.at - b.at);
  const doRead = (r, early) => {
    const start = now, ok = engineOn(start) && rnd() > 0.015;
    const dur = ok ? ri(7, 26) : tmax, t = start + dur, val = ok ? car(r.pid, t) : null;
    let nd; if (early) nd = t + r.p; else { nd = r.due + r.p; if (nd < t) nd = t; }
    r.due = nd; r.last = t;
    E(t, `READ pid=${r.pid} ok=${ok ? 1 : 0} val=${ok ? val : '-'} start=${start} dur=${dur} early=${early ? 1 : 0} next_due=${nd}`);
    now = t;
    if (ok) addRec(t, {pid:r.pid, val});
    now = t + (rnd() < 0.3 ? 1 : 0);
  };
  E(T0, 'FORMAT v=1');
  E(T0, `BOOT boot=${BOOTN} seq_next=${seq} nvs_last=127 log_oldest=65 log_newest=127 lost=0`);
  config(T0, 'boot', PLAN1); E(T0, 'MODE diag=0 running=1');
  while (now < END){
    while (acts.length && acts[0].at <= now){ const a = acts.shift(); const r = a.fn(now); if (r) now = Math.max(now, r); }
    if (now >= nextCmd){ E(now, 'CMD txt="ATRV"'); E(now + 1, 'RESP txt="12.6V\\r\\r>" dur=1'); nextCmd += 5000; }
    if (conn && running && now >= nextGps){
      const k = (now - T0) / 1000, lat = -235505200 + Math.round(k * 900), lon = -466333300 + Math.round(k * 640), alt = 760000 + ri(-500, 500);
      E(now, `GPS lat=${lat} lon=${lon} alt=${alt} t_rx=${now - ri(1, 4)}`); addRec(now, {gps:true, lat, lon, alt}); nextGps += 1000;
    }
    if (blk && now >= blk.t0 + DEADLINE) closeBlock(now, 'deadline');
    freeBufs(now);
    if (!running){ now = Math.max(now + 1, acts.length ? acts[0].at : END); continue; }
    const due = rows.filter(r => now >= r.due);
    if (due.length){ let r = due[0]; for (const x of due) if (x.due < r.due) r = x; E(now, `DECIDE rule=due pids=${r.pid} late=${now - r.due} slack=-`); doRead(r, false); continue; }
    let e = rows[0]; for (const x of rows) if (x.due < e.due) e = x;
    const elig = rows.filter(r => now - r.last >= r.pmin);
    if (e.due - now > tmax && elig.length){ let r = elig[0]; for (const x of elig) if (x.due < r.due) r = x; E(now, `DECIDE rule=early pids=${r.pid} late=- slack=${e.due - now}`); doRead(r, true); continue; }
    let w = e.due; for (const r of rows){ const at = r.last + r.pmin; if (at > now && at < e.due - tmax && at < w) w = at; }
    w = Math.min(w, nextCmd, conn ? nextGps : Infinity, blk ? blk.t0 + DEADLINE : Infinity, acts.length ? acts[0].at : Infinity);
    now = Math.max(now + 1, w);
  }
  uploads.forEach((u, k) => {
    const s = u.t + 4;
    E(s, `UP_SEND seq=${u.seq} oldest=65 len=${u.len}`);
    if (k === 4){ E(s + ACK_TIMEOUT, `UP_REWIND reason=timeout from=${u.seq}`); E(s + ACK_TIMEOUT + 2, `UP_SEND seq=${u.seq} oldest=65 len=${u.len}`); E(s + ACK_TIMEOUT + 240, `UP_ACK seq=${u.seq} advanced=1 deleted_upto=-`); }
    else E(s + ri(150, 420), `UP_ACK seq=${u.seq} advanced=1 deleted_upto=-`);
  });
  const sorted = out.map((o,k) => ({...o, k})).sort((a,b) => a.t - b.t || a.k - b.k);
  // simulate the logger losing 3 events while the engine is off
  let cut = sorted.findIndex(o => o.t > T0 + 36000 && o.s.startsWith('DECIDE'));
  sorted.splice(cut + 3, 0, {t:sorted[cut + 2].t, s:'LOG_DROP count=3'});
  sorted.forEach((o, n) => o.n = n);
  sorted.splice(cut, 3);
  const lines = ['ets Jun  8 2016 00:22:57', 'rst:0x1 (POWERON_RESET),boot:0x13 (SPI_FAST_FLASH_BOOT)', 'I (812) collector: debug events enabled'];
  sorted.forEach((o, k) => { lines.push(`EV ${o.n} ${o.t} ${o.s}`); if (k === 400) lines.push('I (9311) ble: notify mtu=247'); });
  return lines.join('\r\n') + '\r\n';
}

