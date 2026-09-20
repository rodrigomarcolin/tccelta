# Secure BLE layer

The base decorator owns the FreeRTOS queue and delivers a frame to one of the
security implementations. Only authenticated plaintext reaches `Elm327Task`.

```mermaid
sequenceDiagram
    participant BLE as BLE/NUS
    participant Q as Secure queue/task
    participant S as Security implementation
    participant E as Elm327Task
    BLE->>Q: raw frame
    Q->>S: inspect/decrypt
    S->>E: authenticated plaintext
```

The plain transport remains available for development builds. Production builds
should select either `USE_SECURE_PSK` or `USE_SECURE_HANDSHAKE_REPLAY`.
