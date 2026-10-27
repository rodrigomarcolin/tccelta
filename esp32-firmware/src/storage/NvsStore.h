#pragma once
#include <Preferences.h>
#include "collector/CollectorConfig.h"
#include "collector/SeqReserve.h"
#include "sync/SyncEngine.h"

/**
 * Dados que sobrevivem a reinicializações (NVS, namespace `collector`):
 * configuração da coleta (Tabela 6), boot_id, a reserva de `seq`, o último
 * `c_S` aceito e o pedido de wipe de bancada (spec fase-1/03 §1, §6).
 *
 * Em devsec/prod a `nvs` é cifrada pela NVS encryption sem mudança aqui; no
 * dev fica em claro (spec fase-1/01 §9.3).
 */
class NvsStore : public collector::ISeqReservePersistence, public datasync::ISyncStatePersistence {
public:
    bool begin();

    /** Config gravada, ou os padrões se não houver uma válida. */
    collector::CollectorConfig loadConfig();
    void saveConfig(const collector::CollectorConfig& config);

    /** Incrementa e devolve o boot_id deste boot; pula o 0 (0 identifica o LOSS). */
    uint16_t nextBootId();

    /** Reserva de `seq` (`seq_rsv`); 0 = ausente. */
    uint32_t loadSeqReserve();
    bool saveSeqReserve(uint32_t reserve) override;

    /** `last_seq` do formato antigo, lido uma vez na migração e apagado. */
    uint32_t loadLegacyLastSeq();
    void removeLegacyLastSeq();

    /** Wipe de bancada: remove `seq_rsv`, `last_seq`, `last_cs` e `wipe_req`. */
    void clearSeqState();
    bool wipeRequested();
    void requestWipe();

    uint32_t loadLastAcceptedCs() override;
    bool saveLastAcceptedCs(uint32_t cs) override;

private:
    Preferences _prefs;
};
