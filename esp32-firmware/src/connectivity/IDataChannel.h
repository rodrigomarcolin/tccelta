#pragma once
#include <cstddef>
#include <cstdint>

/**
 * Canal binário de dados Dongle → App (blocos, hora). Ver README.md desta
 * pasta para o contrato completo com a coleta.
 */
class IDataChannel {
public:
    virtual ~IDataChannel() = default;

    /** Envia uma mensagem inteira (fragmentando se preciso). false = sem conexão. */
    virtual bool send(const uint8_t* message, size_t len) = 0;
};
