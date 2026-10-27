#pragma once
#include <algorithm>
#include <cstring>
#include <map>
#include <set>
#include <vector>
#include "collector/Clock.h"
#include "collector/Collector.h"
#include "collector/IBlockStore.h"
#include "collector/IMessageAuth.h"
#include "collector/SegmentFormat.h"
#include "collector/SeqReserve.h"
#include "obd2/IObd2.h"

/** Relógio controlado pelo teste; `us` avança junto com `now` (e sozinho em `advanceUs`). */
class FakeClock : public collector::IClock {
public:
    uint32_t nowMs() const override { return now; }
    uint64_t nowUs() const override { return us; }
    void advance(uint32_t ms) { now += ms; us += static_cast<uint64_t>(ms) * 1000u; }
    void advanceUs(uint64_t micros) { us += micros; }
    uint32_t now = 1000;
    uint64_t us = 1000000;
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
    int indexOf(const collector::Block* block) const override {
        return static_cast<int>(block - buffers);
    }

    collector::Block buffers[2] = {};
    std::vector<collector::Block*> free = {&buffers[0], &buffers[1]};
    std::vector<collector::Block> submitted;
};

class FakeValues : public collector::IValueSink {
public:
    void update(const collector::ReadResult& result) override {
        updates.push_back(result);
        values.update(result);
    }
    bool get(uint8_t pid, collector::CurrentValues::Value& out) const override {
        return values.get(pid, out);
    }
    collector::CurrentValues values;
    std::vector<collector::ReadResult> updates;
};

/**
 * Log em memória (spec fase-1/02 §9): `blocks` guarda os blocos com tag e CRC;
 * `failAppends` faz as próximas N gravações falharem; `corruptSeqs` simula
 * registros ilegíveis; `deleteUpTo` apaga por bloco (o real apaga segmentos).
 */
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
    collector::ReadStatus read(uint32_t seq, collector::Block& out) override {
        if (corruptSeqs.count(seq)) {
            for (const auto& block : blocks) {
                if (collector::blockSeq(block) == seq) return collector::ReadStatus::Corrupt;
            }
        }
        for (const auto& block : blocks) {
            if (collector::blockSeq(block) == seq) {
                out = block;
                if (flippedSeqs.count(seq)) out.bytes[collector::kHeaderSize] ^= 0xFF;  // CRC deixa de bater
                return collector::ReadStatus::Ok;
            }
        }
        return collector::ReadStatus::NotFound;
    }
    uint32_t nextStoredSeq(uint32_t fromSeq) const override {
        uint32_t best = 0;
        for (const auto& block : blocks) {
            const uint32_t seq = collector::blockSeq(block);
            if (seq >= fromSeq && (best == 0 || seq < best)) best = seq;
        }
        return best;
    }
    size_t deleteUpTo(uint32_t upTo) override {
        const size_t before = blocks.size();
        std::vector<collector::Block> kept;
        for (const auto& block : blocks) {
            if (collector::blockSeq(block) > upTo) kept.push_back(block);
        }
        blocks = kept;
        return before - blocks.size();
    }
    void startNewSegment() override { ++newSegments; }
    bool empty() const override { return blocks.empty(); }
    uint32_t oldestSeq() const override { return blocks.empty() ? 0 : collector::blockSeq(blocks.front()); }
    uint32_t newestSeq() const override { return blocks.empty() ? 0 : collector::blockSeq(blocks.back()); }
    uint32_t lostBlocks() const override { return 0; }
    uint32_t overflowUpTo() const override { return overflow; }
    uint16_t usagePermille() const override { return usage; }
    bool corrupt(uint32_t seq) override {
        flippedSeqs.insert(seq);
        return true;
    }

    std::vector<collector::Block> blocks;
    std::set<uint32_t> corruptSeqs;  // read -> Corrupt (estrutura quebrada)
    std::set<uint32_t> flippedSeqs;  // read -> Ok mas com o CRC errado
    int failAppends = 0;
    int newSegments = 0;
    uint32_t overflow = 0;
    uint16_t usage = 0;
};

/** Conteúdo da tag = primeiros T bytes de um contador, para conferir as chamadas. */
class FakeAuth : public collector::IMessageAuth {
public:
    explicit FakeAuth(size_t tagLen = 32) : tagLen(tagLen) {}
    size_t tagSize() const override { return tagLen; }
    void sign(const uint8_t*, size_t len, uint8_t* tagOut) override {
        signedLengths.push_back(len);
        for (size_t i = 0; i < tagLen; ++i) tagOut[i] = static_cast<uint8_t>(signedLengths.size() + i);
    }
    bool verify(const uint8_t*, size_t, const uint8_t*) override { return true; }

    size_t tagLen;
    std::vector<size_t> signedLengths;
};

/** NVS falsa: registra as reservas gravadas e a ordem relativa ao `append` (via `log`). */
class FakeSeqReservePersistence : public collector::ISeqReservePersistence {
public:
    bool saveSeqReserve(uint32_t reserve) override {
        if (fail) return false;
        saved.push_back(reserve);
        return true;
    }
    std::vector<uint32_t> saved;
    bool fail = false;
};
