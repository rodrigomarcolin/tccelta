#pragma once
#include <cstdint>
#include <NimBLEDevice.h>
#include "connectivity/ble/LinkSecurityMode.h"

/**
 * Segurança de enlace LESC Just Works sobre a NimBLE (spec fase-2/01): configuração
 * do Security Manager, gate de criação de bond, guarda do armazenamento de bonds
 * e leitura do bond no store. A política de quem pode se conectar mora em
 * `pairing::PairingPolicy`; aqui ficam só as chamadas à pilha.
 */
namespace ble_security {

#if defined(LINK_SECURITY_OPEN)
// Enlace aberto (só bancada, ver LinkSecurityMode.h): sem exigência de cifra.
constexpr uint32_t kSecureRxProps = NIMBLE_PROPERTY::WRITE | NIMBLE_PROPERTY::WRITE_NR;
constexpr uint32_t kSecureTxProps = NIMBLE_PROPERTY::NOTIFY;
#else
/** Escrita app -> dongle: com e sem resposta, só em enlace cifrado. */
constexpr uint32_t kSecureRxProps = NIMBLE_PROPERTY::WRITE | NIMBLE_PROPERTY::WRITE_NR | NIMBLE_PROPERTY::WRITE_ENC;
/**
 * Notificação dongle -> app. A proteção real está no CCCD (perm flags) e no gate
 * de aplicação; READ_ENC (sem READ) só documenta a exigência.
 */
constexpr uint32_t kSecureTxProps = NIMBLE_PROPERTY::NOTIFY | NIMBLE_PROPERTY::READ_ENC;
#endif

/** Dados de fabricante do anúncio (spec fase-2/01 §5.5). */
constexpr uint16_t kCompanyId = 0xFFFF;
constexpr uint8_t  kAdvFormatVersion = 0x01;
/** Intervalos de anúncio em unidades de 0,625 ms: 211,25 ms (pareamento) e 546,25 ms (normal). */
constexpr uint16_t kAdvItvlPairing = 338;
constexpr uint16_t kAdvItvlNormal = 874;

/** Recusa pareamento fora da janela e nunca despeja o bond legítimo (spec fase-2/01 §5.6). */
class BondStoreGuard : public NimBLEDeviceCallbacks {
public:
    int onStoreStatus(ble_store_status_event* event, void* arg) override;
};

struct BondCheck {
    bool found;      // há registro de peer_sec para o endereço
    bool lesc;       // sc == 1
    bool persisted;  // ltk_present
};

class BleSecurity {
public:
    /** Chamar logo após NimBLEDevice::init() e antes de criar o servidor ou anunciar. */
    static void configure();
    /** Liga/desliga a criação de bond (ble_hs_cfg.sm_bonding), sob seção crítica. */
    static void setBondingAllowed(bool allowed);
    static bool bondingAllowed();
    /** Quantos bonds há no store (NimBLEDevice::getNumBonds). */
    static int bondCount();
    /** Lê o bond do peer no store (ble_store_read_peer_sec). */
    static BondCheck checkBond(const NimBLEAddress& idAddress);
    /** Apaga todos os bonds; devolve ok e o tempo gasto. Não derruba conexões. */
    static bool deleteAllBonds(uint32_t& durMs);
    /** Recuperação: apaga o namespace nimble_bond inteiro e reinicia o ESP32. Não retorna. */
    [[noreturn]] static void eraseBondStoreAndRestart();
    /** Mais de um bond, ou bond legado, no boot => apaga todos. Emite NVS. */
    static void sanitizeBondsAtBoot();
    /** Status do último ENC_CHANGE desta conexão (0 = ok). */
    static int lastEncStatus(uint16_t connHandle);
    /** Esquece o status guardado desta conexão (ao cair). */
    static void clearEncStatus(uint16_t connHandle);
    /** Monta os 4 bytes de dados de fabricante: FF FF ver flags. */
    static void manufacturerData(bool windowOpen, bool hasBond, uint8_t out[4]);

private:
    static int gapListener(ble_gap_event* event, void* arg);
};

}  // namespace ble_security
