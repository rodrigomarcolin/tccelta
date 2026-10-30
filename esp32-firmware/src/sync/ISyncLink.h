#pragma once
#include <cstddef>
#include <cstdint>

namespace datasync {

struct SendStats {
    uint16_t fragments = 0;
    uint16_t mtu = 0;
    uint32_t durUs = 0;
    uint16_t retries = 0;
    bool ok = false;
};

/** Saída de mensagens TLV do SyncEngine. Só a SyncTask chama `send()` (spec fase-1/04 §7). */
class ISyncLink {
public:
    virtual ~ISyncLink() = default;
    /** Fragmenta e notifica; false se faltou crédito, houve desconexão ou o app não assina. */
    virtual bool send(const uint8_t* tlv, size_t len, SendStats& stats) = 0;
    virtual bool ready() const = 0;  // conectado e com notificação habilitada
    virtual uint16_t mtu() const = 0;
};

}  // namespace datasync
