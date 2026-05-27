// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NETWORK_VIEWER_APPS_LOOKUP_CLIENT_H
#define NETDATA_NETWORK_VIEWER_APPS_LOOKUP_CLIENT_H

#include "libnetdata/libnetdata.h"

#if defined(OS_LINUX)

void nv_apps_lookup_init(bool *plugin_should_exit);
void nv_apps_lookup_start(void);
void nv_apps_lookup_stop(void);
bool nv_apps_lookup_worker_exited(void);
void nv_apps_lookup_warm_pids(const uint32_t *pids, size_t pid_count);
void nv_apps_lookup_send_charts_to_netdata(usec_t dt);

#else

// APPS_LOOKUP enrichment needs apps.plugin cgroup data; the client is compiled
// on Linux only, so other platforms get no-op stubs to keep network-viewer.c portable.

static inline void nv_apps_lookup_init(bool *plugin_should_exit __maybe_unused) {}
static inline void nv_apps_lookup_start(void) {}
static inline void nv_apps_lookup_stop(void) {}
static inline bool nv_apps_lookup_worker_exited(void) { return false; }
static inline void nv_apps_lookup_warm_pids(const uint32_t *pids __maybe_unused, size_t pid_count __maybe_unused) {}
static inline void nv_apps_lookup_send_charts_to_netdata(usec_t dt __maybe_unused) {}

#endif

#endif /* NETDATA_NETWORK_VIEWER_APPS_LOOKUP_CLIENT_H */
