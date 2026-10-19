#include "Elm327.h"
#include "collector/AtHashParser.h"
#include "collector/Utilization.h"

static constexpr char CR = '\r';
static constexpr char LF = '\n';

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

Elm327::Elm327(IObd2* obd2, collector::ICollectorControl* control)
    : _obd2(obd2), _control(control), _echo(true), _linefeed(false), _headers(false), _spaces(true),
      _timeoutMs(200) {}

String Elm327::prompt() const {
    return _linefeed ? "\r\n>" : "\r>";
}

String Elm327::responseSeparator() const {
    return _linefeed ? "\r\n" : "\r";
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

    if (upper.startsWith("AT#") && _control) {
        response += processAtHash(upper);
    } else if (upper.startsWith("AT")) {
        String compactAt = upper;
        compactAt.replace(" ", "");
        response += processAt(compactAt);
    } else if (collectorMode()) {
        // Fora do diagnóstico, só os PIDs coletados ("01PP"), lidos da CVT.
        String compact = upper;
        compact.replace(" ", "");
        uint8_t service, pid;
        if (compact.length() == 4 && parseHexByte(compact, 0, service) &&
            parseHexByte(compact, 2, pid)) {
            response += processCollectorPid(service, pid);
        } else {
            response += "?" + prompt();
        }
    } else {
        // Strip spaces from OBD command ("01 0C" → "010C")
        String compact = upper;
        compact.replace(" ", "");

        if (compact.length() == 2 || compact.length() == 3) {
            // Mode 03/07/0A: DTC lists, no PID ("03" alone).
            uint8_t service;
            if (parseHexByte(compact, 0, service) &&
                (service == 0x03 || service == 0x07 || service == 0x0A)) {
                uint8_t expected = 0;
                if (compact.length() == 3) {
                    expected = hexNibble(compact[2]);
                    if (expected == 0 || expected > OBD2_MAX_ECU_RESPONSES) {
                        response += "?" + prompt();
                        return response;
                    }
                }
                response += processDtc(service, expected);
            } else {
                response += "?" + prompt();
            }
        } else if (compact.length() != 4 && compact.length() != 5) {
            // Only exactly one service/PID pair is accepted. This prevents a
            // longer malformed command from being partially parsed and sent
            // to the OBD2 backend.
            response += "?" + prompt();
        } else {
            uint8_t service, pid;
            if (!parseHexByte(compact, 0, service) ||
                !parseHexByte(compact, 2, pid)) {
                response += "?" + prompt();
            } else {
                size_t expected = 0;
                if (compact.length() == 5) {
                    uint8_t count = hexNibble(compact[4]);
                    if (count == 0 || count > OBD2_MAX_ECU_RESPONSES) {
                        response += "?" + prompt();
                        return response;
                    }
                    expected = count;
                }
                if (service == 0x02) {
                // Mode 02 (freeze frame): resposta tem um layout diferente
                // (SID+PID+frame# antes do dado, contra SID+PID do Modo
                // 01/09) — não reaproveita processObd/readPid.
                    response += processFreezeFrame(pid, expected);
                } else {
                    response += processObd(service, pid, expected);
                }
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
            _timeoutMs = 200;
            if (!collectorMode()) _obd2->setResponseTimeoutMs(_timeoutMs);
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
        _timeoutMs = 200;
        if (!collectorMode()) _obd2->setResponseTimeoutMs(_timeoutMs);
        return "OK" + prompt();
    }

    // ATDP — describe protocol
    const collector::CanProtocol protocol =
        _control ? _control->config().protocol : collector::CanProtocol::Id11Kbps500;
    if (sub == "DP")  return collector::describe(protocol) + prompt();
    if (sub == "DPN") return String(static_cast<int>(protocol)) + prompt();

    // ATSPh — protocolo (6 a 9). Grava e reinicia o Dongle: não há resposta.
    if (_control && sub.length() == 3 && sub.startsWith("SP")) {
        const uint8_t number = hexNibble(sub[2]);
        if (!collector::isValidProtocol(number)) return "?" + prompt();
        _control->setProtocol(number);
        return "?" + prompt();  // só chega aqui se o reinício falhar
    }

    if (sub.length() == 4 && sub.startsWith("ST")) {
        uint8_t value;
        if (!parseHexByte(sub, 2, value)) return "?" + prompt();
        return processSetTimeout(value);
    }

    // Unknown AT command
    return "?" + prompt();
}

// ATSThh: prazo de resposta em múltiplos de 4 ms. Com coleta, é o Tmax.
String Elm327::processSetTimeout(uint8_t value) {
    const uint32_t timeoutMs = static_cast<uint32_t>(value == 0 ? 1 : value) * 4u;
    if (_control && _control->setTmax(timeoutMs) != collector::ConfigError::None) {
        return "?" + prompt();
    }
    _timeoutMs = timeoutMs;
    if (!collectorMode()) _obd2->setResponseTimeoutMs(_timeoutMs);
    return "OK" + prompt();
}

// ── AT# (coleta) ──────────────────────────────────────────────────────────

String Elm327::processAtHash(const String& upper) {
    using Kind = collector::AtHashCommand::Kind;
    const collector::AtHashCommand cmd = collector::parseAtHash(upper.c_str());
    const String ok = "OK" + prompt();
    const String fail = "?" + prompt();

    switch (cmd.kind) {
        case Kind::SetPid:
            return _control->setPid(cmd.pid, cmd.periodMs) == collector::ConfigError::None ? ok : fail;
        case Kind::DeletePid:
            return _control->deletePid(cmd.pid) ? ok : fail;
        case Kind::List:
            return formatList() + prompt();
        case Kind::Diag:
            return _control->setDiagMode(cmd.flag) ? ok : fail;
        case Kind::Net:
            _control->setInternet(cmd.flag);
            return ok;
        case Kind::Gps:
            _control->reportGps({cmd.latE7, cmd.lonE7, cmd.altMm});
            return ok;
        case Kind::Tmax:
            return _control->startTmaxMeasurement() ? ok : fail;
        case Kind::TmaxQuery:
            return formatTmaxReport(_control->tmaxReport()) + prompt();
        case Kind::Invalid:
            break;
    }
    return fail;
}

// Uma linha por PID ("0C 250 125": PID, Pᵢ, Pᵢᵐⁱⁿ), depois Tmax, U e contadores.
String Elm327::formatList() const {
    const collector::CollectorConfig config = _control->config();
    const collector::CollectorStats stats = _control->stats();
    const String sep = responseSeparator();
    char line[48];
    String out;
    for (size_t i = 0; i < config.pidCount; ++i) {
        const collector::PidSetting& p = config.pids[i];
        snprintf(line, sizeof(line), "%02X %lu %lu", p.pid, static_cast<unsigned long>(p.periodMs),
                 static_cast<unsigned long>(p.minIntervalMs));
        out += line + sep;
    }
    snprintf(line, sizeof(line), "TMAX %lu U %.2f", static_cast<unsigned long>(config.tmaxMs),
             collector::utilization(config));
    out += line + sep;
    snprintf(line, sizeof(line), "DROP %lu GPS %lu FAIL %lu LOST %lu",
             static_cast<unsigned long>(stats.droppedRecords),
             static_cast<unsigned long>(stats.droppedGps),
             static_cast<unsigned long>(stats.failedBlocks),
             static_cast<unsigned long>(stats.lostBlocks));
    return out + line;
}

// "IDLE", "RUNNING k/n", ou uma linha por PID ("0C P99 32 TO 0") + "TMAX 44".
String Elm327::formatTmaxReport(const collector::TmaxReport& report) const {
    using State = collector::TmaxReport::State;
    char line[48];
    if (report.state == State::Idle) return "IDLE";
    if (report.state == State::Running) {
        snprintf(line, sizeof(line), "RUNNING %u/%u", static_cast<unsigned>(report.done),
                 static_cast<unsigned>(report.total));
        return line;
    }
    const String sep = responseSeparator();
    String out;
    for (size_t i = 0; i < report.pidCount; ++i) {
        const auto& p = report.pids[i];
        snprintf(line, sizeof(line), "%02X P99 %lu TO %lu", p.pid,
                 static_cast<unsigned long>(p.p99Ms), static_cast<unsigned long>(p.timeouts));
        out += line + sep;
    }
    snprintf(line, sizeof(line), "TMAX %lu", static_cast<unsigned long>(report.suggestedTmaxMs));
    return out + line;
}

// Modo coletor: responde do último valor lido, no formato do Modo 01.
String Elm327::processCollectorPid(uint8_t service, uint8_t pid) {
    collector::CurrentValues::Value value;
    if (service != 0x01 || !_control->config().find(pid)) return "?" + prompt();
    if (!_control->currentValue(pid, value)) return "NO DATA" + prompt();
    return formatDataBytes(service, pid, value.bytes, value.len, 0x7E8) + prompt();
}

// ── OBD command handler ───────────────────────────────────────────────────

String Elm327::processObd(uint8_t service, uint8_t pid, size_t expectedResponses) {
    if (!_whitelist.isAllowed(service, pid)) return "NO DATA" + prompt();

    Obd2ResponseSet responses;
    int count = _obd2->readPidAll(service, pid, responses, _timeoutMs, expectedResponses);
    if (count <= 0) return "NO DATA" + prompt();

    String result;
    for (size_t i = 0; i < responses.count; ++i) {
        if (i != 0) result += responseSeparator();
        result += formatDataBytes(service, pid, responses.items[i].data,
                                  responses.items[i].len, responses.items[i].ecuId);
    }
    return result + prompt();
}

String Elm327::formatDataBytes(uint8_t service, uint8_t pid,
                               const uint8_t* data, int len, uint32_t ecuId) {
    char hex[3];
    String sep = _spaces ? " " : "";
    String out;

    if (_headers) {
        char id[4];
        snprintf(id, sizeof(id), "%03lX", static_cast<unsigned long>(ecuId));
        out += id;
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

// ── DTC command handler (Mode 03/07/0A) ──────────────────────────────────

String Elm327::processDtc(uint8_t service, size_t expectedResponses) {
    if (!_whitelist.isServiceAllowed(service)) return "NO DATA" + prompt();

    Obd2ResponseSet responses;
    int count = _obd2->readDtcAll(service, responses, _timeoutMs, expectedResponses);
    if (count <= 0) return "NO DATA" + prompt();

    String result;
    for (size_t i = 0; i < responses.count; ++i) {
        if (i != 0) result += responseSeparator();
        result += formatDtcResponse(service, responses.items[i].data,
                                    responses.items[i].len, responses.items[i].ecuId);
    }
    return result + prompt();
}

String Elm327::formatDtcResponse(uint8_t service, const uint8_t* data,
                                  size_t len, uint32_t ecuId) {
    if (len < 2 || data[0] != static_cast<uint8_t>(service | 0x40u)) return "NO DATA";
    size_t count = data[1];
    if (2 + count * 2 > len) count = (len - 2) / 2;

    uint16_t codes[OBD2_MAX_RESPONSE_BYTES / 2] = {};
    for (size_t i = 0; i < count; ++i) {
        codes[i] = static_cast<uint16_t>((data[2 + i * 2] << 8) |
                                         data[3 + i * 2]);
    }
    String result = formatDtcBytes(service, codes, static_cast<int>(count));
    if (_headers) {
        char id[4];
        snprintf(id, sizeof(id), "%03lX", static_cast<unsigned long>(ecuId));
        result = String(id) + (_spaces ? " " : "") + result;
    }
    return result;
}

String Elm327::formatDtcBytes(uint8_t service, const uint16_t* codes, int count) {
    char hex[3];
    String sep = _spaces ? " " : "";
    String out;

    // Response service byte (SID + 0x40) e contagem de DTCs.
    snprintf(hex, sizeof(hex), "%02X", (uint8_t)(service | 0x40u));
    out += hex;
    out += sep;
    snprintf(hex, sizeof(hex), "%02X", (uint8_t)count);
    out += hex;

    for (int i = 0; i < count; i++) {
        out += sep;
        snprintf(hex, sizeof(hex), "%02X", (uint8_t)(codes[i] >> 8));
        out += hex;
        out += sep;
        snprintf(hex, sizeof(hex), "%02X", (uint8_t)(codes[i] & 0xFF));
        out += hex;
    }

    return out;
}

// ── Freeze frame command handler (Mode 02, frame 0 only) ─────────────────

String Elm327::processFreezeFrame(uint8_t pid, size_t expectedResponses) {
    if (!_whitelist.isAllowed(0x02, pid)) return "NO DATA" + prompt();

    Obd2ResponseSet responses;
    int count = _obd2->readFreezeFramePidAll(pid, responses, _timeoutMs,
                                             expectedResponses);
    if (count <= 0) return "NO DATA" + prompt();

    String result;
    for (size_t i = 0; i < responses.count; ++i) {
        if (i != 0) result += responseSeparator();
        result += formatFreezeFrameBytes(pid, responses.items[i].data,
                                         responses.items[i].len,
                                         responses.items[i].ecuId);
    }
    return result + prompt();
}

String Elm327::formatFreezeFrameBytes(uint8_t pid, const uint8_t* data, int len,
                                      uint32_t ecuId) {
    char hex[3];
    String sep = _spaces ? " " : "";
    String out;

    if (_headers) {
        char id[4];
        snprintf(id, sizeof(id), "%03lX", static_cast<unsigned long>(ecuId));
        out += id;
        out += sep;
        snprintf(hex, sizeof(hex), "%02X", (uint8_t)(3 + len));
        out += hex;
        out += sep;
    }

    // SID de resposta do Modo 02 (0x42) + PID + frame# (sempre 0 — único
    // frame suportado, não há histórico de frames antigos).
    snprintf(hex, sizeof(hex), "%02X", 0x42);
    out += hex;
    out += sep;
    snprintf(hex, sizeof(hex), "%02X", pid);
    out += hex;
    out += sep;
    snprintf(hex, sizeof(hex), "%02X", 0x00);
    out += hex;

    for (int i = 0; i < len; i++) {
        out += sep;
        snprintf(hex, sizeof(hex), "%02X", data[i]);
        out += hex;
    }

    return out;
}
