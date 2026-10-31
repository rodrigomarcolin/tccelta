#pragma once
#include <cstdint>

enum class CanBitrate : uint8_t { Kbps250, Kbps500 };

struct CanFrame {
    uint32_t id;
    uint8_t  dlc;
    uint8_t  data[8];
    bool     extended = false;  // true = identificador de 29 bits
};

/** Estado do controlador CAN, para depuração (evento CAN_STATUS). */
struct CanStatus {
    enum class State : uint8_t { Stopped, Running, BusOff, Recovering };
    State    state     = State::Stopped;
    uint32_t txErrors  = 0;  // TEC: >= 128 = erro passivo, > 255 = bus-off
    uint32_t rxErrors  = 0;  // REC
    uint32_t txFailed  = 0;  // quadros cuja transmissão falhou
    uint32_t rxMissed  = 0;  // perdidos com a fila de recepção cheia
    uint32_t rxOverrun = 0;  // perdidos por estouro da FIFO do controlador
    uint32_t arbLost   = 0;  // arbitragens perdidas
    uint32_t busErrors = 0;  // erros de barramento (bit, stuff, forma, ACK, CRC)
    uint32_t txQueued  = 0;  // na fila de transmissão ou aguardando concluir
    uint32_t rxQueued  = 0;  // na fila de recepção, ainda não lidos
    uint32_t txDropped = 0;  // send() recusado localmente (fila de transmissão cheia)
};

/**
 * Abstração da camada CAN.
 *
 * Fornece funções para enviar e receber frames.
 */
class ICanBus {
public:
    virtual ~ICanBus() = default;

    virtual bool begin() = 0;

    /** Transmit a frame. Returns true on success. */
    virtual bool send(const CanFrame& frame) = 0;

    /** Non-blocking receive. Returns true if a frame was available before timeout. */
    virtual bool receive(CanFrame& frame) = 0;

    /** Estado do controlador. Devolve false se a implementação não o expõe. */
    virtual bool status(CanStatus& out) const {
        (void)out;
        return false;
    }
};
