// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NRPC_BUILTIN_H
#define NETDATA_NRPC_BUILTIN_H

#include "rrd.h"

typedef int (*nrpc_builtin_handler_cb_t)(BUFFER *wb, const char *function, BUFFER *payload, const char *source);

void nrpc_method_register_builtin(RRDHOST *host, RRDSET *st, const char *name, int timeout, int priority, uint32_t version,
                             const char *help, const char *tags,
                             HTTP_ACCESS access, nrpc_builtin_handler_cb_t handler);

#endif //NETDATA_NRPC_BUILTIN_H
