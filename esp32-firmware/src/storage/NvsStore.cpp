#include "storage/NvsStore.h"

namespace {

constexpr const char* kNamespace     = "collector";
constexpr const char* kKeyConfig     = "config";
constexpr const char* kKeyConfigVer  = "config_ver";
constexpr const char* kKeyBootId     = "boot_id";
constexpr const char* kKeyLastSeq    = "last_seq";

// Muda sempre que o layout de CollectorConfig mudar, para não ler lixo.
constexpr uint8_t kConfigVersion = 1;

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
    const uint16_t bootId = static_cast<uint16_t>(_prefs.getUShort(kKeyBootId, 0) + 1);
    _prefs.putUShort(kKeyBootId, bootId);
    return bootId;
}

uint32_t NvsStore::loadLastSeq() { return _prefs.getUInt(kKeyLastSeq, 0); }

void NvsStore::saveLastSeq(uint32_t seq) { _prefs.putUInt(kKeyLastSeq, seq); }
