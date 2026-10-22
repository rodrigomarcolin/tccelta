#pragma once
#include <cstddef>
#include <cstdint>

namespace collector {

constexpr size_t kMaxPidsPerRequest = 6;  // limite do serviço 01 (SAE J1979)
constexpr size_t kPidValueBytes     = 2;  // PIDs coletados têm 1 ou 2 bytes

struct ReadResult {
    uint8_t  pid;
    bool     ok;
    uint8_t  value[kPidValueBytes];
    uint8_t  len;
    uint32_t startMs;  // instante em que a requisição foi enviada
    uint32_t tMs;      // instante da resposta (ou do fim do prazo, se não houve)
};

/**
 * Lê PIDs do serviço 01 no barramento. Hoje só existe a leitura de um PID
 * por requisição (pior caso); uma implementação com vários PIDs por
 * requisição entra trocando a instância, sem mexer no escalonador.
 */
class IPidReader {
public:
    virtual ~IPidReader() = default;

    /** Quantos PIDs cabem numa requisição. */
    virtual size_t maxPidsPerRequest() const = 0;

    /**
     * Lê `n` PIDs (n ≤ maxPidsPerRequest) com prazo `timeoutMs`, escrevendo
     * um `ReadResult` por PID em `out`.
     */
    virtual void read(const uint8_t* pids, size_t n, ReadResult* out, uint32_t timeoutMs) = 0;
};

}  // namespace collector
