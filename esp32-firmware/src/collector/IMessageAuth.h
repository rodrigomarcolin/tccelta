#pragma once
#include <cstddef>
#include <cstdint>

namespace collector {

/**
 * Autentica a área de dados de um bloco (cabeçalho + registros, `len` =
 * `tamanho`). A implementação acrescenta o separador de contexto
 * ("MALCONG" || 0x03 || DevId) antes dos dados. `tagSize()` é constante
 * durante toda a vida da instância. Não é reentrante: uma instância por tarefa.
 * Implementações: `NoAuth` (aqui), `security::HmacBlockAuth` e `EcdsaBlockAuth`.
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
