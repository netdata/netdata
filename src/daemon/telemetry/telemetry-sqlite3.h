// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_TELEMETRY_SQLITE3_H
#define NETDATA_TELEMETRY_SQLITE3_H

#include "daemon/common.h"

void telemetry_sqlite3_query_completed(bool success, bool busy, bool locked);
void telemetry_sqlite3_row_completed(void);

#if defined(TELEMETRY_INTERNALS)
void telemetry_sqlite3_do(bool extended);
#endif

#endif //NETDATA_TELEMETRY_SQLITE3_H
