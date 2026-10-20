#pragma once
#include <cstddef>
#include <cstdint>

/**
 * Eventos de depuração da coleta (docs/debug-events.md). Só existem com
 * `COLLECTOR_EVENTS`; sem a flag, `Event` não faz nada e o compilador remove
 * as chamadas.
 *
 * Uso: `Event("READ").hex("pid", pid).flag("ok", ok).num("dur", dur);` — a
 * linha é entregue ao `IEventSink` quando o objeto sai de escopo. O buffer
 * interno (256 B) cabe na pilha de qualquer tarefa; eventos grandes (FLUSH
 * com o bloco em hexa) passam um buffer próprio.
 */
namespace collector {

#if defined(COLLECTOR_EVENTS)
constexpr bool kEventsEnabled = true;
#else
constexpr bool kEventsEnabled = false;
#endif

/** Destino das linhas. Acrescenta "EV <n> <t> " e as envia (serial no ESP32). */
class IEventSink {
public:
    virtual ~IEventSink() = default;
    /**
     * `body` = "EVENTO campo=valor ..." sem prefixo nem quebra de linha.
     * `tMs` = instante do evento, ou nullptr para "agora".
     */
    virtual void emit(const char* body, size_t len, const uint32_t* tMs) = 0;
};

/** Define o destino global (nullptr = descartar). Chamado uma vez no boot. */
void setEventSink(IEventSink* sink);

class Event {
public:
    explicit Event(const char* name);
    Event(const char* name, char* buffer, size_t capacity);
    ~Event();
    Event(const Event&) = delete;
    Event& operator=(const Event&) = delete;

    Event& num(const char* key, int64_t value);
    Event& flag(const char* key, bool value);
    Event& hex8(const char* key, uint8_t value);
    Event& hex(const char* key, const uint8_t* bytes, size_t len);
    Event& hexList(const char* key, const uint8_t* values, size_t count);
    Event& token(const char* key, const char* value);       // enum ou token sem espaços
    Event& text(const char* key, const char* s, size_t len);  // entre aspas, com escapes
    Event& absent(const char* key);

    /** Instante do evento, quando não é "agora" (ex.: fim de uma leitura). */
    Event& at(uint32_t tMs) { _tMs = tMs; _hasTime = true; return *this; }

    static constexpr size_t kSmallCapacity = 256;
    static constexpr size_t kLargeCapacity = 2400;  // cabe o FLUSH com 1 KB em hexa

private:
    char   _small[kEventsEnabled ? kSmallCapacity : 1];
    char*  _buf;
    size_t _cap;
    size_t _len = 0;
    bool   _overflow = false;
    bool   _hasTime = false;
    uint32_t _tMs = 0;

    void append(const char* s, size_t n);
    void append(char c) { append(&c, 1); }
    void key(const char* k);
};

}  // namespace collector
