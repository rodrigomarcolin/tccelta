#pragma once
#include <cstddef>
#include <cstdint>

// Perfil de segurança do build (spec fase-1/01 §1). Só a tag do bloco muda;
// a tag do handshake tem sempre 16 B.
#define SECURITY_PROFILE_NONE 0
#define SECURITY_PROFILE_HMAC 1
#define SECURITY_PROFILE_ASYM 2
#ifndef SECURITY_PROFILE
#define SECURITY_PROFILE SECURITY_PROFILE_HMAC
#endif

namespace security {

enum class Profile : uint8_t { None = 0, Hmac = 1, Asym = 2 };
constexpr Profile kProfile = static_cast<Profile>(SECURITY_PROFILE);
constexpr size_t kHandshakeTagSize = 16;  // fixo (Tabela 2)
constexpr size_t kBlockTagSize = kProfile == Profile::None ? 0 : kProfile == Profile::Hmac ? 32 : 64;
constexpr const char* kProfileName =
    kProfile == Profile::None ? "none" : kProfile == Profile::Hmac ? "hmac" : "asym";
static_assert(SECURITY_PROFILE >= 0 && SECURITY_PROFILE <= 2, "SECURITY_PROFILE inválido");

}  // namespace security
