#pragma once
#include <Arduino.h>
#include "obd2/IObd2.h"

/**
 * Processador de comandos compatível com ELM327.
 *
 * Recebe um comando ASCII e retorna uma string formatada pronta
 * para ser enviada de volta ao Client.
 *
 * Supported AT commands: ATZ, ATI, ATE0/1, ATH0/1, ATL0/1, ATS0/1,
 *   ATSP[0-C], ATDP, ATPC, ATRV, ATD, ATAT[0-2], ATST[hh], AT@1
 * Supported OBD commands: mode 01/02/09 PIDs in "SSPP" hex format
 *   (spaces in the command string are stripped before parsing).
 */
class Elm327 {
public:
    explicit Elm327(IObd2* obd2);

    /**
     * Processa uma string de comando e retorna a resposta ELM327 
     * including the trailing prompt character '>'.
     */
    String process(const String& cmd);

private:
    IObd2* _obd2;
    bool   _echo;
    bool   _linefeed;
    bool   _headers;
    bool   _spaces;

    String processAt(const String& upper);
    String processObd(uint8_t service, uint8_t pid);
    String formatDataBytes(uint8_t service, uint8_t pid,
                           const uint8_t* data, int len);
    String prompt() const;
};
