#pragma once
#include "can/ICanBus.h"
#include <vector>
#include <initializer_list>
#include <cstring>

/**
 * Dublê de ICanBus para testes: `receive()` devolve, em ordem, os frames
 * previamente enfileirados via `enqueue()`; `send()` só registra o frame em
 * `sent` para inspeção pelo teste (nenhum loopback automático entre
 * send()/receive() — cada teste programa exatamente os frames que o peer
 * simulado "responderia").
 */
class FakeCanBus : public ICanBus {
public:
    bool begin() override { return true; }

    bool send(const CanFrame& frame) override {
        sent.push_back(frame);
        return true;
    }

    bool receive(CanFrame& frame) override {
        if (_rx.empty()) return false;
        frame = _rx.front();
        _rx.erase(_rx.begin());
        return true;
    }

    void enqueue(uint32_t id, std::initializer_list<uint8_t> bytes) {
        CanFrame f = {};
        f.id  = id;
        f.dlc = 8;
        size_t i = 0;
        for (uint8_t b : bytes) {
            if (i >= sizeof(f.data)) break;
            f.data[i++] = b;
        }
        _rx.push_back(f);
    }

    std::vector<CanFrame> sent;

private:
    std::vector<CanFrame> _rx;
};
