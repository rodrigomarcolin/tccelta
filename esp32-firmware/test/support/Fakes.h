#pragma once
#include <map>
#include <vector>
#include "collector/Clock.h"
#include "collector/Collector.h"
#include "collector/IBlockStore.h"
#include "obd2/IObd2.h"

/** Relógio controlado pelo teste. */
class FakeClock : public collector::IClock {
public:
    uint32_t nowMs() const override { return now; }
    void advance(uint32_t ms) { now += ms; }
    uint32_t now = 1000;
};

/**
 * ECU simulada: cada leitura consome `latencyMs` do relógio; PIDs em
 * `silent` não respondem e consomem o prazo inteiro.
 */
class FakeObd2 : public IObd2 {
public:
    explicit FakeObd2(FakeClock& clock) : _clock(clock) {}

    bool begin() override { return true; }
    int readPid(uint8_t, uint8_t, uint8_t*, size_t) override { return -1; }
    int readDtc(uint8_t, uint16_t*, size_t) override { return -1; }
    int readFreezeFramePid(uint8_t, uint8_t*, size_t) override { return -1; }

    int readEnginePid(uint8_t pid, uint8_t* buf, size_t maxLen, uint32_t timeoutMs) override {
        requests.push_back(pid);
        if (silent.count(pid)) {
            _clock.advance(timeoutMs);
            return -1;
        }
        _clock.advance(latencyMs);
        if (maxLen < 2) return -1;
        buf[0] = pid;
        buf[1] = 0xAB;
        return 2;
    }

    uint32_t latencyMs = 10;
    std::map<uint8_t, bool> silent;
    std::vector<uint8_t> requests;

private:
    FakeClock& _clock;
};

/** Pool de dois buffers que registra o que foi entregue. */
class FakePool : public collector::IBlockPool {
public:
    collector::Block* acquire() override {
        if (free.empty()) return nullptr;
        collector::Block* block = free.back();
        free.pop_back();
        return block;
    }
    void submit(collector::Block* block) override {
        submitted.push_back(*block);
        free.push_back(block);  // a "Flush" grava na hora
    }
    void release(collector::Block* block) override { free.push_back(block); }

    collector::Block buffers[2] = {};
    std::vector<collector::Block*> free = {&buffers[0], &buffers[1]};
    std::vector<collector::Block> submitted;
};

class FakeValues : public collector::IValueSink {
public:
    void update(const collector::ReadResult& result) override { updates.push_back(result); }
    std::vector<collector::ReadResult> updates;
};

/** Log em memória; `failAppends` faz as próximas N gravações falharem. */
class MemoryBlockStore : public collector::IBlockStore {
public:
    bool append(const collector::Block& block) override {
        if (failAppends > 0) {
            --failAppends;
            return false;
        }
        blocks.push_back(block);
        return true;
    }
    bool read(uint32_t seq, collector::Block& out) override {
        for (const auto& block : blocks) {
            if (collector::blockSeq(block) == seq) {
                out = block;
                return true;
            }
        }
        return false;
    }
    void deleteUpTo(uint32_t upTo) override {
        std::vector<collector::Block> kept;
        for (const auto& block : blocks) {
            if (collector::blockSeq(block) > upTo) kept.push_back(block);
        }
        blocks = kept;
    }
    bool empty() const override { return blocks.empty(); }
    uint32_t oldestSeq() const override { return blocks.empty() ? 0 : collector::blockSeq(blocks.front()); }
    uint32_t newestSeq() const override { return blocks.empty() ? 0 : collector::blockSeq(blocks.back()); }
    uint32_t lostBlocks() const override { return 0; }

    std::vector<collector::Block> blocks;
    int failAppends = 0;
};

class FakeSeqPersistence : public collector::ISeqPersistence {
public:
    void saveLastSeq(uint32_t seq) override { saved.push_back(seq); }
    std::vector<uint32_t> saved;
};
