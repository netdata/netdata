// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_APPS_CGROUPS_LOOKUP_CLIENT_H
#define NETDATA_APPS_CGROUPS_LOOKUP_CLIENT_H 1

#include "apps_plugin.h"
#include "libnetdata/netipc/netipc_netdata.h"

#ifdef OS_LINUX

#define APPS_CGROUPS_LOOKUP_QUEUE_MAX 4096U
#define APPS_CGROUPS_LOOKUP_CACHE_MAX 4096U

struct cgroup_lookup_label {
    STRING *key;
    STRING *value;
};

struct cgroup_lookup_entry {
    STRING *key;
    uint16_t cgroup_status;
    uint16_t orchestrator;
    STRING *cgroup_name;
    struct cgroup_lookup_label *cgroup_labels;
    uint16_t cgroup_label_count;
    uint64_t generation;
    uint64_t last_used_iteration;
    uint32_t refcount;
    bool pending;
};

void apps_cgroups_lookup_init(void);
void apps_cgroups_lookup_cleanup(void);
void apps_cgroups_lookup_scan_pids(void);
void apps_cgroups_lookup_unlink_pid(struct pid_stat *p);
void apps_cgroups_lookup_set_pid_cgroup_path(struct pid_stat *p, const char *path);
bool apps_cgroups_lookup_is_host_root_path(const char *path);
void apps_cgroups_lookup_send_charts_to_netdata(usec_t dt);

#else

static inline void apps_cgroups_lookup_init(void) {}
static inline void apps_cgroups_lookup_cleanup(void) {}
static inline void apps_cgroups_lookup_scan_pids(void) {}
static inline void apps_cgroups_lookup_unlink_pid(struct pid_stat *p __maybe_unused) {}
static inline void apps_cgroups_lookup_set_pid_cgroup_path(struct pid_stat *p __maybe_unused, const char *path __maybe_unused) {}
static inline bool apps_cgroups_lookup_is_host_root_path(const char *path __maybe_unused) { return false; }
static inline void apps_cgroups_lookup_send_charts_to_netdata(usec_t dt __maybe_unused) {}

#endif

#endif /* NETDATA_APPS_CGROUPS_LOOKUP_CLIENT_H */
