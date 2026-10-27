#include "storage/KeyStore.h"

#include <Arduino.h>
#include <esp_efuse.h>
#include <esp_efuse_table.h>
#include <esp_flash_encrypt.h>
#include <esp_mac.h>
#include <esp_partition.h>
#include <esp_secure_boot.h>
#include <nvs.h>
#include <nvs_flash.h>
#include <cstring>
#include "crypto/ConstantTime.h"
#include "security/SecurityProfile.h"

// Guardas derivadas do sdkconfig, não de build flag (spec fase-1/01 §7).
#if CONFIG_SECURE_FLASH_ENCRYPTION_MODE_RELEASE
static_assert(SECURITY_PROFILE == SECURITY_PROFILE_HMAC, "prod exige o perfil hmac");
#  ifdef SECURITY_DEV_FIXED_KEYS
#    error "chaves fixas de bancada não podem ir para o perfil prod"
#  endif
#endif

namespace {

#if CONFIG_NVS_ENCRYPTION
constexpr int kNvsEncrypted = 1;
#else
constexpr int kNvsEncrypted = 0;
#endif

constexpr const char* kPartition = "secnvs";
constexpr const char* kNamespace = "sec";

bool readBlob(nvs_handle_t handle, const char* key, uint8_t* out, size_t expected) {
    size_t size = 0;
    if (nvs_get_blob(handle, key, nullptr, &size) != ESP_OK || size != expected) return false;
    return nvs_get_blob(handle, key, out, &size) == ESP_OK && size == expected;
}

/** Inicializa a `secnvs` (cifrada em devsec/prod, em claro no dev). */
bool initPartition() {
#if CONFIG_NVS_ENCRYPTION
    const esp_partition_t* keyPart =
        esp_partition_find_first(ESP_PARTITION_TYPE_DATA, ESP_PARTITION_SUBTYPE_DATA_NVS_KEYS, "nvs_key");
    if (!keyPart) {
        Serial.println("[keys] partição nvs_key ausente");
        return false;
    }
    nvs_sec_cfg_t cfg;
    const esp_err_t keyRc = nvs_flash_read_security_cfg(keyPart, &cfg);
    if (keyRc != ESP_OK) {
        // O initArduino já gerou as chaves se a nvs_key estava em branco: aqui é erro.
        Serial.println(keyRc == ESP_ERR_NVS_CORRUPT_KEY_PART ? "[keys] nvs_key corrompida"
                                                             : "[keys] nvs_key não inicializada");
        return false;
    }
    const esp_err_t rc = nvs_flash_secure_init_partition(kPartition, &cfg);
    crypto::secureZero(&cfg, sizeof(cfg));
    return rc == ESP_OK;
#else
    return nvs_flash_init_partition(kPartition) == ESP_OK;
#endif
}

}  // namespace

bool KeyStore::load(security::DeviceKeys& out, security::KeySource& source) {
    memset(&out, 0, sizeof(out));
    source = security::KeySource::Missing;

    bool haveId = false, haveAuth = false, haveRec = false, haveSig = false;
    if (initPartition()) {
        nvs_handle_t handle;
        if (nvs_open_from_partition(kPartition, kNamespace, NVS_READONLY, &handle) == ESP_OK) {
            uint8_t version = 0;
            if (nvs_get_u8(handle, "prov_ver", &version) == ESP_OK && version == 1) {
                haveId = readBlob(handle, "dev_id", out.devId, sizeof(out.devId));
                haveAuth = readBlob(handle, "k_auth", out.kAuth, sizeof(out.kAuth));
                haveRec = readBlob(handle, "k_rec", out.kRec, sizeof(out.kRec));
                haveSig = readBlob(handle, "k_sig", out.kSig, sizeof(out.kSig));
            }
            nvs_close(handle);
        }
    }
    out.hasRec = haveRec;
    out.hasSig = haveSig;

    using security::Profile;
    bool complete = false;
    switch (security::kProfile) {
        case Profile::Hmac: complete = haveId && haveAuth && haveRec; break;
        case Profile::Asym: complete = haveId && haveAuth && haveSig; break;
        case Profile::None: complete = haveId; break;
    }
    if (complete) {
        source = security::KeySource::Partition;
        return true;
    }

#ifdef SECURITY_DEV_FIXED_KEYS
    memcpy(out.devId, security::kDevFixedKeys.devId, sizeof(out.devId));
    memcpy(out.kAuth, security::kDevFixedKeys.kAuth, sizeof(out.kAuth));
    memcpy(out.kRec, security::kDevFixedKeys.kRec, sizeof(out.kRec));
    memcpy(out.kSig, security::kDevFixedKeys.kSig, sizeof(out.kSig));
    out.hasRec = out.hasSig = true;
    source = security::KeySource::DevFixed;
    return true;
#else
    if (security::kProfile == Profile::None) {
        // DevId = 0x00 0x00 || MAC Wi-Fi (6 B): só para a linha de base do benchmark.
        out.devId[0] = out.devId[1] = 0;
        esp_read_mac(out.devId + 2, ESP_MAC_WIFI_STA);
        source = security::KeySource::MacDerived;
        return true;
    }
    Serial.println("[keys] não provisionado");
    return false;
#endif
}

KeyStore::HardwareState KeyStore::hardwareState() const {
    const esp_flash_enc_mode_t mode = esp_get_flash_encryption_mode();
    const char* fe = mode == ESP_FLASH_ENC_MODE_RELEASE       ? "release"
                     : mode == ESP_FLASH_ENC_MODE_DEVELOPMENT ? "dev"
                                                              : "off";
    return {fe, esp_secure_boot_enabled()};
}

void KeyStore::printBootLine() const {
    const HardwareState hw = hardwareState();
    uint32_t cryptCnt = 0;
    esp_efuse_read_field_blob(ESP_EFUSE_FLASH_CRYPT_CNT, &cryptCnt, 7);
    Serial.printf("[sec] fe=%s sb=%d jtag_off=%d dl_off=%d crypt_cnt=%lu nvs_enc=%d\n", hw.flashEncryption,
                  hw.secureBoot ? 1 : 0, esp_efuse_read_field_bit(ESP_EFUSE_DISABLE_JTAG) ? 1 : 0,
                  esp_efuse_read_field_bit(ESP_EFUSE_UART_DOWNLOAD_DIS) ? 1 : 0,
                  static_cast<unsigned long>(cryptCnt), kNvsEncrypted);
}
