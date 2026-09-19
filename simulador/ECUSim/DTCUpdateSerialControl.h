#ifndef DTCUPDATE_SERIALCONTROL_H
#define DTCUPDATE_SERIALCONTROL_H

#include "ECUSim.h"

bool isDtcControlMessage(char firstChar);
void parseDTCUpdateMessage();

#endif
