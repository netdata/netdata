// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_TELEMETRY_ARAL_H
#define NETDATA_TELEMETRY_ARAL_H

#include "daemon/common.h"

void telemetry_aral_register(ARAL *ar, const char *name);
void telemetry_aral_unregister(ARAL *ar);

#if defined(TELEMETRY_INTERNALS)
void telemerty_aral_init(void);
void telemetry_aral_do(bool extended);
#endif

#endif //NETDATA_TELEMETRY_ARAL_H
