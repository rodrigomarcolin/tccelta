#pragma once
#include <cstddef>
#include <cstdint>

namespace collector {

/**
 * Ponto de encaixe da segurança Dongle ↔ Servidor. O Dongle assina o que
 * envia (BATCH, TIME_REPORT) e verifica o que recebe (ACK, TIME_RESP) antes
 * de agir. A implementação real fica a cargo da parte de segurança.
 */
class IMessageAuth {
public:
    virtual ~IMessageAuth() = default;
    virtual size_t tagSize() const = 0;
    virtual void sign(const uint8_t* message, size_t len, uint8_t* tagOut) = 0;
    virtual bool verify(const uint8_t* message, size_t len, const uint8_t* tag) = 0;
};

/** Sem autenticação: não acrescenta nada e aceita tudo. */
class NoAuth : public IMessageAuth {
public:
    size_t tagSize() const override { return 0; }
    void sign(const uint8_t*, size_t, uint8_t*) override {}
    bool verify(const uint8_t*, size_t, const uint8_t*) override { return true; }
};

}  // namespace collector
