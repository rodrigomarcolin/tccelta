#pragma once
#include "can/ICanBus.h"
#include <vector>
#include <initializer_list>
#include <cstring>

/**
 * Dublê de ICanBus para testes: `send()` registra o frame em `sent` para
 * inspeção pelo teste e libera, em ordem, os frames programados via
 * `enqueue()` — como uma ECU real, que só responde depois de receber a
 * requisição. Isso importa porque o cliente ISO-TP esvazia a recepção antes
 * de enviar: frames já presentes nesse momento são sobras e devem ser
 * descartados. Para simular essas sobras, use `preload()`.
 */
class FakeCanBus : public ICanBus {
public:
    bool begin() override { return true; }

    bool send(const CanFrame& frame) override {
        sent.push_back(frame);
        _rx.insert(_rx.end(), _pending.begin(), _pending.end());
        _pending.clear();
        return true;
    }

    bool receive(CanFrame& frame) override {
        if (_rx.empty()) return false;
        frame = _rx.front();
        _rx.erase(_rx.begin());
        return true;
    }

    /** Frame que o peer manda em resposta ao próximo `send()`. */
    void enqueue(uint32_t id, std::initializer_list<uint8_t> bytes) {
        _pending.push_back(makeFrame(id, bytes));
    }

    /** Frame que já está na recepção antes de qualquer `send()` (sobra). */
    void preload(uint32_t id, std::initializer_list<uint8_t> bytes) {
        _rx.push_back(makeFrame(id, bytes));
    }

    std::vector<CanFrame> sent;

private:
    std::vector<CanFrame> _rx;
    std::vector<CanFrame> _pending;

    static CanFrame makeFrame(uint32_t id, std::initializer_list<uint8_t> bytes) {
        CanFrame f = {};
        f.id  = id;
        f.dlc = 8;
        f.extended = id > 0x7FF;  // como um driver real reporta IDs de 29 bits
        size_t i = 0;
        for (uint8_t b : bytes) {
            if (i >= sizeof(f.data)) break;
            f.data[i++] = b;
        }
        return f;
    }
};
