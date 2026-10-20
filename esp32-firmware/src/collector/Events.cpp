#include "collector/Events.h"
#include <cstdio>
#include <cstring>

namespace collector {

namespace {

IEventSink* gSink = nullptr;

const char kHexDigits[] = "0123456789ABCDEF";

}  // namespace

void setEventSink(IEventSink* sink) { gSink = sink; }

Event::Event(const char* name) : Event(name, _small, sizeof(_small)) {}

Event::Event(const char* name, char* buffer, size_t capacity) : _buf(buffer), _cap(capacity) {
    if (!kEventsEnabled) return;
    append(name, strlen(name));
}

Event::~Event() {
    if (!kEventsEnabled || !gSink || _overflow) return;
    gSink->emit(_buf, _len, _hasTime ? &_tMs : nullptr);
}

void Event::append(const char* s, size_t n) {
    if (!kEventsEnabled) return;
    if (_len + n > _cap) {
        _overflow = true;  // linha truncada não é enviada: falta é melhor que dado errado
        return;
    }
    memcpy(_buf + _len, s, n);
    _len += n;
}

void Event::key(const char* k) {
    append(' ');
    append(k, strlen(k));
    append('=');
}

Event& Event::num(const char* k, int64_t value) {
    if (!kEventsEnabled) return *this;
    char digits[24];
    const int n = snprintf(digits, sizeof(digits), "%lld", static_cast<long long>(value));
    key(k);
    append(digits, static_cast<size_t>(n));
    return *this;
}

Event& Event::flag(const char* k, bool value) {
    if (!kEventsEnabled) return *this;
    key(k);
    append(value ? '1' : '0');
    return *this;
}

Event& Event::hex8(const char* k, uint8_t value) { return hex(k, &value, 1); }

Event& Event::hex(const char* k, const uint8_t* bytes, size_t len) {
    if (!kEventsEnabled) return *this;
    key(k);
    for (size_t i = 0; i < len; ++i) {
        append(kHexDigits[bytes[i] >> 4]);
        append(kHexDigits[bytes[i] & 0x0F]);
    }
    return *this;
}

Event& Event::hexList(const char* k, const uint8_t* values, size_t count) {
    if (!kEventsEnabled) return *this;
    key(k);
    if (count == 0) append('-');
    for (size_t i = 0; i < count; ++i) {
        if (i > 0) append(',');
        append(kHexDigits[values[i] >> 4]);
        append(kHexDigits[values[i] & 0x0F]);
    }
    return *this;
}

Event& Event::token(const char* k, const char* value) {
    if (!kEventsEnabled) return *this;
    key(k);
    append(value, strlen(value));
    return *this;
}

Event& Event::text(const char* k, const char* s, size_t len) {
    if (!kEventsEnabled) return *this;
    key(k);
    append('"');
    for (size_t i = 0; i < len; ++i) {
        switch (s[i]) {
            case '"':  append("\\\"", 2); break;
            case '\\': append("\\\\", 2); break;
            case '\r': append("\\r", 2); break;
            case '\n': append("\\n", 2); break;
            default:   append(s[i]); break;
        }
    }
    append('"');
    return *this;
}

Event& Event::absent(const char* k) {
    if (!kEventsEnabled) return *this;
    key(k);
    append('-');
    return *this;
}

}  // namespace collector
