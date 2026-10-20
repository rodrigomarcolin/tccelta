#pragma once
#include <Preferences.h>
#include "collector/CollectorConfig.h"
#include "collector/IBlockStore.h"

/**
 * Dados que sobrevivem a reinicializações (NVS): configuração da coleta
 * (Tabela 6), boot_id e o último `seq` gravado na flash.
 */
class NvsStore : public collector::ISeqPersistence {
public:
    bool begin();

    /** Config gravada, ou os padrões se não houver uma válida. */
    collector::CollectorConfig loadConfig();
    void saveConfig(const collector::CollectorConfig& config);

    /** Incrementa e devolve o boot_id deste boot. */
    uint16_t nextBootId();

    /** Último `seq` gravado (0 = nenhum). */
    uint32_t loadLastSeq();
    void saveLastSeq(uint32_t seq) override;

private:
    Preferences _prefs;
};
