# ELM327 layer

`Elm327` is the protocol boundary after secure BLE validation. It accepts a
small local AT subset and whitelisted OBD requests; only OBD requests call
`IObd2`. `Elm327Task` serializes commands and sends exactly one response ending
in `>` before the next queued command is processed.

```mermaid
sequenceDiagram
    participant App
    participant Secure as Secure BLE
    participant Task as Elm327Task
    participant ELM as Elm327
    participant OBD as IObd2
    App->>Secure: authenticated command
    Secure->>Task: plaintext command
    Task->>ELM: process(command)
    ELM->>ELM: parse AT or OBD
    ELM->>OBD: whitelisted request
    OBD-->>ELM: one or more ECU responses
    ELM-->>Task: formatted response + >
    Task-->>Secure: response
    Secure-->>App: authenticated response
```

OBD syntax is `SSPP` or `SSPPN`, and service-only DTC syntax is `SS` or
`SSN`; `N` is the optional number of complete ECU responses expected. With no
`N`, the OBD layer waits for its configured quiet timeout. `ATSThh` controls
that timeout in 4 ms units (`ATST32` is 200 ms). It does not change ISO-TP
per-frame deadlines.

`ATH1` reports the CAN ID captured from each ECU. The AT allowlist is local and
does not permit address injection, raw transmission, or fabricated
voltage/status commands.

With the collector (`USE_COLLECTOR`), `Elm327` also receives an
`ICollectorControl`: `ATSPh` selects one of the four ISO 15765-4 protocols
(6–9) and restarts the dongle, `ATSThh` sets Tmax, and the `AT#` commands
(`PID`, `DEL`, `LIST`, `DIAG`, `NET`, `GPS`, `TMAX`, `TMAX?`) configure the
collection. Outside diagnostic mode, collected PIDs are answered from the last
value read, without touching the bus; any other request answers `?`.
