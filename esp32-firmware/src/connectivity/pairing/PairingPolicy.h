#pragma once
#include <cstddef>
#include <cstdint>

namespace pairing {

constexpr size_t   kMaxLinks = 4;
constexpr uint16_t kNoConn   = 0xFFFF;  // = BLE_HS_CONN_HANDLE_NONE

/** Motivos HCI usados pelo dongle ao derrubar uma conexão (spec fase-2/01 §5.4). */
constexpr uint8_t kHciAuthFailure  = 0x05;
constexpr uint8_t kHciConnLimit    = 0x09;
constexpr uint8_t kHciRemoteUser   = 0x13;

enum class AdvMode : uint8_t { Off, Normal, Pairing };
enum class WindowState : uint8_t { Closed, Wiping, Open };

struct Timings {
    uint32_t windowMs;
    uint32_t linkAuthTimeoutMs;
    uint32_t pairAuthTimeoutMs;
    uint32_t rejectRetryMs = 2000;
};

/** Espelho do NimBLEConnInfo + status do ENC_CHANGE (sem tipos da NimBLE). */
struct EncryptionInfo {
    int     status = 0;       // 0 = ok; códigos BLE_HS_* da NimBLE
    bool    encrypted = false;
    bool    bonded = false;
    uint8_t keySize = 0;
    bool    lesc = true;      // bond no store com sc == 1 (só faz sentido com bonded)
    bool    persisted = true; // bond no store com LTK presente
};

struct Reject {
    uint16_t conn = kNoConn;
    uint8_t  hciReason = 0;  // 0x05, 0x09
};

/** O que a camada RTOS executa, fora do mutex, nesta ordem dos campos. */
struct Actions {
    bool     advertisingOffFirst = false;
    bool     disconnectAll = false;
    bool     deleteAllBonds = false;
    uint16_t deleteBond[kMaxLinks] = {};  // apagar o bond do peer desta conexão
    uint8_t  deleteBondCount = 0;
    bool     recoverStore = false;        // apagar nimble_bond e reiniciar
    Reject   reject[kMaxLinks + 1] = {};  // + a conexão que acabou de chegar sem slot
    uint8_t  rejectCount = 0;
    uint16_t authorize = kNoConn;
    bool     deauthorize = false;
    uint16_t deauthorizeConn = kNoConn;
    bool     setBonding = false;
    bool     bondingAllowed = false;
    bool     setAdvertising = false;
    AdvMode  adv = AdvMode::Off;
    bool     windowOpen = false;
    bool     hasBond = false;
};

/**
 * Núcleo puro da política de pareamento (spec fase-2/02 §4.3): janela de
 * pareamento, tabela de conexões e regra de autorização. Sem NimBLE nem
 * FreeRTOS; o tempo entra como parâmetro. Não é thread-safe: o
 * PairingController serializa as chamadas.
 */
class PairingPolicy {
public:
    PairingPolicy(const Timings& timings, uint8_t maxConnections);

    void begin(uint32_t nowMs, bool hasBond, Actions& out);
    void onButtonHold(uint32_t nowMs, Actions& out);
    void onBondsDeleted(bool ok, uint32_t nowMs, Actions& out);
    void onLinkUp(uint16_t conn, uint32_t nowMs, Actions& out);
    void onEncryptionChange(uint16_t conn, const EncryptionInfo& info, uint32_t nowMs, Actions& out);
    void onLinkDown(uint16_t conn, uint32_t nowMs, Actions& out);
    void tick(uint32_t nowMs, Actions& out);

    WindowState window() const { return _window; }
    uint32_t    windowDeadline() const { return _deadline; }
    bool        bondingAllowed() const { return _window == WindowState::Open; }
    bool        hasBond() const { return _hasBond; }
    uint16_t    authorizedConn() const;
    bool        isAuthorized(uint16_t conn) const;
    AdvMode     advMode() const;
    size_t      activeLinks() const;

private:
    enum class LinkState : uint8_t { Free, Pending, Authorized, Rejecting };

    struct Link {
        uint16_t  conn = kNoConn;
        LinkState state = LinkState::Free;
        uint32_t  connectedAt = 0;
        uint32_t  rejectAt = 0;
        uint8_t   rejectHci = 0;
        bool      admittedInWindow = false;
        bool      wasAuthorized = false;
    };

    Timings  _t;
    uint8_t  _maxConnections;
    WindowState _window = WindowState::Closed;
    uint32_t _deadline = 0;
    uint32_t _openedAt = 0;
    bool     _consumed = false;
    bool     _hasBond = false;
    Link     _links[kMaxLinks];

    // Último estado de advertising publicado em Actions (para só pedir quando muda).
    bool     _advPublished = false;
    AdvMode  _pubAdv = AdvMode::Off;
    bool     _pubWindow = false;
    bool     _pubBond = false;

    Link* find(uint16_t conn);
    Link* allocate(uint16_t conn);
    bool  hasAuthorizedLink() const;
    void  reject(Link& link, uint8_t hci, uint32_t now, Actions& out);
    void  addReject(Actions& out, uint16_t conn, uint8_t hci);
    void  addDeleteBond(Actions& out, uint16_t conn);
    void  closeWindow(const char* reason, uint32_t now);
    void  finish(Actions& out, bool forceAdv = false);
};

}  // namespace pairing
