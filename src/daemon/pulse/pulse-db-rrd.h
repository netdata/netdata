// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PULSE_DB_RRD_H
#define NETDATA_PULSE_DB_RRD_H

#include "libnetdata/libnetdata.h"

void pulse_db_rrd_memory_change(int64_t value);
void pulse_db_rrd_memory_add(uint64_t value);
void pulse_db_rrd_memory_sub(uint64_t value);

#if defined(PULSE_INTERNALS)
extern int64_t pulse_rrd_memory_size;
#endif

#endif //NETDATA_PULSE_DB_RRD_H
