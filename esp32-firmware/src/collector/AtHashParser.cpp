#include "collector/AtHashParser.h"
#include <cstring>

namespace collector {

namespace {

using Kind = AtHashCommand::Kind;

/** Leitor de argumentos separados por espaço. */
class Args {
public:
    explicit Args(const char* text) : _p(text) {}

    bool hexByte(uint8_t& out) {
        skipSpaces();
        uint8_t value = 0;
        for (int i = 0; i < 2; ++i) {
            const int nibble = hexNibble(_p[i]);
            if (nibble < 0) return false;
            value = static_cast<uint8_t>((value << 4) | nibble);
        }
        _p += 2;
        out = value;
        return endOfToken();
    }

    bool decimal(int64_t& out, int64_t min, int64_t max) {
        skipSpaces();
        bool negative = false;
        if (*_p == '-') { negative = true; ++_p; }
        if (*_p < '0' || *_p > '9') return false;
        int64_t value = 0;
        while (*_p >= '0' && *_p <= '9') {
            value = value * 10 + (*_p++ - '0');
            if (value > kMaxMagnitude) return false;  // evita estouro; o limite real vem de min/max
        }
        if (negative) value = -value;
        out = value;
        return endOfToken() && value >= min && value <= max;
    }

    bool flag(bool& out) {
        int64_t value;
        if (!decimal(value, 0, 1)) return false;
        out = value == 1;
        return true;
    }

    bool atEnd() {
        skipSpaces();
        return *_p == '\0';
    }

private:
    static constexpr int64_t kMaxMagnitude = 99999999999LL;  // 11 dígitos

    const char* _p;

    void skipSpaces() { while (*_p == ' ') ++_p; }
    bool endOfToken() const { return *_p == ' ' || *_p == '\0'; }

    static int hexNibble(char c) {
        if (c >= '0' && c <= '9') return c - '0';
        if (c >= 'A' && c <= 'F') return c - 'A' + 10;
        return -1;
    }
};

/** `text` começa com `name` seguido de fim ou espaço? Avança `rest` para depois do nome. */
bool matches(const char* text, const char* name, const char*& rest) {
    const size_t n = strlen(name);
    if (strncmp(text, name, n) != 0) return false;
    if (text[n] != '\0' && text[n] != ' ') return false;
    rest = text + n;
    return true;
}

AtHashCommand parseArgs(Kind kind, const char* rest) {
    AtHashCommand cmd;
    Args args(rest);
    int64_t a, b, c;
    bool ok = false;

    switch (kind) {
        case Kind::SetPid:
            ok = args.hexByte(cmd.pid) && args.decimal(a, 1, UINT32_MAX);
            cmd.periodMs = static_cast<uint32_t>(a);
            break;
        case Kind::DeletePid:
            ok = args.hexByte(cmd.pid);
            break;
        case Kind::Flush:
            ok = args.decimal(a, 1, UINT32_MAX);
            cmd.periodMs = static_cast<uint32_t>(a);
            break;
        case Kind::Ack:
            ok = args.decimal(a, 1, UINT32_MAX);
            cmd.seq = static_cast<uint32_t>(a);
            break;
        case Kind::Diag:
        case Kind::Net:
            ok = args.flag(cmd.flag);
            break;
        case Kind::Gps:
            ok = args.decimal(a, -900000000, 900000000) &&
                 args.decimal(b, -1800000000, 1800000000) &&
                 args.decimal(c, INT32_MIN, INT32_MAX);
            cmd.latE7 = static_cast<int32_t>(a);
            cmd.lonE7 = static_cast<int32_t>(b);
            cmd.altMm = static_cast<int32_t>(c);
            break;
        default:
            ok = true;  // comandos sem argumento
            break;
    }

    if (!ok || !args.atEnd()) return AtHashCommand{};
    cmd.kind = kind;
    return cmd;
}

}  // namespace

AtHashCommand parseAtHash(const char* text) {
    static const struct { const char* name; Kind kind; } kCommands[] = {
        {"AT#PID", Kind::SetPid},     {"AT#DEL", Kind::DeletePid},
        {"AT#LIST", Kind::List},      {"AT#DIAG", Kind::Diag},
        {"AT#NET", Kind::Net},        {"AT#GPS", Kind::Gps},
        {"AT#TMAX?", Kind::TmaxQuery}, {"AT#TMAX", Kind::Tmax},
        {"AT#ACK", Kind::Ack},        {"AT#FLUSH", Kind::Flush},
    };

    for (const auto& command : kCommands) {
        const char* rest = nullptr;
        if (matches(text, command.name, rest)) return parseArgs(command.kind, rest);
    }
    return AtHashCommand{};
}

}  // namespace collector
