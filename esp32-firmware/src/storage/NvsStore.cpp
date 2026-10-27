#include "storage/NvsStore.h"

namespace {

constexpr const char* kNamespace         = "collector";
constexpr const char* kKeyConfig         = "config";
constexpr const char* kKeyConfigVer      = "config_ver";
constexpr const char* kKeyBootId         = "boot_id";
constexpr const char* kKeySeqReserve     = "seq_rsv";
constexpr const char* kKeyLastCs         = "last_cs";
constexpr const char* kKeyLegacyLastSeq  = "last_seq";
constexpr const char* kKeyWipeRequest    = "wipe_req";

// Muda sempre que o layout de CollectorConfig mudar, para não ler lixo.
constexpr uint8_t kConfigVersion = 2;  // 2: + flushIntervalMs

}  // namespace

bool NvsStore::begin() { return _prefs.begin(kNamespace, false); }

collector::CollectorConfig NvsStore::loadConfig() {
    collector::CollectorConfig config;
    const bool stored = _prefs.getUChar(kKeyConfigVer, 0) == kConfigVersion &&
                        _prefs.getBytes(kKeyConfig, &config, sizeof(config)) == sizeof(config);
    if (!stored || collector::validate(config) != collector::ConfigError::None) {
        return collector::CollectorConfig::defaults();
    }
    return config;
}

void NvsStore::saveConfig(const collector::CollectorConfig& config) {
    _prefs.putBytes(kKeyConfig, &config, sizeof(config));
    _prefs.putUChar(kKeyConfigVer, kConfigVersion);
}

uint16_t NvsStore::nextBootId() {
    uint16_t bootId = static_cast<uint16_t>(_prefs.getUShort(kKeyBootId, 0) + 1);
    if (bootId == 0) bootId = 1;  // boot_id = 0 identifica o LOSS
    _prefs.putUShort(kKeyBootId, bootId);
    return bootId;
}

uint32_t NvsStore::loadSeqReserve() { return _prefs.getUInt(kKeySeqReserve, 0); }

bool NvsStore::saveSeqReserve(uint32_t reserve) {
    return _prefs.putUInt(kKeySeqReserve, reserve) == sizeof(uint32_t);  // putUInt já faz commit
}

uint32_t NvsStore::loadLegacyLastSeq() { return _prefs.getUInt(kKeyLegacyLastSeq, 0); }

void NvsStore::removeLegacyLastSeq() { _prefs.remove(kKeyLegacyLastSeq); }

void NvsStore::clearSeqState() {
    _prefs.remove(kKeySeqReserve);
    _prefs.remove(kKeyLegacyLastSeq);
    _prefs.remove(kKeyLastCs);
    _prefs.remove(kKeyWipeRequest);
}

bool NvsStore::wipeRequested() { return _prefs.getUChar(kKeyWipeRequest, 0) != 0; }

void NvsStore::requestWipe() { _prefs.putUChar(kKeyWipeRequest, 1); }

uint32_t NvsStore::loadLastAcceptedCs() { return _prefs.getUInt(kKeyLastCs, 0); }

bool NvsStore::saveLastAcceptedCs(uint32_t cs) { return _prefs.putUInt(kKeyLastCs, cs) == sizeof(uint32_t); }
