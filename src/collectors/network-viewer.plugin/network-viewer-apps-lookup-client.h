// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_NETWORK_VIEWER_APPS_LOOKUP_CLIENT_H
#define NETDATA_NETWORK_VIEWER_APPS_LOOKUP_CLIENT_H

#include "libnetdata/libnetdata.h"

typedef struct {
    // cppcheck-suppress unusedStructMember
    char *key;
    // cppcheck-suppress unusedStructMember
    char *value;
} NV_APPS_LOOKUP_LABEL;

typedef struct {
    // cppcheck-suppress unusedStructMember
    uint16_t cgroup_status;
    // cppcheck-suppress unusedStructMember
    uint16_t orchestrator;
    // cppcheck-suppress unusedStructMember
    uint64_t starttime;
    // cppcheck-suppress unusedStructMember
    uint64_t apps_lookup_generation_observed;
    // cppcheck-suppress unusedStructMember
    char *cgroup_path;
    // cppcheck-suppress unusedStructMember
    char *cgroup_name;
    // cppcheck-suppress unusedStructMember
    NV_APPS_LOOKUP_LABEL *cgroup_labels;
    // cppcheck-suppress unusedStructMember
    uint16_t cgroup_label_count;
} NV_APPS_LOOKUP_FIELDS;

#if defined(OS_LINUX)

void nv_apps_lookup_init(bool *plugin_should_exit);
void nv_apps_lookup_start(void);
void nv_apps_lookup_stop(void);
bool nv_apps_lookup_worker_exited(void);
void nv_apps_lookup_warm_pids(const uint32_t *pids, size_t pid_count);
bool nv_cache_lookup_pid(uint32_t pid, uint64_t expected_starttime, NV_APPS_LOOKUP_FIELDS *out);
void nv_cache_lookup_fields_free(NV_APPS_LOOKUP_FIELDS *fields);
void nv_apps_lookup_send_charts_to_netdata(usec_t dt);

#else

// APPS_LOOKUP enrichment needs apps.plugin cgroup data; the client is compiled
// on Linux only, so other platforms get no-op stubs to keep network-viewer.c portable.

static inline void nv_apps_lookup_init(bool *plugin_should_exit __maybe_unused) {}
static inline void nv_apps_lookup_start(void) {}
static inline void nv_apps_lookup_stop(void) {}
static inline bool nv_apps_lookup_worker_exited(void) { return false; }
static inline void nv_apps_lookup_warm_pids(const uint32_t *pids __maybe_unused, size_t pid_count __maybe_unused) {}
static inline bool nv_cache_lookup_pid(
    uint32_t pid __maybe_unused,
    uint64_t expected_starttime __maybe_unused,
    NV_APPS_LOOKUP_FIELDS *out __maybe_unused)
{
    return false;
}
static inline void nv_cache_lookup_fields_free(NV_APPS_LOOKUP_FIELDS *fields __maybe_unused) {}
static inline void nv_apps_lookup_send_charts_to_netdata(usec_t dt __maybe_unused) {}

#endif

#endif /* NETDATA_NETWORK_VIEWER_APPS_LOOKUP_CLIENT_H */
