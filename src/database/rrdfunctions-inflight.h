// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDFUNCTIONS_INFLIGHT_H
#define NETDATA_RRDFUNCTIONS_INFLIGHT_H

#include "rrd.h"

void rrd_functions_inflight_init(void);

// cancel a running function, to be run from anywhere
void rrd_function_cancel(const char *transaction);

void rrd_function_progress(const char *transaction);
void rrd_function_call_progresser(nd_uuid_t *transaction);

#endif //NETDATA_RRDFUNCTIONS_INFLIGHT_H
