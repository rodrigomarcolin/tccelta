#pragma once
#include <vector>
#include "sync/ISyncLink.h"
#include "sync/SyncEngine.h"

/** Link falso: grava os TLVs enviados; `failOnSend` (>= 0) faz o envio de índice N falhar. */
class FakeSyncLink : public datasync::ISyncLink {
public:
    bool send(const uint8_t* tlv, size_t len, datasync::SendStats& stats) override {
        const int index = static_cast<int>(attempts++);
        stats.fragments = 1;
        stats.mtu = mtuValue;
        stats.durUs = 1100;
        if (!isReady || index == failOnSend) {
            stats.ok = false;
            return false;
        }
        stats.ok = true;
        sent.emplace_back(tlv, tlv + len);
        return true;
    }
    bool ready() const override { return isReady; }
    uint16_t mtu() const override { return mtuValue; }

    /** TLVs enviados com o tipo `type` (primeiro byte). */
    std::vector<std::vector<uint8_t>> ofType(uint8_t type) const {
        std::vector<std::vector<uint8_t>> out;
        for (const auto& m : sent) {
            if (m[0] == type) out.push_back(m);
        }
        return out;
    }

    std::vector<std::vector<uint8_t>> sent;
    size_t attempts = 0;
    int failOnSend = -1;
    bool isReady = true;
    uint16_t mtuValue = 247;
};

/** `last_cs` em RAM; registra quantas mensagens já tinham sido enviadas ao gravar (ordem). */
class FakeSyncPersistence : public datasync::ISyncStatePersistence {
public:
    uint32_t loadLastAcceptedCs() override { return lastCs; }
    bool saveLastAcceptedCs(uint32_t cs) override {
        if (fail) return false;
        lastCs = cs;
        sentAtSave = link ? link->sent.size() : 0;
        ++saves;
        return true;
    }

    uint32_t lastCs = 0;
    bool fail = false;
    int saves = 0;
    size_t sentAtSave = 0;
    const FakeSyncLink* link = nullptr;
};
