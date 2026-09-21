# Real OBD-II over CAN

`Obd2Can` maps the ELM327 OBD operations to functional request ID `0x7DF` and
response IDs `0x7E8`–`0x7EF`. `IsoTpClient` owns ISO-TP framing and collects
responses independently by ECU ID.

```mermaid
sequenceDiagram
    participant OBD as Obd2Can
    participant ISO as IsoTpClient
    participant CAN as ICanBus
    participant E1 as ECU 7E8
    participant E2 as ECU 7E9
    OBD->>ISO: requestAll(service, pid, timeout, expected)
    ISO->>CAN: functional request 7DF
    CAN->>E1: request
    CAN->>E2: request
    E1-->>CAN: SF or FF/CF
    E2-->>CAN: SF or FF/CF
    ISO->>CAN: FC to each ECU that sends FF
    ISO->>ISO: route CF by ECU ID and sequence
    ISO-->>OBD: ResponseSet{ecu ID, OBD payload}
```

The collector resets the quiet-response timer after valid activity. It stops
early when the optional response count is complete; otherwise it returns after
the quiet timeout. ISO-TP `N_CR` remains independent, so an incomplete
multi-frame ECU response cannot extend the global wait indefinitely.

The fixed-size response set avoids heap allocation on the ESP32 and supports
up to eight response ECUs with up to 256 reassembled payload bytes each.
`request()` remains as a single-response compatibility wrapper over
`requestAll()`.
