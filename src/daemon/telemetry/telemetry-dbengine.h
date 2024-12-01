// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_TELEMETRY_DBENGINE_H
#define NETDATA_TELEMETRY_DBENGINE_H

#include "daemon/common.h"

#if defined(TELEMETRY_INTERNALS)
extern size_t telemetry_dbengine_total_memory;

#if defined(ENABLE_DBENGINE)
void telemetry_dbengine_do(bool extended);
#endif

#endif

#endif //NETDATA_TELEMETRY_DBENGINE_H
