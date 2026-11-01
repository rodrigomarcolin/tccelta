# Collector debug events — format specification (v2)

This document specifies the debug event stream emitted by the TCCELTA dongle
firmware when built with collector debug events enabled. It is the contract
for any tool that parses the stream, in particular a step-by-step **viewer**
that reconstructs the collector's behaviour. A reader should be able to build
such a viewer from this document alone, without reading the firmware.

Status: v2 implemented in the firmware. v2 adds the security and SYNC events
(spec `fase-1/06`), microsecond durations in `*_us` fields, and the block
format with marker records, tag and CRC. Parsers should accept `FORMAT v=1` and
`v=2`; `UP_*` and `TIME_*` (v1) no longer exist.

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
| Block | Header + records. Opened on its first record. Closed when full, `flush` ms after opening (10 s by default, `CONFIG.flush`), or on mode/config change. |
| Double buffer | Two block buffers (`buf=0`, `buf=1`). One is being filled while the other may be being written to flash. If none is free, records are dropped. |
| `seq` | Sequence number of a block, assigned only when it is written to flash. Strictly increasing, no gaps unless blocks are lost. |
| `boot` | Boot counter, incremented on every restart. Timestamps restart from 0 on each boot. |
| Diagnostic mode | The app reads the car directly; the scheduler is suspended. |

## 2. Enabling

Compile-time only. The PlatformIO environments `collector_mock_debug`,
`collector_twai_debug` and `collector_mcp2515_debug` define `COLLECTOR_EVENTS`;
without it, no event code is compiled in. Debug builds use the serial port at
**921600 baud**.

Two bench aids interact with the stream:

- `COLLECTOR_WIPE_ON_BOOT` (build flag, off by default): every boot starts
  with an empty flash log and `seq` back at 1; `boot` keeps increasing.
  Visible as `BOOT wiped=1`.
- `AT#ACK n` (debug builds only): injects an ACK with a valid tag, as if it came from the app, so the session can open and flash be freed without the app. Also `AT#SYNC?` (engine state), `AT#CORRUPT n` (corrupt block n on flash) and `AT#WIPE` (wipe at next boot). `COLLECTOR_EVENTS_NO_HEX` (benchmark envs `*_bench_*`) drops `hex`/`tag` from `FLUSH`; `SECURITY_DEV_FIXED_KEYS` (debug builds) uses the bench keys when the `secnvs` partition is not provisioned.

  It appears as `CMD txt="AT#ACK n"` followed by `HS_ACK`.

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
| `FORMAT` | `v:int` | First event of a boot. `v=2` for this spec. |
| `KEYS` | `profile:enum(none,hmac,asym)` `source:enum(partition,dev_fixed,mac,missing)` `dev_id:hex` `tag_len:int` `fe:enum(off,dev,release)` `sb:int` `load_us:us` | Keys loaded from the `secnvs` partition. `tag_len` = block tag size of the profile (0/32/64). Never shows key material. |
| `BOOT` | `boot:int` `wiped:bool` `rsv:int` `rsv_new:int` `legacy_last:int\|-` `boot_seq:int` `from:int` `to:int` `seq_next:int` `log_oldest:int` `log_newest:int` `lost:int` `log_max:int` | Seq recovery. `rsv` = reserve read from NVS (0 = absent), `rsv_new` = reserve written for the BOOT block, `boot_seq` = `R`, `[from,to]` = range declared by the BOOT marker (empty if `to = from - 1`), `seq_next` = next seq after the BOOT. `log_max` = block-count limit N of the log (margin or `BLOCK_LOG_MAX_BLOCKS`). `wiped=1`: log, `seq_rsv` and `last_cs` were erased (bench builds). |
| `LOG_MIGRATE` | `removed:int` | Files of the v1 log (`/log`) deleted at the first boot of the v2 firmware. |
| `CONFIG` | `reason:enum(boot,apply)` `proto:int` `tmax:int` `flush:int` `u:int` `pids:list` | Active configuration. `proto` 6–9 (CAN 11/29 bit, 500/250 kbps). `flush` = block deadline in ms (1000–30000, default 10000; absent in older logs = 10000). `u` = planned bus utilization **in permille** (`Σ tmax/p × 1000`, e.g. `782`). Followed by one `CVT` line per PID. |
| `CVT` | `pid:hex` `p:int` `pmin:int` `due:ms` `last:ms` `val:hex\|-` | Full snapshot of one scheduler table row. Emitted for every PID right after `CONFIG`. |

### 5.2 Scheduler and bus

| Event | Fields | Meaning |
|---|---|---|
| `DECIDE` | `rule:enum(due,early)` `pids:list` `late:int\|-` `slack:int\|-` | The scheduler decided to read. `rule=due`: rule 1, `late` = ms past the earliest `due` among `pids` (`slack=-`). `rule=early`: rule 2, `slack` = ms until the earliest `due` of the whole table (`late=-`). Waits are **not** logged. |
| `READ` | `pid:hex` `ok:bool` `val:hex\|-` `start:ms` `dur:int` `early:bool` `next_due:ms` | One PID was read. `t` = end of the read = the new `last` of that row. `start` = request sent, `dur = t − start` (bus occupancy). `ok=0` means no answer (`dur ≈ tmax`). `next_due` = new `due` of the row after rescheduling. One `READ` per PID in the preceding `DECIDE`. |
| `BUS` | `kind:enum(diag)` `start:ms` `dur:int` | The bus was used for something other than a scheduled read (a diagnostic request from the app). Whether the car answered is in the following `RESP`. |
| `CAN_STATUS` | `state:enum(running,bus_off,recovering,stopped)` `tec:int` `rec:int` `tx_failed:int` `tx_drop:int` `tx_q:int` `rx_q:int` `rx_missed:int` `rx_overrun:int` `arb_lost:int` `bus_err:int` | CAN controller state, emitted right after every request that got no answer (scheduled read, diagnostic request or Tmax sample). TWAI builds only; the MCP2515 backend does not report it. `tec`/`rec` = transmit/receive error counters (`tec ≥ 128` = error passive). `tx_failed`, `rx_missed`, `rx_overrun`, `arb_lost`, `bus_err` = counters since boot. `tx_drop` = `send()` refused locally because the TX queue was full. `tx_q`/`rx_q` = frames waiting in the TX/RX queues. Reading: `tec=128` with `tx_q > 0` = nobody ACKs (no node listening, wrong bitrate or wiring); `bus_off` = bus errors (bitrate, CANH/CANL, termination); counters at 0 = the bus is fine and the ECU simply did not answer. |

### 5.3 Blocks (RAM)

| Event | Fields | Meaning |
|---|---|---|
| `BLK_OPEN` | `buf:int` `t0:ms` | A buffer was taken and a block opened. `t0` = block start time = time of its first record. |
| `REC` | `kind:enum(pid,gps)` `pid:hex\|-` `dt:int` `val:hex\|-` `lat:int\|-` `lon:int\|-` `alt:int\|-` `len:int` | A record was appended to the open block. `dt` = record time − `t0`; it can be **negative** for GPS (position received just before the block was opened). PID record: `pid`, `val` (always 2 bytes, 4 hex digits; 1-byte PIDs have `00` as second byte). GPS record: `lat`/`lon` in degrees × 10⁷, `alt` in mm. `len` = bytes used in the block after this record, header included. |
| `REC_DROP` | `kind:enum(pid,gps)` `pid:hex\|-` `reason:enum(no_buffer)` | A record could not be stored: no free buffer. |
| `BLK_CLOSE` | `buf:int` `reason:enum(full,deadline,config,mode,stop)` `len:int` `recs:int` `age:int` | The open block was closed. `age = t − t0`. `full`: the next record did not fit. `deadline`: `CONFIG.flush` ms elapsed. Others: closed by a configuration change, a mode change, or a restart. The block is handed to the flash writer. |
| `BLK_RELEASE` | `buf:int` | An empty block was closed and its buffer returned without writing. |

### 5.4 Flash

| Event | Fields | Meaning |
|---|---|---|
| `FLUSH` | `buf:int\|-` `kind:enum(data,boot)` `seq:int` `boot:int` `len:int` `tag_len:int` `ok:bool` `attempts:int` `reserved:bool` `reserve_us:us` `sign_us:us` `crc_us:us` `write_us:us` `dur:int` `crc:hex` `hex:hex\|-` `tag:hex\|-` | A block write finished. `t` = end of the write; `dur` (ms) = total time. `reserved=1`: `seq_rsv` was written to NVS first (`reserve_us`). `hex` = **the exact data bytes written** (`len` bytes, see §7), `tag` = the `tag_len` bytes of the tag, `crc` = CRC32 of data and tag as an 8-digit big-endian value (stored little-endian on flash). Builds with `COLLECTOR_EVENTS_NO_HEX` print `hex=-` and `tag=-`. `ok=0`: both attempts failed, the block was discarded and `seq` was **not** consumed. |
| `MARKER` | `kind:enum(boot,loss)` `seq:int` `from:int` `to:int` `reason:enum(boot,crc,missing)` | A marker block was built: BOOT at boot (before its `FLUSH kind=boot`), LOSS at send time (never stored). |
| `SEG_NEW` | `first:int` `tag_len:int` `reason:enum(first,full,boot,sealed,tag_len)` | A new log segment was opened. |
| `SEG_SEAL` | `first:int` `last:int` `reason:enum(write_fail,scan_tail)` | A segment stopped accepting writes (write failure or invalid tail found at boot). |
| `SEG_DROP` | `first:int` `last:int` `count:int` | Block limit or flash full: the oldest segment (blocks `first..last`) was deleted. These blocks are lost; `overflowUpTo` becomes `last`. |

### 5.5 Commands from the app (ELM327 text channel)

| Event | Fields | Meaning |
|---|---|---|
| `CMD` | `txt:str` | Command received (`AT…`, `AT#…`, OBD request like `010C`), as received without CR/LF. |
| `RESP` | `txt:str` `dur:int` | Response sent for the preceding `CMD`. CR/LF inside are escaped as `\r`/`\n` literally (backslash + letter). `dur` = processing time. |

### 5.6 Modes, GPS and Tmax measurement

| Event | Fields | Meaning |
|---|---|---|
| `MODE` | `diag:bool` `running:bool` | Mode changed. `running=1` = scheduler active. It is 0 in diagnostic mode, during Tmax measurement, and after a stop (before a restart). |
| `GPS` | `lat:int` `lon:int` `alt:int` `t_rx:ms` | A GPS position from the app was accepted. `t_rx` = when the command was received. Followed by `REC kind=gps` or `REC_DROP`. GPS only exists while the app is connected (the app sends it). |
| `GPS_DROP` | `reason:enum(diag,rate,queue)` `t_rx:ms` | A GPS position was discarded: `diag` = diagnostic mode; `rate` = less than 1 s (minus 100 ms of tolerance) after the last accepted one; `queue` = the acquisition task's queue was full. |
| `TMAX_START` | `pids:list` `samples:int` | Tmax measurement started (`samples` per PID). |
| `TMAX_SAMPLE` | `pid:hex` `ok:bool` `lat:int` | One measurement request. `lat` = response latency in ms (bus occupancy). |
| `TMAX_DONE` | `tmax:int` | Measurement finished; suggested Tmax in ms. |

### 5.7 Connection, SYNC and handshake

Durations in `*_us` are microseconds from the same clock as `t`; they never
include event formatting.

| Event | Fields | Meaning |
|---|---|---|
| `CONN` | `up:bool` `gen:int` | Authorized session opened / closed (encrypted link with a valid LESC bond, see 5.9); `gen` = connection generation. |
| `NET` | `up:bool` | The app reported Internet available / unavailable (informational). |
| `SYNC_SUB` | `on:bool` | Notifications on SYNC_TX enabled / disabled. |
| `SYNC_MTU` | `mtu:int` | Negotiated MTU changed. |
| `SYNC_RX` | `type:hex\|-` `len:int` `q_us:us` `ok:bool` `reason:enum(ok,short,bad_len,unexpected,queue_full)` | Message received on SYNC_RX. `queue_full` (with `type=-`, `len` = messages lost) is emitted by the sync task from a counter. |
| `SYNC_TX` | `type:hex` `len:int` `frags:int` `mtu:int` `dur_us:us` `retries:int` `ok:bool` | A message was sent (all notifications). |
| `SYNC_ERR` | `dir:enum(tx,rx)` `code:hex` | Error message (0x06) sent to or received from the app. |
| `HS_CHALLENGE` | `c_s:int` `last_cs:int` `ack:int` `c_conf:int` `flags:hex` `diag_min:int\|-` `len:int` `verify_us:us` `result:enum(ok,bad_len,bad_tag,old_cs,nvs_fail)` | Challenge processed. |
| `HS_RESPONSE` | `c_s:int` `tail:int` `head:int` `boot:int` `t_ms:ms` `deleted_upto:int\|-` `nvs_us:us` `tag_us:us` `total_us:us` `q_us:us` | Response sent. `t_ms` is the time anchor. |
| `HS_ACK` | `ack:int` `c_conf:int` `flags:hex` `verify_us:us` `result:enum(ok,bad_len,bad_tag,bad_seq)` `advanced:bool` `opened:bool` `anchor_ok:bool` | ACK processed. `opened=1` opened the session. |
| `SYNC_SESSION` | `open:bool` `reason:enum(connect,ack,overflow,disconnect)` | Session state changed. |
| `SYNC_NOTICE` | `head:int` `flags:hex` `reason:enum(subscribe,flush,ack,resync,overflow,flash,retry)` | New-block notice (0x07) sent. |
| `SYNC_REQ` | `k:int` `cursor:int` `head:int` `end:int\|-` `result:enum(ok,no_session,empty,ignored)` | Batch request received. |
| `SYNC_BLOCK` | `seq:int` `kind:enum(data,boot,loss)` `len:int` `tag_len:int` `read_us:us` `crc_us:us\|-` `crc_ok:bool\|-` `sign_us:us\|-` `tx_us:us` `frags:int` `mtu:int` `cursor:int` | One block sent; `cursor` is the value after the send. |
| `SYNC_BATCH_END` | `reason:enum(done,link,overflow,app_error,superseded,empty)` `sent:int` `bytes:int` `dur_us:us` | End of a batch. |
| `SYNC_CURSOR` | `from:int` `to:int` `reason:enum(connect,ack,app_error,boot_gap,loss)` | Cursor jump outside the normal +1 per block. |
| `SYNC_FREE` | `upto:int` `segs:int` `oldest:int` `dur_us:us` | Flash freed after an ACK or challenge. |
| `SYNC_OVERFLOW` | `upto:int` `ack:int` | Flash-full discard reached unconfirmed blocks; session invalidated. |

### 5.8 Health

| Event | Fields | Meaning |
|---|---|---|
| `STACK` | `task:enum(acquisition,flush,elm327,sync,pairing,logger)` `free:int` | Every 60 s, one line per task: minimum free stack ever (bytes). |
| `HEAP` | `free:int` `min:int` | Every 60 s: current and minimum free heap (bytes). |
| `LOG_DROP` | `count:int` | `count` events were lost because the logger queue was full. Also visible as a gap in `n`. |


### 5.9 Pairing and link security

Emitted by the LESC Just Works link layer (`PairButton`, `PairingPolicy`,
`PairingController`, `BleConnectivity`, `BleSecurity`). `conn` is the NimBLE
connection handle (the same in all events of a connection; it may be reused
after `LINK_DOWN`). New events are additive: `FORMAT v` does not change.

| Event | Fields | Meaning |
|---|---|---|
| `PAIR_BTN` | `action:enum(press,hold,release,blocked)` `held:int` `fired:bool` | Pairing button transition. `press`: debounced press (`held=0`). `hold`: `PAIR_BUTTON_HOLD_MS` reached (`held` = real time, `fired=1`). `release`: `held` = total duration, `fired=1` if the press fired. `blocked`: the firmware booted with the button pressed and ignores it until released. |
| `PAIR_WINDOW` | `state:enum(open,close)` `reason:enum(boot,button,timeout,bonded,error)` `deadline:ms\|-` `dur:int\|-` | Pairing window. `open` (always `reason=button`): `deadline` = instant it closes. `close`: `dur` = time it stayed open (`-` for `reason=boot`). `bonded`: a phone paired. `error`: it did not open because erasing the bonds failed (`dur=0`). |
| `BOND_DELETE` | `reason:enum(button,policy,recover)` `count:int` `ok:bool` `attempts:int` `dur:int` | Bonds erased. `policy`: boot hygiene (more than one bond or a legacy bond, `dur` = time spent) or the bond of a refused connection (`count=1`, decided by the policy, `dur=0`). `recover`: the whole `nimble_bond` namespace is erased before a restart. |
| `BOND_VERIFY` | `conn:int` `ok:bool` `lesc:bool` `persisted:bool` | New bond checked in the store right after pairing. `ok=0` refuses the connection and keeps the window open. |
| `NVS` | `free:int` `used:int` `total:int` | Entries of the `nvs` partition at boot (shared by bonds and collection). |
| `CONN_DROP` | `reason:enum(button)` `count:int` | All connections dropped (`0x13`) for a new pairing. Each one still produces its `LINK_DOWN`. |
| `ADV` | `mode:enum(off,normal,pairing)` `window:bool` `bonded:bool` | Advertising reconfigured; only when something changes. `window`/`bonded` = bits of the manufacturer data (`FF FF 01 flags`). |
| `LINK_UP` | `conn:int` `addr:hex` `atype:enum(pub,rnd)` `window:bool` | Link accepted, **not yet** authorized. `addr` = over-the-air address (display order, no `:`; a rotating RPA on phones). `window=1` if the window was open. |
| `LINK_ENC` | `conn:int` `up:bool` `bonded:bool` `fresh:bool` `key:int` `reason:enum(ok,pairing,other)` `rc:int` `dur:int` | Encryption result (`ENC_CHANGE`). `fresh=1`: new key (pairing); `0`: restored LTK. `up=0`: `pairing` = pairing failed (see `PAIR_RESULT`), `other` = anything else. Restoring without an LTK on the dongle emits nothing. `dur` = `t - LINK_UP.t`. |
| `PAIR_RESULT` | `conn:int` `ok:bool` `reason:enum(ok,window_closed,not_bonded,legacy,not_persisted,busy,peer_reject,local_reject,timeout,other)` `late:bool` `rc:int` `dur:int` | End of a pairing attempt that reached the SM (not emitted for restores nor for requests refused in `BOND_FULL`). `ok=1` only with a LESC bond created in the window and confirmed in the store. `late=1`: admitted in the window, finished after it closed. `dur` = pairing time (includes the OS dialog). |
| `CONN_REJECT` | `conn:int` `reason:enum(not_bonded,enc_fail,key_size,auth_timeout,busy)` `hci:hex` `dur:int` | The dongle drops the connection. `hci` = reason sent (`05` refusal, `09` busy). |
| `LINK_DOWN` | `conn:int` `rc:int` `hci:hex\|-` `auth:bool` `dur:int` | Link down. `rc` = raw NimBLE reason; `hci` = `rc - 0x200` for HCI errors (`13` remote, `16` local, `08` timeout). `auth=1` if it was the authorized session. |
| `WRITE_DROP` | `conn:int` `chr:enum(nus_rx,sync_rx)` `len:int` | Write from an unauthorized connection, discarded. |
| `BOND_FULL` | `event:enum(full,overflow)` `action:enum(reject,allow)` `obj:int` `conn:int\|-` `window:bool` | `full`: a Pairing Request arrived with the store full (any request when a bond exists, `MAX_BONDS=1`); `reject` outside the window (the existing bond is untouched), `allow` inside it; `obj` is `2` (peer_sec). `overflow`: attempt to write a second bond; always `reject`, `obj` = NimBLE object type (`1` our_sec, `2` peer_sec). |
| `LINK_OPEN` | `warn:enum(no_lesc)` | Bench-only build (`LINK_SECURITY_OPEN`, env `collector_mock_debug_open`): LESC and bonds are off and every connection is authorized on connect (`CONN up=1` right after `LINK_UP`); no other pairing event appears. Emitted once at boot. The build fails with Secure Boot / flash encryption (devsec, prod). |

`CONN` now means an **authorized session** (encrypted link with a valid LESC
bond) opened/closed; a refused connection never produces `CONN`.

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

### 6.5 Session and handshake

Per `gen` (from `CONN up=1` to the next `CONN up=0`): `SYNC_SUB`,
`SYNC_NOTICE` (flags bit 0), `HS_CHALLENGE result=ok`, `HS_RESPONSE`,
`HS_ACK opened=1`, `SYNC_SESSION open=1`. Every `HS_RESPONSE.c_s` is greater
than the `last_cs` of the preceding `HS_CHALLENGE`; no `SYNC_BLOCK` precedes
`SYNC_SESSION open=1`; every `SYNC_REQ result=no_session` is followed by
`SYNC_ERR dir=tx code=05`.

### 6.6 Cursor

Per `gen`, the cursor starts at `ack + 1` (`SYNC_CURSOR reason=connect`); each
`SYNC_BLOCK.seq` equals the reconstructed cursor and sets it to `seq + 1`
(data/BOOT) or to `to + 1` of the preceding `MARKER kind=loss`; jumps only
through `SYNC_CURSOR`.

### 6.7 `seq` audit

Over the successful `FLUSH`es and the `BOOT` events: `seq` is strictly
increasing; the first `FLUSH` of each boot is `kind=boot seq=boot_seq`; every
integer from 1 to the last `seq` lies in a successful `FLUSH`, in a BOOT range
`[from,to]` or in a `MARKER kind=loss` range. Losses by full flash appear as
`SEG_DROP`.

### 6.8 Benchmark metrics

`tools/event-bench/bench_report.py` computes, from `FLUSH`, `SYNC_BLOCK`,
`HS_*`, `SYNC_TX` and `STACK`, the cost of signing and CRC per block, the
handshake cost at the dongle, the BLE throughput per MTU, the fragmentation
overhead and the confirmation latency per `seq`, for the `none`, `hmac` and
`asym` builds (`collector_*_bench_*` envs).


### 6.9 Pairing and link security

- **Pairing time:** `PAIR_RESULT ok=1 dur`. Since the button: `PAIR_RESULT.t - PAIR_BTN(action=hold).t`.
- **Reconnect with bond:** `LINK_ENC fresh=0 up=1 dur`; until the session: `CONN(up=1).t - LINK_UP.t` of the same connection.
- **Window usage:** `PAIR_WINDOW open`/`close` pairs; `reason=timeout` without `PAIR_RESULT ok=1` in between is a wasted window.
- **Improper attempts:** `CONN_REJECT` by `reason`; `WRITE_DROP` by `conn`.
- **Availability (A1.5):** sum of `dur` of `LINK_DOWN auth=0` = time strangers held slots.

## 7. Block binary format (for `FLUSH hex`)

Little-endian. All offsets in bytes.

```
Data area (`tamanho` bytes, header included = FLUSH.len)
Header (12 bytes)
  0  seq        u32
  4  boot_id    u16   (0 in LOSS)
  6  t_ms_start u32   (= BLK_OPEN.t0; 0 in LOSS)
 10  tamanho    u16

Records, back to back until `tamanho`:
  PID record (5 bytes):    type u8 (0x00-0xFC = PID) | dt u16 | value 2 bytes
  LOSS marker (11 bytes):  0xFD | dt u16 | from_seq u32 | to_seq u32
  BOOT marker (11 bytes):  0xFE | dt u16 | from_seq u32 | to_seq u32
  GPS record (15 bytes):   0xFF | dt u16 | lat i32 | lon i32 | alt i32
```

`dt` is unsigned ms since `t_ms_start` (0-65535). A record earlier than the
block start closes the block and opens another, so `t_ms_start` is not
increasing between consecutive blocks. A marker block has a single 11-byte
record (`tamanho = 23`).

On flash each record is `data(tamanho) | tag(T) | CRC32 LE` with `T` = 0, 32
or 64 by security profile and the CRC over data and tag; segment files start
with `"TCS2" | version 2 | tag_len | reserved u16`. The tag is
`HMAC-SHA256_{K_rec}("MALCONG" || 0x03 || DevId || data)` (profile `hmac`),
an ECDSA P-256 `r||s` over the SHA-256 of the same input (`asym`), or empty
(`none`).

Over BLE the SYNC service carries TLV messages `T(1) | L(2 LE) | V`; SYNC_TX
notifications are `C | piece of TLV` with `C` = bit 7 (first fragment) | counter
mod 128, and a piece of `MTU - 4` bytes.

## 8. Example

```
EV 0 812 FORMAT v=1
EV 1 812 BOOT boot=3 wiped=0 seq_next=128 nvs_last=127 log_oldest=65 log_newest=127 lost=0
EV 2 812 CONFIG reason=boot proto=6 tmax=50 flush=10000 u=782 pids=0C,0D,49,04,10,05,2F
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


New pairing through the button (Android):

```
EV 120 30112 PAIR_BTN action=press held=0 fired=0
EV 121 35113 PAIR_BTN action=hold held=5021 fired=1
EV 122 35113 ADV mode=off window=0 bonded=1
EV 123 35114 CONN_DROP reason=button count=1
EV 124 35131 BOND_DELETE reason=button count=1 ok=1 attempts=1 dur=17
EV 125 35131 PAIR_WINDOW state=open reason=button deadline=80131 dur=-
EV 126 35132 ADV mode=pairing window=1 bonded=0
EV 127 35190 LINK_DOWN conn=1 rc=534 hci=16 auth=1 dur=812345
EV 128 35191 CONN up=0 gen=2
EV 130 41220 LINK_UP conn=2 addr=5A1F3C2B9D44 atype=rnd window=1
EV 131 44877 LINK_ENC conn=2 up=1 bonded=1 fresh=1 key=16 reason=ok rc=0 dur=3657
EV 132 44877 BOND_VERIFY conn=2 ok=1 lesc=1 persisted=1
EV 133 44877 PAIR_RESULT conn=2 ok=1 reason=ok late=0 rc=0 dur=3657
EV 134 44877 PAIR_WINDOW state=close reason=bonded deadline=- dur=9746
EV 135 44877 ADV mode=off window=0 bonded=1
EV 136 44878 CONN up=1 gen=3
```

Stranger outside the window while a bond exists (refused in the store):

```
EV 280 70100 LINK_UP conn=2 addr=4C9E01D2A7B3 atype=rnd window=0
EV 281 71620 BOND_FULL event=full action=reject obj=2 conn=2 window=0
EV 282 75100 CONN_REJECT conn=2 reason=auth_timeout hci=05 dur=5000
EV 283 75140 LINK_DOWN conn=2 rc=534 hci=16 auth=0 dur=5040
```

## 9. Limits

- Events lost to `LOG_DROP` cannot be recovered; the viewer must show the gap.
- Waits are not logged; the viewer infers them from the reconstructed table.
- Resolution is 1 ms; reads shorter than 1 ms show `dur=0`.
- Timestamps are relative to boot. Absolute time comes only from the server's
  the handshake response (`HS_RESPONSE t_ms`), which the dongle does not log in absolute terms.
