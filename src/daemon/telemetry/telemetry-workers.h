// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_TELEMETRY_WORKERS_H
#define NETDATA_TELEMETRY_WORKERS_H

#include "daemon/common.h"

#if defined(TELEMETRY_INTERNALS)
void telemetry_workers_do(bool extended);
void telemetry_workers_cleanup(void);
#endif

#endif //NETDATA_TELEMETRY_WORKERS_H
