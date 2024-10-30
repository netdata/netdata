// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDFUNCTIONS_INLINE_H
#define NETDATA_RRDFUNCTIONS_INLINE_H

#include "rrd.h"

typedef int (*rrd_function_execute_inline_cb_t)(BUFFER *wb, const char *function, BUFFER *payload, const char *source);

void rrd_function_add_inline(RRDHOST *host, RRDSET *st, const char *name, int timeout, int priority, uint32_t version,
                             const char *help, const char *tags,
                             HTTP_ACCESS access, rrd_function_execute_inline_cb_t execute_cb);

#endif //NETDATA_RRDFUNCTIONS_INLINE_H
