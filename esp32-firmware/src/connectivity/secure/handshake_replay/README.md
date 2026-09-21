# Handshake with intra-session replay protection

This mode uses the same nonce exchange and session-key derivation as
`handshake`, then adds an independent strictly increasing counter per
direction. The counter and direction are authenticated as GCM AAD.

```mermaid
sequenceDiagram
    participant A as App
    participant D as Dongle
    A->>D: HELLO / CHALLENGE / PROOF / OK
    Note over A,D: Fresh session key established
    A->>D: counter || IV || ciphertext || tag
    D->>D: require counter > last app counter
    D->>D: GCM AAD = counter || app-to-dongle
    D-->>A: counter || IV || ciphertext || tag
    A->>A: require counter > last dongle counter
```

Replayed or older frames are discarded. Counters reset with a new handshake.
