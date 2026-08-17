// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NRPC_CALLS_H
#define NETDATA_NRPC_CALLS_H

#include "rrd.h"

// the call_id broker: create at startup; destroy exists only for the
// ASAN-gated shutdown path (see nrpc_inflight_calls_destroy)
void nrpc_inflight_calls_create(void);
void nrpc_inflight_calls_destroy(void);

// Broker-keyed deadline accessor: looks the call_id up in the broker,
// atomically reads its deadline, releases. Returns false when the call_id
// is not (or no longer) in flight. This is how the transports read deadlines -
// they must NOT hold pointers into broker records.
bool nrpc_call_deadline(const char *call_id, usec_t *out_stop_monotonic_ut);

// cancel a running function, to be run from anywhere
void nrpc_call_cancel(const char *call_id);

void nrpc_call_progress(const char *call_id);
void nrpc_call_request_progress(nd_uuid_t *call_id);

#endif //NETDATA_NRPC_CALLS_H
