#pragma once
#include <cstdint>

namespace collector {

/** Comandos AT# do projeto (Tabela 8 + AT#TMAX). */
struct AtHashCommand {
    enum class Kind : uint8_t {
        Invalid,
        SetPid,      // AT#PID pp mmmm
        DeletePid,   // AT#DEL pp
        List,        // AT#LIST
        Diag,        // AT#DIAG b
        Net,         // AT#NET b
        Gps,         // AT#GPS lat lon alt
        Tmax,        // AT#TMAX
        TmaxQuery,   // AT#TMAX?
        Ack,         // AT#ACK n (só nas builds de depuração: simula o ACK do servidor)
    };

    Kind     kind = Kind::Invalid;
    uint8_t  pid = 0;
    uint32_t periodMs = 0;
    uint32_t seq = 0;
    bool     flag = false;
    int32_t  latE7 = 0;
    int32_t  lonE7 = 0;
    int32_t  altMm = 0;
};

/**
 * Interpreta um comando já em maiúsculas e sem CR/LF, começando por "AT#".
 * Argumentos separados por espaços; PID em hexadecimal (2 dígitos), demais
 * números em decimal.
 */
AtHashCommand parseAtHash(const char* text);

}  // namespace collector
