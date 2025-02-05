// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PULSE_DB_DBENGINE_H
#define NETDATA_PULSE_DB_DBENGINE_H

#include "daemon/common.h"

#if defined(PULSE_INTERNALS)
extern int64_t pulse_dbengine_total_memory;

#if defined(ENABLE_DBENGINE)
void pulse_dbengine_do(bool extended);
#endif

#endif

#endif //NETDATA_PULSE_DB_DBENGINE_H
