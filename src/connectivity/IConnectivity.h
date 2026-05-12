#pragma once
#include <cstdint>
#include <functional>

using CommandCallback = std::function<void(const uint8_t* data, size_t len)>;

/**
 * Opaque session context — reserved for future auth/crypto state.
 * Stub fields to nullptr until SecureBleConnectivity is implemented.
 */
struct SessionContext {
    void* token = nullptr;  // TODO: auth token
    void* key   = nullptr;  // TODO: session key
};

/**
 * Abstração de Transporte para o canal requisição (comando) / resposta.
 *
 * Supõe que a classe que a implementa recebe um comando de algum modo,
 * e fornece:
 * 
 *  setOnCommandReceivedCallback : setter da função a ser chamada quando um comando for recebido.
 *  sendResponse : função que envia uma resposta ao client
 */
class IConnectivity {
public:
    virtual ~IConnectivity() = default;

    virtual bool begin() = 0;

    /** Send a response payload (binary-capable). */
    virtual void sendResponse(const uint8_t* data, size_t len) = 0;

    /** Register the callback invoked when a command arrives. */
    virtual void setOnCommandReceivedCallback(CommandCallback cb) = 0;

    /**
     * Returns the current session context, or nullptr if none.
     * Override in SecureBleConnectivity to expose auth state.
     */
    virtual void* getSessionContext() { return nullptr; }
};
