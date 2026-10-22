#pragma once
#include <cstddef>
#include <cstdint>
#include "collector/BlockFormat.h"
#include "collector/IMessageAuth.h"

/**
 * Mensagens binárias do canal de dados (Dongle ↔ App ↔ Servidor).
 *
 * Mensagem: tipo u8 | corpo | tag de autenticação (tamanho de IMessageAuth)
 * No BLE, cada mensagem vai num quadro: tamanho u16 (little-endian) | mensagem,
 * o que permite fragmentá-la em várias notificações.
 *
 *   BATCH       (D→S) oldestSeq u32 | bloco
 *   ACK         (S→D) seq u32
 *   TIME_REQ    (D→S) boot_id u16 | t1 u32
 *   TIME_RESP   (S→D) boot_id u16 | t1 u32   (eco do pedido; amarra a resposta a ele)
 *   TIME_REPORT (D→S) boot_id u16 | t1 u32 | t2 u32
 */
namespace collector {

enum class MessageType : uint8_t {
    Batch        = 0x01,
    Ack          = 0x02,
    TimeRequest  = 0x03,
    TimeResponse = 0x04,
    TimeReport   = 0x05,
};

constexpr size_t kMaxTagSize      = 32;
constexpr size_t kFrameHeaderSize = 2;
constexpr size_t kMaxMessageSize  = 1 + 4 + kBlockSize + kMaxTagSize;

struct TimeExchange {
    uint16_t bootId;
    uint32_t t1Ms;
    uint32_t t2Ms;  // só no TIME_REPORT
};

/**
 * Codifica e decodifica mensagens, assinando as que saem e verificando as que
 * chegam. Os encoders devolvem o tamanho da mensagem (0 se não coube).
 */
class DataProtocol {
public:
    explicit DataProtocol(IMessageAuth& auth) : _auth(auth) {}

    size_t encodeBatch(uint32_t oldestSeq, const Block& block, uint8_t* out, size_t cap);
    size_t encodeTimeRequest(const TimeExchange& request, uint8_t* out, size_t cap);
    size_t encodeTimeReport(const TimeExchange& report, uint8_t* out, size_t cap);
    /** ACK como o servidor o enviaria (bancada: AT#ACK; testes). */
    size_t encodeAck(uint32_t seq, uint8_t* out, size_t cap);

    /** Tipo de uma mensagem recebida, já verificada. false = inválida/não autêntica. */
    bool verifiedType(const uint8_t* msg, size_t len, MessageType& type);
    bool decodeAck(const uint8_t* msg, size_t len, uint32_t& seq);
    bool decodeTimeResponse(const uint8_t* msg, size_t len, TimeExchange& response);

private:
    IMessageAuth& _auth;

    size_t seal(uint8_t* out, size_t bodyLen, size_t cap);
    /** Tamanho do corpo (sem tag) se a mensagem é autêntica; -1 caso contrário. */
    int open(const uint8_t* msg, size_t len);
};

/** Envolve uma mensagem num quadro (tamanho u16 | mensagem). */
size_t frameMessage(const uint8_t* msg, size_t len, uint8_t* out, size_t cap);

/**
 * Remonta quadros a partir de fragmentos recebidos em sequência. Quadros
 * maiores que o buffer são descartados inteiros.
 */
template <size_t Capacity>
class FrameDecoder {
public:
    /** Consome `len` bytes; chama `onMessage(msg, msgLen)` para cada quadro completo. */
    template <typename OnMessage>
    void push(const uint8_t* data, size_t len, OnMessage onMessage) {
        for (size_t i = 0; i < len; ++i) {
            if (_headerRead < kFrameHeaderSize) {
                _header[_headerRead++] = data[i];
                if (_headerRead == kFrameHeaderSize) {
                    _expected = getU16(_header);
                    _received = 0;
                    if (_expected == 0) reset();
                }
                continue;
            }
            if (_received < Capacity) _buffer[_received] = data[i];
            if (++_received == _expected) {
                if (_expected <= Capacity) onMessage(_buffer, _expected);
                reset();
            }
        }
    }

    void reset() { _headerRead = 0; _expected = 0; _received = 0; }

private:
    uint8_t _header[kFrameHeaderSize] = {};
    uint8_t _buffer[Capacity] = {};
    size_t  _headerRead = 0;
    size_t  _expected = 0;
    size_t  _received = 0;
};

}  // namespace collector
