// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_APPS_LOOKUP_NETIPC_H
#define NETDATA_APPS_LOOKUP_NETIPC_H 1

#include "apps_plugin.h"

#ifdef OS_LINUX

#define APPS_NETIPC_LOOKUP_SERVICE_NAME "apps-lookup"
#define APPS_NETIPC_LOOKUP_WORKER_COUNT 2
#define APPS_LOOKUP_MAX_PIDS_PER_REQUEST 8192U

void apps_lookup_netipc_init(void);
void apps_lookup_netipc_cleanup(void);
void apps_lookup_netipc_send_charts_to_netdata(usec_t dt);

#else

static inline void apps_lookup_netipc_init(void) {}
static inline void apps_lookup_netipc_cleanup(void) {}
static inline void apps_lookup_netipc_send_charts_to_netdata(usec_t dt __maybe_unused) {}

#endif

#endif /* NETDATA_APPS_LOOKUP_NETIPC_H */
