# Handshake session security

This mode authenticates both sides with the PSK and derives a fresh AES-256
session key from fresh nonces. It deliberately has no message counter.

```mermaid
sequenceDiagram
    participant A as App
    participant D as Dongle
    A->>D: HELLO(appNonce)
    D->>A: CHALLENGE(dongleNonce)
    A->>D: PROOF(HMAC(PSK, appNonce || dongleNonce))
    D->>A: OK(HMAC(sessionKey, "confirm"))
    Note over A,D: HKDF(PSK, appNonce || dongleNonce, "session-key")
    A->>D: AES-GCM(sessionKey)
    D-->>A: AES-GCM(sessionKey)
```

A new `HELLO` discards the previous session and creates a new key.
