// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NRPC_SERVING_H
#define NETDATA_NRPC_SERVING_H

#include "libnetdata/libnetdata.h"

// ----------------------------------------------------------------------------
// public API

void rrd_function_provider_started(void);
void rrd_function_provider_finished(void);

#endif //NETDATA_NRPC_SERVING_H
