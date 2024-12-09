// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PULSE_SQLITE3_H
#define NETDATA_PULSE_SQLITE3_H

#include "daemon/common.h"

void pulse_sqlite3_query_completed(bool success, bool busy, bool locked);
void pulse_sqlite3_row_completed(void);

#if defined(PULSE_INTERNALS)
void pulse_sqlite3_do(bool extended);
#endif

#endif //NETDATA_PULSE_SQLITE3_H
