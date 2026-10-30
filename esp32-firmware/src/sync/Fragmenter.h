#pragma once
#include <cstddef>
#include <cstdint>

namespace datasync {

/**
 * Enquadramento do SYNC_TX (spec fase-1/04 §5, Fig. 7): cada notificação é
 * `C | pedaço do TLV`, com C = bit 7 (primeiro fragmento) | contador mod 128.
 * O pedaço útil é `MTU - 4`; MTU abaixo de 23 vale 23.
 */
class Fragmenter {
public:
    Fragmenter(const uint8_t* msg, size_t len, uint16_t mtu)
        : _msg(msg), _len(len), _chunk((mtu < 23 ? 23 : mtu) - 4) {}

    /** Quantidade de fragmentos da mensagem. */
    uint16_t count() const { return static_cast<uint16_t>((_len + _chunk - 1) / _chunk); }

    /** Próximo fragmento em `out` (capacidade >= MTU - 3); false quando acabou. */
    bool next(uint8_t* out, size_t& outLen) {
        if (_off >= _len) return false;
        size_t n = _len - _off;
        if (n > _chunk) n = _chunk;
        out[0] = static_cast<uint8_t>((_index == 0 ? 0x80 : 0x00) | (_index & 0x7F));
        for (size_t i = 0; i < n; ++i) out[1 + i] = _msg[_off + i];
        _off += n;
        ++_index;
        outLen = n + 1;
        return true;
    }

private:
    const uint8_t* _msg;
    size_t _len;
    size_t _chunk;
    size_t _off = 0;
    size_t _index = 0;
};

}  // namespace datasync
