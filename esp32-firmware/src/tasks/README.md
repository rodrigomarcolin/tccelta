# ELM327 task

`Elm327Task` is the concurrency boundary. BLE callbacks only copy incoming
data into a FreeRTOS queue; the task calls `Elm327::process()` synchronously.
That preserves the strict rule that a new request cannot start while the
previous response is still being collected, formatted, or sent.

```mermaid
stateDiagram-v2
    [*] --> Idle
    Idle --> Queued: authenticated command
    Queued --> Processing: task receives command
    Processing --> WaitingForCan: OBD request
    WaitingForCan --> Sending: response complete or timeout
    Processing --> Sending: local AT command
    Sending --> Idle: response ends with >
```
