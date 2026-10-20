# Collector debug events — format specification (v1)

This document specifies the debug event stream emitted by the TCCELTA dongle
firmware when built with collector debug events enabled. It is the contract
for any tool that parses the stream, in particular a step-by-step **viewer**
that reconstructs the collector's behaviour. A reader should be able to build
such a viewer from this document alone, without reading the firmware.

Status: specification agreed; firmware implementation pending.

## 1. Context in one page

The dongle is an ESP32 plugged into a car's OBD-II port. In **collector mode**
it reads a fixed list of OBD-II PIDs (engine RPM, speed, …) from the car's CAN
bus, one request at a time, and stores each answer as a **record** in a
**block** of at most 1024 bytes. Blocks are written to flash and later
uploaded to a server through a phone app.

Key concepts used by the events:

| Concept | Meaning |
|---|---|
| PID | One OBD-II parameter (1 byte id, e.g. `0C` = RPM). Values are 1–2 bytes. |
| `P` (period) | Maximum desired interval between two reads of a PID, in ms. |
| `Pmin` | Minimum interval between two reads of a PID, in ms. Below `P`, a PID may be read *early* if the bus is idle. |
| `Tmax` | Deadline for an answer from the car, in ms. A read without answer occupies the bus for `Tmax`. |
| Scheduler table (CVT) | Per PID: `p`, `pmin`, `due` (when it must be read next), `last` (when it was last read), `val` (last value). |
| Scheduler rules | 1. If any PID is due (`now ≥ due`), read the one most overdue. 2. Otherwise, if the earliest `due` is more than `Tmax` away, read early the eligible PID (`now − last ≥ pmin`) with the earliest `due`. 3. Otherwise wait. |
| Rescheduling | After a due read: `due += p` (if still in the past, `due = now`). After an early read: `due = now + p`. Always `last = now` (time the read finished). |
| Block | Header + records. Opened on its first record. Closed when full, 10 s after opening, or on mode/config change. |
| Double buffer | Two block buffers (`buf=0`, `buf=1`). One is being filled while the other may be being written to flash. If none is free, records are dropped. |
| `seq` | Sequence number of a block, assigned only when it is written to flash. Strictly increasing, no gaps unless blocks are lost. |
| `boot` | Boot counter, incremented on every restart. Timestamps restart from 0 on each boot. |
| Diagnostic mode | The app reads the car directly; the scheduler is suspended. |

## 2. Enabling

Compile-time only. The PlatformIO environments `collector_mock_debug` and
`collector_twai_debug` define `COLLECTOR_EVENTS`; without it, no event code is
compiled in. Debug builds use the serial port at **921600 baud**.

## 3. Line grammar

One event per line, terminated by `\r\n` (accept `\n` too). Only lines that
start with `EV ` are events; **ignore every other line** (the firmware also
prints free-form logs on the same port).

```
line    = "EV" SP n SP t SP EVENT *(SP field)
n       = 1*DIGIT                 ; event counter, starts at 0 on each boot
t       = 1*DIGIT                 ; ms since boot when the event happened
EVENT   = 1*(UPPER / "_")
field   = key "=" value
key     = 1*(LOWER / DIGIT / "_")
value   = token / quoted
token   = 1*(any char except SP, DQUOTE)
quoted  = DQUOTE *(char / escape) DQUOTE
escape  = BACKSLASH ( BACKSLASH / DQUOTE / "r" / "n" )   ; \\  \"  \r (CR)  \n (LF)
```

Value conventions (by field type in the catalog below):

| Type | Encoding | Example |
|---|---|---|
| `int` | Decimal, may be negative | `dur=12`, `lat=-235505200` |
| `ms` | Decimal ms since boot (same clock as `t`) | `next_due=12284` |
| `bool` | `0` or `1` | `ok=1` |
| `hex` | Uppercase hex, no `0x`, even length | `pid=0C`, `val=1AF8` |
| `list` | Comma-separated tokens, no spaces | `pids=0C,0D` |
| `str` | Quoted string | `txt="AT#PID 0C 250"` |
| `enum` | One of the listed lowercase words | `reason=deadline` |
| absent | `-` (value not available) | `val=-` |

Parsers must ignore unknown events and unknown fields (forward compatibility).
Fields of an event always appear in the order listed, but parsers should not
rely on it.

## 4. Ordering and timing semantics

- `n` is a single counter shared by all tasks, taken atomically when the event
  is emitted. **`n` order is the true order of events.** Events are printed by a
  low-priority logger task, so printing never delays the measured behaviour.
- `t` is captured at emission, not at printing. Events from the two CPU cores
  may have equal `t`; use `n` to order them.
- A gap in `n` (e.g. 41 followed by 45) means events were lost. The firmware
  also emits `LOG_DROP` when its queue overflows. A viewer must flag any gap;
  reconstructed state after a gap is unreliable until the next full snapshot
  (`CVT` lines, emitted at boot and on every config change).
- `FORMAT` resets everything: it marks a new boot (`n` and `t` restart at 0).
- Time resolution is 1 ms.

## 5. Event catalog

"Source" is where the firmware emits the event; it is informative only.

### 5.1 Boot and configuration

| Event | Fields | Meaning |
|---|---|---|
| `FORMAT` | `v:int` | First event of a boot. `v=1` for this spec. |
| `BOOT` | `boot:int` `seq_next:int` `nvs_last:int` `log_oldest:int` `log_newest:int` `lost:int` | Persistent counters at start. `seq_next = max(nvs_last, log_newest) + 1`. `log_oldest/log_newest` are 0 if the flash log is empty. `lost` = blocks dropped for lack of space so far (this boot). |
| `CONFIG` | `reason:enum(boot,apply)` `proto:int` `tmax:int` `u:int` `pids:list` | Active configuration. `proto` 6–9 (CAN 11/29 bit, 500/250 kbps). `u` = planned bus utilization **in permille** (`Σ tmax/p × 1000`, e.g. `782`). Followed by one `CVT` line per PID. |
| `CVT` | `pid:hex` `p:int` `pmin:int` `due:ms` `last:ms` `val:hex\|-` | Full snapshot of one scheduler table row. Emitted for every PID right after `CONFIG`. |

### 5.2 Scheduler and bus

| Event | Fields | Meaning |
|---|---|---|
| `DECIDE` | `rule:enum(due,early)` `pids:list` `late:int\|-` `slack:int\|-` | The scheduler decided to read. `rule=due`: rule 1, `late` = ms past the earliest `due` among `pids` (`slack=-`). `rule=early`: rule 2, `slack` = ms until the earliest `due` of the whole table (`late=-`). Waits are **not** logged. |
| `READ` | `pid:hex` `ok:bool` `val:hex\|-` `start:ms` `dur:int` `early:bool` `next_due:ms` | One PID was read. `t` = end of the read = the new `last` of that row. `start` = request sent, `dur = t − start` (bus occupancy). `ok=0` means no answer (`dur ≈ tmax`). `next_due` = new `due` of the row after rescheduling. One `READ` per PID in the preceding `DECIDE`. |
| `BUS` | `kind:enum(diag)` `start:ms` `dur:int` `ok:bool` | The bus was used for something other than a scheduled read (a diagnostic request from the app). |

### 5.3 Blocks (RAM)

| Event | Fields | Meaning |
|---|---|---|
| `BLK_OPEN` | `buf:int` `t0:ms` | A buffer was taken and a block opened. `t0` = block start time = time of its first record. |
| `REC` | `kind:enum(pid,gps)` `pid:hex\|-` `dt:int` `val:hex\|-` `lat:int\|-` `lon:int\|-` `alt:int\|-` `len:int` | A record was appended to the open block. `dt` = record time − `t0`; it can be **negative** for GPS (position received just before the block was opened). PID record: `pid`, `val` (always 2 bytes, 4 hex digits; 1-byte PIDs have `00` as second byte). GPS record: `lat`/`lon` in degrees × 10⁷, `alt` in mm. `len` = bytes used in the block after this record, header included. |
| `REC_DROP` | `kind:enum(pid,gps)` `pid:hex\|-` `reason:enum(no_buffer)` | A record could not be stored: no free buffer. |
| `BLK_CLOSE` | `buf:int` `reason:enum(full,deadline,config,mode,stop)` `len:int` `recs:int` `age:int` | The open block was closed. `age = t − t0`. `full`: the next record did not fit. `deadline`: 10 s elapsed. Others: closed by a configuration change, a mode change, or a restart. The block is handed to the flash writer. |
| `BLK_RELEASE` | `buf:int` | An empty block was closed and its buffer returned without writing. |

### 5.4 Flash

| Event | Fields | Meaning |
|---|---|---|
| `FLUSH` | `buf:int` `seq:int` `boot:int` `len:int` `ok:bool` `attempts:int` `dur:int` `hex:hex` | A block write finished. `t` = end of the write; `dur` = time spent writing (start = `t − dur`). `hex` = **the exact bytes written** (`len` bytes, header included, see §7). `ok=0`: both attempts failed, the block was discarded and `seq` was **not** consumed (the next `FLUSH` reuses the same `seq`). |
| `SEG_DROP` | `first:int` `last:int` `count:int` | Flash full: the oldest segment (blocks `first..last`) was deleted. These blocks are lost. |

### 5.5 Commands from the app (ELM327 text channel)

| Event | Fields | Meaning |
|---|---|---|
| `CMD` | `txt:str` | Command received (`AT…`, `AT#…`, OBD request like `010C`), as received without CR/LF. |
| `RESP` | `txt:str` `dur:int` | Response sent for the preceding `CMD`. CR/LF inside are escaped as `\r`/`\n` literally (backslash + letter). `dur` = processing time. |

### 5.6 Modes, GPS and Tmax measurement

| Event | Fields | Meaning |
|---|---|---|
| `MODE` | `diag:bool` `running:bool` | Mode changed. `running=1` = scheduler active. It is 0 in diagnostic mode, during Tmax measurement, and after a stop (before a restart). |
| `GPS` | `lat:int` `lon:int` `alt:int` `t_rx:ms` | A GPS position from the app reached the acquisition task. `t_rx` = when the command was received. Followed by `REC kind=gps` or `REC_DROP`. |
| `TMAX_START` | `pids:list` `samples:int` | Tmax measurement started (`samples` per PID). |
| `TMAX_SAMPLE` | `pid:hex` `ok:bool` `lat:int` | One measurement request. `lat` = response latency in ms (bus occupancy). |
| `TMAX_DONE` | `tmax:int` | Measurement finished; suggested Tmax in ms. |

### 5.7 Connection, upload and time sync

| Event | Fields | Meaning |
|---|---|---|
| `CONN` | `up:bool` | App connected / disconnected over BLE. |
| `NET` | `up:bool` | The app reported Internet available / unavailable. |
| `UP_SEND` | `seq:int` `oldest:int` `len:int` | A block was sent towards the server. `oldest` = oldest block still in flash. |
| `UP_ACK` | `seq:int` `advanced:bool` `deleted_upto:int\|-` | The server confirmed all blocks up to `seq`. `advanced=0`: stale ACK, ignored. |
| `UP_REWIND` | `reason:enum(timeout,disconnect)` `from:int` | No ACK in 5 s, or app disconnected: resend from `seq=from`. |
| `TIME_REQ` | `boot:int` `t1:ms` | Time sync request sent. |
| `TIME_RESP` | `ok:bool` | Time sync response received; `ok=0` if it did not match the pending request. |
| `TIME_REPORT` | `t1:ms` `t2:ms` | Round trip complete; sent to the server. |

### 5.8 Health

| Event | Fields | Meaning |
|---|---|---|
| `STACK` | `task:enum(acquisition,flush,elm327,session,upload,logger)` `free:int` | Every 60 s, one line per task: minimum free stack ever (bytes). |
| `HEAP` | `free:int` `min:int` | Every 60 s: current and minimum free heap (bytes). |
| `LOG_DROP` | `count:int` | `count` events were lost because the logger queue was full. Also visible as a gap in `n`. |

## 6. Reconstruction recipes

### 6.1 Scheduler table at any event

Start from the `CVT` lines after the latest `CONFIG`. Then for each `READ`:
`last = t`, `due = next_due`, and if `ok=1`, `val = val`. At any instant
`now`, a row is **due** if `now ≥ due` and **early-eligible** if
`now − last ≥ pmin`. Recomputing the rules from §1 on the reconstructed table
must yield the same `DECIDE`; a mismatch indicates a bug or lost events.

### 6.2 Early reads and calibration

- Early reads: `DECIDE rule=early` (or `READ early=1`).
- Lateness of due reads: distribution of `DECIDE late` per PID.
- Effective interval per PID: difference between `start` of consecutive
  `READ`s of the same PID. Guaranteed bound: `≤ p + Σ(tmax of the other PIDs)`.
- Effective sampling rate per PID vs. planned `1/p` and maximum `1/pmin`.

### 6.3 Bus utilization

- Planned: `CONFIG u / 1000`.
- Real, over a window `[a, b)`: sum of the parts of `[start, t]` of every
  `READ`, `BUS` and `TMAX_SAMPLE` (for `TMAX_SAMPLE`, the interval is
  `[t − lat, t]`) that fall in the window, divided by `b − a`.
- Idle time = window − busy time. The bus is never used by two things at once.

### 6.4 Block and flash behaviour

- Block lifetime: `BLK_OPEN` → `REC`s → `BLK_CLOSE` on the same `buf`.
- Filled before 10 s vs. closed by time: `BLK_CLOSE reason` with `len` and
  `age`. Fill rate = `len / age`.
- Write queueing: `(FLUSH.t − FLUSH.dur) − BLK_CLOSE.t` for the same `buf`
  (the next `FLUSH` with that `buf` after the `BLK_CLOSE`).
- Double-buffer pressure: both buffers between `BLK_CLOSE`/`BLK_OPEN` and
  `FLUSH` at once; `REC_DROP reason=no_buffer` when it overflows.
- **Content check:** decode `FLUSH hex` (§7) and compare against the `REC`
  events of that block, in order: same count, `kind`, `pid`, `dt`, `val`
  (or `lat/lon/alt`), and header `t_ms start = t0`, `length = len`. Also
  check that each `REC kind=pid` matches the `READ` that produced it
  (`pid`, `val`, and `t0 + dt = READ.t`).
- `seq` continuity: successful `FLUSH`es have `seq` = previous + 1 within a
  boot; across boots, the first `FLUSH` has `seq = BOOT.seq_next`.

### 6.5 Upload

The server's progress is the latest `UP_ACK advanced=1 seq`. Blocks in flight
= sent (`UP_SEND`) but not yet covered by an ACK. Data lost to full flash:
`SEG_DROP` ranges.

## 7. Block binary format (for `FLUSH hex`)

Little-endian. All offsets in bytes.

```
Header (12 bytes)
  0  seq        u32
  4  boot_id    u16
  6  t_ms_start u32   (= BLK_OPEN.t0)
 10  length     u16   (total block length, header included = FLUSH.len)

Records, back to back until `length`:
  PID record (5 bytes):  type u8 (= PID, 0x00–0xFE) | dt i16 | value 2 bytes
  GPS record (15 bytes): type u8 (= 0xFF)           | dt i16 | lat i32 | lon i32 | alt i32
```

`dt` is signed ms since `t_ms_start` (two's complement); a GPS record may be
slightly earlier than the block start. Record type `0xFF` is GPS; any other value is a
PID record.

## 8. Example

```
EV 0 812 FORMAT v=1
EV 1 812 BOOT boot=3 seq_next=128 nvs_last=127 log_oldest=65 log_newest=127 lost=0
EV 2 812 CONFIG reason=boot proto=6 tmax=50 u=782 pids=0C,0D,49,04,10,05,2F
EV 3 812 CVT pid=0C p=250 pmin=125 due=812 last=687 val=-
EV 4 812 CVT pid=0D p=250 pmin=125 due=812 last=687 val=-
...
EV 10 813 DECIDE rule=due pids=0C late=1 slack=-
EV 11 825 READ pid=0C ok=1 val=1AF8 start=813 dur=12 early=0 next_due=1062
EV 12 825 BLK_OPEN buf=0 t0=825
EV 13 825 REC kind=pid pid=0C dt=0 val=1AF8 lat=- lon=- alt=- len=17
EV 14 826 DECIDE rule=due pids=0D late=14 slack=-
EV 15 876 READ pid=0D ok=0 val=- start=826 dur=50 early=0 next_due=1062
EV 16 990 CMD txt="010C"
EV 17 991 RESP txt="41 0C 1A F8\r>" dur=1
EV 18 1001 DECIDE rule=early pids=04 late=- slack=61
...
EV 230 10825 BLK_CLOSE buf=0 reason=deadline len=612 recs=120 age=10000
EV 231 10849 FLUSH buf=0 seq=128 boot=3 len=612 ok=1 attempts=1 dur=24 hex=80000000030039030000640...
```

## 9. Limits

- Events lost to `LOG_DROP` cannot be recovered; the viewer must show the gap.
- Waits are not logged; the viewer infers them from the reconstructed table.
- Resolution is 1 ms; reads shorter than 1 ms show `dur=0`.
- Timestamps are relative to boot. Absolute time comes only from the server's
  time sync (`TIME_REPORT`), which the dongle does not log in absolute terms.
