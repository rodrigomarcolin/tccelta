#pragma once
#include <cstddef>
#include <cstdint>

#include "collector/BlockFormat.h"
#include "collector/Clock.h"
#include "collector/IBlockStore.h"
#include "collector/IMessageAuth.h"
#include "security/HandshakeAuth.h"
#include "sync/ISyncLink.h"
#include "sync/SyncProtocol.h"

/**
 * Lado dongle do handshake, cursor de envio, lotes, avisos e erros
 * (spec fase-1/05). C++ puro: a SyncTask é a casca FreeRTOS.
 */
namespace sync {

/** Onde o SyncEngine guarda o maior c_S aceito (NVS, spec fase-1/03 §6). */
class ISyncStatePersistence {
public:
    virtual ~ISyncStatePersistence() = default;
    virtual uint32_t loadLastAcceptedCs() = 0;
    virtual bool saveLastAcceptedCs(uint32_t cs) = 0;
};

/** Gancho do modo diagnóstico pelo desafio (spec fase-1/05 §11): vazio na Fase 1. */
class IDiagGate {
public:
    virtual ~IDiagGate() = default;
    virtual void grant(uint16_t minutes) = 0;
};

struct SyncDeps {
    collector::IBlockStore& store;
    ISyncStatePersistence& persistence;
    security::IHandshakeAuth& handshakeAuth;  // K_auth
    collector::IMessageAuth& lossAuth;        // K_rec / k_sig / none: assina o LOSS
    ISyncLink& link;
    const collector::IClock& clock;
    const uint8_t* devId;  // 8 B
    uint16_t bootId;
    IDiagGate* diagGate;  // nullptr na Fase 1
};

struct SyncStatus {
    bool connected = false, subscribed = false, sessionOpen = false, batchActive = false;
    uint32_t ackLocal = 0, cursor = 0, batchEnd = 0, head = 0, tail = 0;
    uint32_t lastAcceptedCs = 0, respondedCs = 0;
    uint8_t flags = 0;
    uint16_t mtu = 0;
};

/**
 * Único dono do estado do protocolo; só a SyncTask o chama. Emite os eventos
 * da spec fase-1/06 diretamente.
 */
class SyncEngine {
public:
    explicit SyncEngine(const SyncDeps& deps);

    void begin();  // carrega last_cs; ackLocal = TAIL efetivo - 1
    void onConnected(uint32_t generation);
    void onDisconnected();
    void onSubscribed(bool on);
    void onFlushed(uint32_t seq);  // seq = 0: só repetir o aviso (noticePending)
    void onMessage(const uint8_t* tlv, size_t len, uint32_t queuedUs);
    bool sendNextBlock();  // no máximo uma mensagem de bloco por chamada
    void tick();           // a cada ~1 s

    bool batchActive() const { return _batchActive; }
    uint32_t generation() const { return _generation; }
    SyncStatus status() const;

    /** Só COLLECTOR_EVENTS (AT#ACK): processa um ACK com tag válida como se viesse do app. */
    void injectAck(uint32_t ackSeq);

    static constexpr uint32_t kNoticeRetryMs = 15000;
    static constexpr uint32_t kMaxLossRun = 64;

private:
    enum class NoticeReason : uint8_t { Subscribe, Flush, Ack, Resync, Overflow, Flash, Retry };
    enum class BatchEnd : uint8_t { Done, Link, Overflow, AppError, Superseded, Empty };

    SyncDeps _d;

    uint32_t _ackLocal = 0, _cursor = 0, _batchEnd = 0;
    uint32_t _lastAcceptedCs = 0, _respondedCs = 0;
    bool _hasResponded = false, _sessionOpen = false;
    bool _noHsConn = true, _noHsBoot = true, _flashHigh = false;
    uint32_t _overflowSeen = 0;
    bool _connected = false, _subscribed = false;
    uint32_t _generation = 0;
    bool _batchActive = false;
    uint32_t _batchSent = 0, _batchBytes = 0;
    uint64_t _batchStartUs = 0;
    uint32_t _lastNoticeMs = 0;

    collector::Block _blk;   // ~1,1 KB cada: SyncEngine é estático no firmware
    collector::Block _loss;
    uint8_t _tx[kMaxTxMessage];

    // Mensagens
    void handleChallenge(const uint8_t* v, size_t len, uint32_t queuedUs, uint64_t startUs);
    void handleAck(const uint8_t* v, size_t len);
    void handleBatchRequest(const uint8_t* v, size_t len);
    void handleAppError(const uint8_t* v, size_t len);

    // Envio
    bool sendTlv(uint8_t type, size_t valueLen, SendStats& stats);
    void sendError(ErrorCode code);
    void sendNotice(NoticeReason reason);

    // Cursor
    uint32_t effectiveTail();
    void advanceAck(uint32_t ack, size_t* freedSegs = nullptr);
    void checkOverflow();
    void endBatch(BatchEnd reason);
    void setSession(bool open, const char* reason);
    bool sendLoss(uint32_t s, uint32_t to, const char* reason);
    uint32_t findLossEnd(uint32_t s, uint32_t limit);
    uint8_t noticeFlags() const;
    uint32_t sinceUs(uint64_t t0) const { return static_cast<uint32_t>(_d.clock.nowUs() - t0); }
};

}  // namespace sync
