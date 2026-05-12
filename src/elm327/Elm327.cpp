#include "Elm327.h"

static constexpr char CR = '\r';
static constexpr char LF = '\n';

// TODO: Revisar/Melhorar. Este arquivo foi gerado com IA.
// ── helpers ───────────────────────────────────────────────────────────────

static uint8_t hexNibble(char c) {
    if (c >= '0' && c <= '9') return c - '0';
    if (c >= 'A' && c <= 'F') return c - 'A' + 10;
    if (c >= 'a' && c <= 'f') return c - 'a' + 10;
    return 0xFF;  // invalid
}

static bool parseHexByte(const String& s, int pos, uint8_t& out) {
    if (pos + 1 >= (int)s.length()) return false;
    uint8_t hi = hexNibble(s[pos]);
    uint8_t lo = hexNibble(s[pos + 1]);
    if (hi == 0xFF || lo == 0xFF) return false;
    out = (hi << 4) | lo;
    return true;
}

// ── Elm327 ────────────────────────────────────────────────────────────────

Elm327::Elm327(IObd2* obd2)
    : _obd2(obd2), _echo(true), _linefeed(false), _headers(false), _spaces(true) {}

String Elm327::prompt() const {
    return _linefeed ? "\r\n>" : "\r>";
}

String Elm327::process(const String& cmd) {
    // Strip CR/LF
    String trimmed = cmd;
    trimmed.trim();
    trimmed.replace("\r", "");
    trimmed.replace("\n", "");

    if (trimmed.length() == 0) return prompt();

    // Echo (before processing)
    String response;
    if (_echo) response = trimmed + CR;

    // Upper-case copy for AT detection
    String upper = trimmed;
    upper.toUpperCase();

    if (upper.startsWith("AT")) {
        response += processAt(upper);
    } else {
        // Strip spaces from OBD command ("01 0C" → "010C")
        String compact = upper;
        compact.replace(" ", "");

        if (compact.length() < 4) {
            response += "?" + prompt();
        } else {
            uint8_t service, pid;
            if (!parseHexByte(compact, 0, service) ||
                !parseHexByte(compact, 2, pid)) {
                response += "?" + prompt();
            } else {
                response += processObd(service, pid);
            }
        }
    }

    return response;
}

// ── AT command handler ────────────────────────────────────────────────────

String Elm327::processAt(const String& upper) {
    // upper already starts with "AT"
    String sub = upper.substring(2);  // everything after "AT"

    // ATZ / ATI — identify / reset
    if (sub == "Z" || sub == "I" || sub == "@1") {
        // Reset state on ATZ
        if (sub == "Z") {
            _echo     = true;
            _linefeed = false;
            _headers  = false;
            _spaces   = true;
        }
        return "\r\rELM327 v1.5\r\r" + prompt();
    }

    // ATE0 / ATE1 — echo
    if (sub == "E0") { _echo = false; return "OK" + prompt(); }
    if (sub == "E1") { _echo = true;  return "OK" + prompt(); }

    // ATH0 / ATH1 — headers
    if (sub == "H0") { _headers = false; return "OK" + prompt(); }
    if (sub == "H1") { _headers = true;  return "OK" + prompt(); }

    // ATL0 / ATL1 — linefeeds
    if (sub == "L0") { _linefeed = false; return "OK" + prompt(); }
    if (sub == "L1") { _linefeed = true;  return "OK" + prompt(); }

    // ATS0 / ATS1 — spaces
    if (sub == "S0") { _spaces = false; return "OK" + prompt(); }
    if (sub == "S1") { _spaces = true;  return "OK" + prompt(); }

    // ATD — set defaults
    if (sub == "D") {
        _echo = true; _linefeed = false; _headers = false; _spaces = true;
        return "OK" + prompt();
    }

    // ATSP[0-C] — set protocol (ignored; we always use ISO 15765-4)
    if (sub.startsWith("SP")) return "OK" + prompt();

    // ATDP — describe protocol
    if (sub == "DP")  return "ISO 15765-4 (CAN 11/500)" + prompt();
    if (sub == "DPN") return "6" + prompt();  // protocol number

    // ATPC — protocol close
    if (sub == "PC") return "OK" + prompt();

    // ATRV — read voltage (stub)
    if (sub == "RV") return "12.0V" + prompt();

    // ATAT[0-2] — adaptive timing
    if (sub == "AT0" || sub == "AT1" || sub == "AT2") return "OK" + prompt();

    // ATST[hh] — set timeout (accepted, ignored)
    if (sub.startsWith("ST")) return "OK" + prompt();

    // ATAR — auto receive address
    if (sub == "AR") return "OK" + prompt();

    // ATAL — allow long messages
    if (sub == "AL") return "OK" + prompt();

    // ATM0 / ATM1 — memory
    if (sub == "M0" || sub == "M1") return "OK" + prompt();

    // ATCAF0 / ATCAF1 — CAN auto-formatting
    if (sub == "CAF0" || sub == "CAF1") return "OK" + prompt();

    // Unknown AT command
    return "?" + prompt();
}

// ── OBD command handler ───────────────────────────────────────────────────

String Elm327::processObd(uint8_t service, uint8_t pid) {
    uint8_t buf[7] = {};
    int len = _obd2->readPid(service, pid, buf, sizeof(buf));

    if (len <= 0) return "NO DATA" + prompt();

    return formatDataBytes(service, pid, buf, len) + prompt();
}

String Elm327::formatDataBytes(uint8_t service, uint8_t pid,
                               const uint8_t* data, int len) {
    char hex[3];
    String sep = _spaces ? " " : "";
    String out;

    if (_headers) {
        // Minimal single-frame header: "7E8 06 SS+40 PP D0 D1 …"
        out += "7E8";
        out += sep;
        snprintf(hex, sizeof(hex), "%02X", (uint8_t)(2 + len));
        out += hex;
        out += sep;
    }

    // Response service byte
    snprintf(hex, sizeof(hex), "%02X", (uint8_t)(service | 0x40u));
    out += hex;
    out += sep;

    // PID byte
    snprintf(hex, sizeof(hex), "%02X", pid);
    out += hex;

    // Data bytes
    for (int i = 0; i < len; i++) {
        out += sep;
        snprintf(hex, sizeof(hex), "%02X", data[i]);
        out += hex;
    }

    return out;
}
