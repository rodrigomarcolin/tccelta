#pragma once
#include <cstddef>
#include <cstdint>

namespace collector {

constexpr size_t   kUploadWindow         = 8;     // blocos sem confirmação
constexpr uint32_t kUploadAckTimeoutMs   = 5000;  // sem ACK → retransmite

/**
 * O que enviar ao servidor e quando retransmitir (plano §5.8).
 *
 * `ackSeq` é o maior `seq` que o servidor confirmou ter recebido sem lacunas.
 * Os blocos de `ackSeq + 1` a `nextSeq - 1` estão em trânsito. Só existe em
 * RAM: após um boot, recomeça do bloco mais antigo do log e o servidor
 * descarta os duplicados.
 */
class UploadCursor {
public:
    explicit UploadCursor(size_t window = kUploadWindow, uint32_t ackTimeoutMs = kUploadAckTimeoutMs)
        : _window(window), _ackTimeoutMs(ackTimeoutMs) {}

    /** Salta blocos que o Dongle já apagou (partição cheia). Chamar antes de cada envio. */
    void resync(uint32_t oldestSeq);

    /** Há bloco a enviar e espaço na janela? */
    bool canSend(uint32_t newestSeq) const;

    /** Marca o próximo bloco como enviado e devolve seu `seq`. */
    uint32_t takeNext(uint32_t nowMs);

    /** Processa ACK(n). false = ACK velho, ignorado. */
    bool onAck(uint32_t seq, uint32_t nowMs);

    /** Há blocos em trânsito há mais que o prazo, sem progresso. */
    bool timedOut(uint32_t nowMs) const;

    /** Volta a enviar a partir de `ackSeq + 1` (prazo ou reconexão). */
    void rewind() { _nextSeq = _ackSeq + 1; }

    bool caughtUp(uint32_t newestSeq) const { return _ackSeq >= newestSeq; }
    uint32_t ackSeq() const { return _ackSeq; }
    size_t inFlight() const { return _nextSeq - 1 - _ackSeq; }

private:
    size_t   _window;
    uint32_t _ackTimeoutMs;
    uint32_t _ackSeq = 0;
    uint32_t _nextSeq = 1;
    uint32_t _lastProgressMs = 0;
};

}  // namespace collector
