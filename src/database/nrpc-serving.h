// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NRPC_SERVING_H
#define NETDATA_NRPC_SERVING_H

#include "libnetdata/libnetdata.h"

// ----------------------------------------------------------------------------
// public API

void nrpc_serving_started(void);
void nrpc_serving_finished(void);

#endif //NETDATA_NRPC_SERVING_H
