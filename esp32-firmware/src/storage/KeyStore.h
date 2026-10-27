#pragma once
#include "security/DeviceKeys.h"

/**
 * Chaves do dongle na partição `secnvs` (namespace `sec`; spec fase-1/01 §7).
 * Em devsec/prod a NVS encryption cifra a partição (`CONFIG_NVS_ENCRYPTION`);
 * no dev ela fica em claro. Esse `#if` é o único ponto que varia entre os
 * perfis de hardware. Usa a API C da NVS: nenhum `Preferences::begin(…, "secnvs")`,
 * que abriria a partição em claro.
 */
class KeyStore {
public:
    /**
     * Lê a `secnvs`; cai para as chaves fixas de bancada (SECURITY_DEV_FIXED_KEYS)
     * ou, no perfil `none`, para um DevId derivado do MAC. false = faltam chaves
     * exigidas pelo perfil.
     */
    bool load(security::DeviceKeys& out, security::KeySource& source);

    /** Linha de boot `[sec] fe=… sb=… jtag_off=… dl_off=… crypt_cnt=… nvs_enc=…` e estado para o evento KEYS. */
    struct HardwareState {
        const char* flashEncryption;  // off | dev | release
        bool secureBoot;
    };
    HardwareState hardwareState() const;
    void printBootLine() const;
};
