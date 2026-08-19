// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NRPC_SERVING_H
#define NETDATA_NRPC_SERVING_H

#include "libnetdata/libnetdata.h"

// Every thread that registers methods brackets its registering life with
// these two calls. The handle they manage is part of every method's
// availability: once finished() runs, everything this thread registered
// becomes unavailable in every view at once (the entries stay in the
// registry until something unregisters or replaces them). started() is
// idempotent; finished() also drains in-flight cancel/progress dispatches
// aimed at this thread before it may exit.
void nrpc_serving_started(void);
void nrpc_serving_finished(void);

#endif //NETDATA_NRPC_SERVING_H
