#pragma once
#include <sdkconfig.h>

/**
 * Enlace BLE aberto, só para bancada (`-DLINK_SECURITY_OPEN`, env `collector_mock_debug_open`).
 *
 * Desliga a exigência de LESC/bond: qualquer central conecta, é autorizada na hora e escreve no NUS e no
 * SYNC (incluindo AT#WIPE). Existe para o `tools/sync-bench/sync_bench.py`, que não pareia. O perfil de
 * segurança do protocolo (tags, handshake, K_auth/K_rec) não muda. Sem o flag, nada disto tem efeito.
 */
#if defined(LINK_SECURITY_OPEN)
#if !defined(COLLECTOR_EVENTS)
#error "LINK_SECURITY_OPEN exige COLLECTOR_EVENTS: o enlace aberto precisa ser visível na serial"
#endif
#if defined(CONFIG_SECURE_BOOT) || defined(CONFIG_SECURE_FLASH_ENC_ENABLED)
#error "LINK_SECURITY_OPEN nunca junto de Secure Boot ou flash encryption (builds devsec/prod)"
#endif
#endif

namespace ble_security {

#if defined(LINK_SECURITY_OPEN)
constexpr bool kLinkOpen = true;
#else
constexpr bool kLinkOpen = false;
#endif

}  // namespace ble_security
