// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NRPC_CALLS_H
#define NETDATA_NRPC_CALLS_H

#include "rrd.h"

// the transaction broker: create at startup; destroy exists only for the
// ASAN-gated shutdown path (see rrd_function_transactions_destroy)
void rrd_function_transactions_create(void);
void rrd_function_transactions_destroy(void);

// Broker-keyed deadline accessor: looks the transaction up in the broker,
// atomically reads its deadline, releases. Returns false when the transaction
// is not (or no longer) in flight. This is how the transports read deadlines -
// they must NOT hold pointers into broker records.
bool rrd_function_transaction_deadline(const char *transaction, usec_t *out_stop_monotonic_ut);

// cancel a running function, to be run from anywhere
void rrd_function_cancel(const char *transaction);

void rrd_function_progress(const char *transaction);
void rrd_function_call_progresser(nd_uuid_t *transaction);

#endif //NETDATA_NRPC_CALLS_H
