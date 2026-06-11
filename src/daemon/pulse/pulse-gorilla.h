// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PULSE_GORILLA_H
#define NETDATA_PULSE_GORILLA_H

#include "daemon/common.h"

#if defined(PULSE_INTERNALS)
void pulse_gorilla_do(bool extended);
#endif

#endif //NETDATA_PULSE_GORILLA_H
