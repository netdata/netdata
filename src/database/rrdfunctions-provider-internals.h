// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_RRDFUNCTIONS_PROVIDER_INTERNALS_H
#define NETDATA_RRDFUNCTIONS_PROVIDER_INTERNALS_H

#include "rrd.h"

struct rrd_function_provider;
struct rrd_function_provider *rrd_function_provider_acquire_current_thread(void);
void rrd_function_provider_release(struct rrd_function_provider *rdc);
extern __thread struct rrd_function_provider *thread_rrd_function_provider;
bool rrd_function_provider_running(struct rrd_function_provider *rdc);
pid_t rrd_function_provider_tid(struct rrd_function_provider *rdc);
bool rrd_function_provider_dispatcher_acquire(struct rrd_function_provider *rdc);
void rrd_function_provider_dispatcher_release(struct rrd_function_provider *rdc);

#endif //NETDATA_RRDFUNCTIONS_PROVIDER_INTERNALS_H
