// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_STATIC_THREADS_H
#define NETDATA_STATIC_THREADS_H

#include "common.h"

extern const struct netdata_static_thread static_threads_common[];

struct netdata_static_thread *
static_threads_concat(const struct netdata_static_thread *lhs,
                      const struct netdata_static_thread *rhs);

struct netdata_static_thread *static_threads_get();

#endif /* NETDATA_STATIC_THREADS_H */
