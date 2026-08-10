# ESP32 OBD2 Dongle Firmware

BLE-based OBD2 dongle firmware for ESP32, built with PlatformIO + FreeRTOS.

---

## Architecture

```
IConnectivity (interface)
├── BleConnectivity                     plain BLE transport (NUS profile)
└── SecureBleConnectivity               abstract base (FreeRTOS queue + task)
    ├── SecurePskBleConnectivity        AES-256-GCM, static PSK  ← USE_SECURE_PSK
    └── SecureHandshakeBleConnectivity  handshake stub            ← USE_SECURE_HANDSHAKE

IObd2 (interface)
├── Obd2Can → ICanBus
│   ├── Mcp2515Can    (SPI)  ← USE_MCP2515
│   └── TwaiCan       (native TWAI)  ← USE_TWAI
└── Obd2Mock                          ← USE_MOCK
```

Inbound data flow:
```
BleConnectivity::onWrite()
    └─► SecureBleConnectivity._rawQueue   (ISR-safe enqueue)
            └─► [SecureTask]
                    └─► onRawFrameReceived()   ← subclass decrypts
                            └─► deliverPlaintext() → _userCallback
                                                       └─► [Elm327Task]
```

---

## Build Environments

Run with: `pio run -e <env>` / `pio run -e <env> -t upload`

| Environment | CAN backend | Security |
|-------------|------------|----------|
| `mock` | Simulated | None |
| `twai` | ESP32 TWAI | None |
| `mcp2515` | MCP2515 SPI | None |
| `mock_psk` | Simulated | AES-256-GCM PSK |
| `twai_psk` | ESP32 TWAI | AES-256-GCM PSK |
| `mcp2515_psk` | MCP2515 SPI | AES-256-GCM PSK |
| `mock_handshake` | Simulated | Handshake stub |
| `twai_handshake` | ESP32 TWAI | Handshake stub |

### Quick start (no hardware)

```bash
# Plain transport
pio run -e mock

# Encrypted (uses placeholder key from platformio.ini — safe for local dev/CI)
pio run -e mock_psk

# Flash to device
pio run -e twai_psk -t upload

# Open serial monitor
pio device monitor
```

---

## Configuration

### PSK Key (`USE_SECURE_PSK` environments)

The PSK environments in `platformio.ini` ship with a **placeholder key** that
is NOT safe for production. You **must** replace it before flashing to a real
device.

**Step 1 — Generate a key**

```bash
# Unix / macOS / Git Bash
openssl rand -hex 32
# Output example: a3f1...  (64 hex chars)
```

**Step 2 — Create your secrets file** (already in `.gitignore`)

```bash
cp platformio_secrets.ini.example platformio_secrets.ini
```

Edit `platformio_secrets.ini` and uncomment / fill in the environments you use:

```ini
[env:mock_psk]
build_flags =
    ${env:_secure_psk_base.build_flags}
    -DUSE_MOCK
    -DSECURE_PSK_HEX=a3f1<...your 64 hex chars...>
```

**Step 3 — Build**

```bash
pio run -e mock_psk
```

PlatformIO merges `platformio_secrets.ini` automatically (via `extra_configs`).

### CI / CD

Export the flag as an environment variable — PlatformIO picks it up:

```bash
export PLATFORMIO_BUILD_FLAGS="-DUSE_MOCK -DSECURE_PSK_HEX=$(openssl rand -hex 32)"
pio run -e mock_psk
```

Or inject it as a GitHub Actions secret:

```yaml
- run: pio run -e mock_psk
  env:
    PLATFORMIO_BUILD_FLAGS: "-DUSE_MOCK -DSECURE_PSK_HEX=${{ secrets.OBD2_PSK_HEX }}"
```

---

## PSK Wire Protocol (`SecurePskBleConnectivity`)

All frames are ASCII hex, newline-terminated, sent over the BLE NUS RX/TX
characteristics.

```
<24 hex — IV (12 bytes)><variable hex — ciphertext><32 hex — GCM tag (16 bytes)>\n
```

- **Algorithm**: AES-256-GCM (mbedTLS, bundled with ESP-IDF)
- **IV**: 12 bytes, fresh CSPRNG (`esp_fill_random`) per message
- **AAD**: none (may be added in a future version)
- **No replay protection** in this version — add a counter field at the app layer if needed

### Encoding example (Python)

```python
from Crypto.Cipher import AES
import os, binascii

key = bytes.fromhex("deadbeef...")   # 64 hex chars → 32 bytes
iv  = os.urandom(12)
cipher = AES.new(key, AES.MODE_GCM, nonce=iv)
ct, tag = cipher.encrypt_and_digest(b"01 00 0C\r")   # OBD2 command

frame = (iv + ct + tag).hex() + "\n"
# Send `frame` via BLE WRITE to the RX characteristic
```

---

## Hardware Notes

| GPIO | Role |
|------|------|
| 21   | TWAI TX |
| 22   | TWAI RX |
| 5    | MCP2515 CS |

Transceiver: SN65HVD230 (TWAI) or MCP2551 (MCP2515).
