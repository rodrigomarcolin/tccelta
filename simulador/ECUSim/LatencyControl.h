#ifndef LATENCY_CONTROL_H
#define LATENCY_CONTROL_H

#include "ECUSim.h"

// Canal serial de latência: 'Y' + modo + PID + baseline + A + B + LF (ver README).
bool isLatencyControlMessage(char firstChar);
void parseLatencyUpdateMessage();
void initializeLatency();

// Latência (ms) a aplicar à resposta de uma requisição OBD.
// `request` aponta para o SID; `length` = SID + dados. Em Mode 01 com vários PIDs
// retorna o máximo das latências sorteadas.
uint16_t computeResponseLatencyMs(const byte* request, uint8_t length);

#endif
