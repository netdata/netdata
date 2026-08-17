// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDFUNCTIONS_INFLIGHT_H
#define NETDATA_RRDFUNCTIONS_INFLIGHT_H

#include "rrd.h"

// the transaction broker: create at startup; destroy exists only for the
// ASAN-gated shutdown path (see rrd_function_transactions_destroy)
void rrd_function_transactions_create(void);
void rrd_function_transactions_destroy(void);

// cancel a running function, to be run from anywhere
void rrd_function_cancel(const char *transaction);

void rrd_function_progress(const char *transaction);
void rrd_function_call_progresser(nd_uuid_t *transaction);

#endif //NETDATA_RRDFUNCTIONS_INFLIGHT_H
