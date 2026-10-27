#include "connectivity/pairing/PairingPolicy.h"
#include "collector/Events.h"
#include "connectivity/pairing/PairTime.h"

namespace pairing {

namespace {

enum class StatusClass : uint8_t { PeerReject, LocalReject, Timeout, Other };

// Faixas de erro do Security Manager da NimBLE (ble_hs.h): BLE_HS_ERR_SM_PEER_BASE / _US_BASE.
constexpr int kSmPeerBase = 0x500;
constexpr int kSmUsBase   = 0x400;
constexpr int kHsTimeout  = 13;  // BLE_HS_ETIMEOUT

StatusClass classify(int status) {
    if (status >= kSmPeerBase && status < kSmPeerBase + 0x100) return StatusClass::PeerReject;
    if (status >= kSmUsBase && status < kSmUsBase + 0x100) return StatusClass::LocalReject;
    if (status == kHsTimeout) return StatusClass::Timeout;
    return StatusClass::Other;
}

const char* className(StatusClass c) {
    switch (c) {
        case StatusClass::PeerReject:  return "peer_reject";
        case StatusClass::LocalReject: return "local_reject";
        case StatusClass::Timeout:     return "timeout";
        case StatusClass::Other:       break;
    }
    return "other";
}

void emitLinkEnc(uint16_t conn, bool up, const EncryptionInfo& info, bool fresh, const char* reason,
                 uint32_t dur) {
    collector::Event("LINK_ENC").num("conn", conn).flag("up", up).flag("bonded", info.bonded).flag("fresh", fresh)
        .num("key", info.keySize).token("reason", reason).num("rc", info.status).num("dur", dur);
}

void emitPairResult(uint16_t conn, bool ok, const char* reason, bool late, int rc, uint32_t dur) {
    collector::Event("PAIR_RESULT").num("conn", conn).flag("ok", ok).token("reason", reason).flag("late", late)
        .num("rc", rc).num("dur", dur);
}

void emitReject(uint16_t conn, const char* reason, uint8_t hci, uint32_t dur) {
    collector::Event("CONN_REJECT").num("conn", conn).token("reason", reason).hex8("hci", hci).num("dur", dur);
}

void emitBondDeletePolicy() {
    collector::Event("BOND_DELETE").token("reason", "policy").num("count", 1).flag("ok", true).num("attempts", 1)
        .num("dur", 0);
}

}  // namespace

PairingPolicy::PairingPolicy(const Timings& timings, uint8_t maxConnections)
    : _t(timings), _maxConnections(maxConnections) {}

// ── Consultas ────────────────────────────────────────────────────────────────

uint16_t PairingPolicy::authorizedConn() const {
    for (const Link& l : _links) {
        if (l.state == LinkState::Authorized) return l.conn;
    }
    return kNoConn;
}

bool PairingPolicy::isAuthorized(uint16_t conn) const {
    for (const Link& l : _links) {
        if (l.state == LinkState::Authorized && l.conn == conn) return true;
    }
    return false;
}

bool PairingPolicy::hasAuthorizedLink() const { return authorizedConn() != kNoConn; }

size_t PairingPolicy::activeLinks() const {
    size_t n = 0;
    for (const Link& l : _links) {
        if (l.state != LinkState::Free) ++n;
    }
    return n;
}

AdvMode PairingPolicy::advMode() const {
    if (_window == WindowState::Wiping) return AdvMode::Off;
    if (hasAuthorizedLink()) return AdvMode::Off;
    if (activeLinks() >= _maxConnections) return AdvMode::Off;
    if (_window == WindowState::Open) return AdvMode::Pairing;
    return AdvMode::Normal;
}

// ── Auxiliares ───────────────────────────────────────────────────────────────

PairingPolicy::Link* PairingPolicy::find(uint16_t conn) {
    for (Link& l : _links) {
        if (l.state != LinkState::Free && l.conn == conn) return &l;
    }
    return nullptr;
}

PairingPolicy::Link* PairingPolicy::allocate(uint16_t conn) {
    if (Link* existing = find(conn)) return existing;
    for (Link& l : _links) {
        if (l.state == LinkState::Free) {
            l = Link{};
            l.conn = conn;
            return &l;
        }
    }
    return nullptr;
}

void PairingPolicy::addReject(Actions& out, uint16_t conn, uint8_t hci) {
    if (out.rejectCount < kMaxLinks + 1) {
        Reject& r = out.reject[out.rejectCount++];
        r.conn = conn;
        r.hciReason = hci;
    }
}

void PairingPolicy::addDeleteBond(Actions& out, uint16_t conn) {
    if (out.deleteBondCount < kMaxLinks) out.deleteBond[out.deleteBondCount++] = conn;
}

void PairingPolicy::reject(Link& link, uint8_t hci, uint32_t now, Actions& out) {
    link.state = LinkState::Rejecting;
    link.rejectAt = now;
    link.rejectHci = hci;
    addReject(out, link.conn, hci);
}

void PairingPolicy::closeWindow(const char* reason, uint32_t now) {
    collector::Event("PAIR_WINDOW").token("state", "close").token("reason", reason).absent("deadline")
        .num("dur", elapsedMs(now, _openedAt));
    _window = WindowState::Closed;
}

/** Fecha o ciclo de uma entrada: recalcula o advertising e preenche o estado corrente. */
void PairingPolicy::finish(Actions& out, bool forceAdv) {
    const AdvMode mode = advMode();
    const bool windowOpen = _window == WindowState::Open;
    out.adv = mode;
    out.windowOpen = windowOpen;
    out.hasBond = _hasBond;
    out.bondingAllowed = bondingAllowed();
    if (forceAdv || !_advPublished || mode != _pubAdv || windowOpen != _pubWindow || _hasBond != _pubBond) {
        out.setAdvertising = true;
        _advPublished = true;
        _pubAdv = mode;
        _pubWindow = windowOpen;
        _pubBond = _hasBond;
    }
}

// ── Janela ───────────────────────────────────────────────────────────────────

void PairingPolicy::begin(uint32_t, bool hasBond, Actions& out) {
    _window = WindowState::Closed;
    _hasBond = hasBond;
    _consumed = false;
    for (Link& l : _links) l = Link{};
    collector::Event("PAIR_WINDOW").token("state", "close").token("reason", "boot").absent("deadline").absent("dur");
    out.setBonding = true;
    finish(out, true);
}

void PairingPolicy::onButtonHold(uint32_t now, Actions& out) {
    for (Link& l : _links) {
        if (l.state == LinkState::Free) continue;
        if (l.state == LinkState::Authorized) l.wasAuthorized = true;
        l.state = LinkState::Rejecting;
        l.rejectAt = now;
        l.rejectHci = kHciRemoteUser;
    }
    _window = WindowState::Wiping;
    out.advertisingOffFirst = true;
    out.disconnectAll = true;
    out.deleteAllBonds = true;
    out.setBonding = true;
    finish(out);
}

void PairingPolicy::onBondsDeleted(bool ok, uint32_t now, Actions& out) {
    if (_window != WindowState::Wiping) {
        finish(out);
        return;
    }
    if (ok) {
        _window = WindowState::Open;
        _openedAt = now;
        _deadline = now + _t.windowMs;
        _consumed = false;
        _hasBond = false;
        for (Link& l : _links) {
            if (l.state == LinkState::Pending) l.admittedInWindow = true;
        }
        collector::Event("PAIR_WINDOW").token("state", "open").token("reason", "button")
            .num("deadline", _deadline).absent("dur");
    } else {
        _window = WindowState::Closed;
        out.recoverStore = true;
        collector::Event("PAIR_WINDOW").token("state", "close").token("reason", "error").absent("deadline")
            .num("dur", 0);
    }
    out.setBonding = true;
    finish(out);
}

// ── Conexões ─────────────────────────────────────────────────────────────────

void PairingPolicy::onLinkUp(uint16_t conn, uint32_t now, Actions& out) {
    const bool busy = _window == WindowState::Wiping || hasAuthorizedLink();
    Link* link = allocate(conn);
    if (busy || !link) {
        if (link) {
            link->connectedAt = now;
            reject(*link, kHciConnLimit, now, out);
        } else {
            addReject(out, conn, kHciConnLimit);  // sem slot na tabela: só recusa
        }
        emitReject(conn, "busy", kHciConnLimit, 0);
    } else {
        link->state = LinkState::Pending;
        link->connectedAt = now;
        link->admittedInWindow = _window == WindowState::Open;
    }
    finish(out);
}

void PairingPolicy::onLinkDown(uint16_t conn, uint32_t, Actions& out) {
    if (Link* link = find(conn)) {
        if (link->state == LinkState::Authorized || link->wasAuthorized) {
            out.deauthorize = true;
            out.deauthorizeConn = conn;
        }
        *link = Link{};
    }
    finish(out);
}

void PairingPolicy::onEncryptionChange(uint16_t conn, const EncryptionInfo& info, uint32_t now, Actions& out) {
    Link* link = find(conn);
    if (!link) {
        finish(out);
        return;
    }
    const uint32_t dur = static_cast<uint32_t>(elapsedMs(now, link->connectedAt));
    const bool inWindow = link->admittedInWindow;
    const bool late = inWindow && _window != WindowState::Open;
    const bool encOk = info.status == 0 && info.encrypted;

    switch (link->state) {
        case LinkState::Pending: {
            if (!encOk) {
                const StatusClass cls = classify(info.status);
                const bool pairingFailure = info.status != 0 && cls != StatusClass::Other;
                emitLinkEnc(conn, false, info, false, pairingFailure ? "pairing" : "other", dur);
                if (pairingFailure) emitPairResult(conn, false, className(cls), late, info.status, dur);
                emitReject(conn, "enc_fail", kHciAuthFailure, dur);
                reject(*link, kHciAuthFailure, now, out);
                break;
            }
            if (info.keySize < 16) {
                emitLinkEnc(conn, true, info, inWindow, "ok", dur);
                emitReject(conn, "key_size", kHciAuthFailure, dur);
                if (info.bonded) {
                    addDeleteBond(out, conn);
                    emitBondDeletePolicy();
                }
                reject(*link, kHciAuthFailure, now, out);
                break;
            }
            if (!info.bonded) {
                emitLinkEnc(conn, true, info, true, "ok", dur);
                emitPairResult(conn, false, _window == WindowState::Open ? "not_bonded" : "window_closed", late,
                               info.status, dur);
                emitReject(conn, "not_bonded", kHciAuthFailure, dur);
                reject(*link, kHciAuthFailure, now, out);
                break;
            }
            if (!info.lesc) {
                emitLinkEnc(conn, true, info, inWindow, "ok", dur);
                if (inWindow) emitPairResult(conn, false, "legacy", late, info.status, dur);
                emitReject(conn, "not_bonded", kHciAuthFailure, dur);
                addDeleteBond(out, conn);
                emitBondDeletePolicy();
                reject(*link, kHciAuthFailure, now, out);
                break;
            }
            if (!inWindow) {
                // Restauração: fora da janela nenhum bond é criado, logo este veio da LTK já guardada.
                emitLinkEnc(conn, true, info, false, "ok", dur);
                if (hasAuthorizedLink()) {
                    emitReject(conn, "busy", kHciConnLimit, dur);
                    reject(*link, kHciConnLimit, now, out);
                    break;
                }
                link->state = LinkState::Authorized;
                link->wasAuthorized = true;
                out.authorize = conn;
                break;
            }
            // Conexão admitida na janela, com bond.
            emitLinkEnc(conn, true, info, true, "ok", dur);
            if (_consumed) {
                emitPairResult(conn, false, "busy", late, info.status, dur);
                emitReject(conn, "busy", kHciConnLimit, dur);
                addDeleteBond(out, conn);
                emitBondDeletePolicy();
                reject(*link, kHciConnLimit, now, out);
                break;
            }
            if (!info.persisted) {
                collector::Event("BOND_VERIFY").num("conn", conn).flag("ok", false).flag("lesc", info.lesc)
                    .flag("persisted", false);
                emitPairResult(conn, false, "not_persisted", late, info.status, dur);
                emitReject(conn, "enc_fail", kHciAuthFailure, dur);
                addDeleteBond(out, conn);
                emitBondDeletePolicy();
                reject(*link, kHciAuthFailure, now, out);
                break;
            }
            collector::Event("BOND_VERIFY").num("conn", conn).flag("ok", true).flag("lesc", true)
                .flag("persisted", true);
            emitPairResult(conn, true, "ok", late, info.status, dur);
            link->state = LinkState::Authorized;
            link->wasAuthorized = true;
            out.authorize = conn;
            _consumed = true;
            _hasBond = true;
            if (_window == WindowState::Open) {
                closeWindow("bonded", now);
                out.setBonding = true;
            }
            break;
        }

        case LinkState::Authorized:
            if (encOk) {
                emitLinkEnc(conn, true, info, false, "ok", dur);
            } else {
                emitReject(conn, "enc_fail", kHciAuthFailure, dur);
                reject(*link, kHciAuthFailure, now, out);
            }
            break;

        case LinkState::Rejecting:
            // Pareamento que terminou depois da recusa: o bond dessa conexão não pode ficar.
            if (encOk && info.bonded && link->admittedInWindow) {
                link->admittedInWindow = false;
                addDeleteBond(out, conn);
                emitBondDeletePolicy();
            }
            break;

        case LinkState::Free:
            break;
    }
    finish(out);
}

void PairingPolicy::tick(uint32_t now, Actions& out) {
    if (_window == WindowState::Open && reached(now, _deadline)) {
        closeWindow("timeout", now);
        out.setBonding = true;
    }
    for (Link& l : _links) {
        if (l.state == LinkState::Pending) {
            const uint32_t limit = l.admittedInWindow ? _t.pairAuthTimeoutMs : _t.linkAuthTimeoutMs;
            const int32_t waited = elapsedMs(now, l.connectedAt);
            if (waited >= static_cast<int32_t>(limit)) {
                emitReject(l.conn, "auth_timeout", kHciAuthFailure, static_cast<uint32_t>(waited));
                reject(l, kHciAuthFailure, now, out);
            }
        } else if (l.state == LinkState::Rejecting &&
                   elapsedMs(now, l.rejectAt) >= static_cast<int32_t>(_t.rejectRetryMs)) {
            l.rejectAt = now;
            addReject(out, l.conn, l.rejectHci);
        }
    }
    finish(out);
}

}  // namespace pairing
