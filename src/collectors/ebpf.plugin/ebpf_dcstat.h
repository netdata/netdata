// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_DCSTAT_H
#define NETDATA_EBPF_DCSTAT_H 1

#include "ebpf.h"

// Module name & description
#define NETDATA_EBPF_MODULE_NAME_DCSTAT "dcstat"
#define NETDATA_EBPF_DC_MODULE_DESC                                                                                    \
    "Monitor file access using directory cache. This thread is integrated with apps and cgroup."

// charts
#define NETDATA_DC_HIT_CHART "dc_hit_ratio"
#define NETDATA_DC_REFERENCE_CHART "dc_reference"
#define NETDATA_DC_REQUEST_NOT_CACHE_CHART "dc_not_cache"
#define NETDATA_DC_REQUEST_NOT_FOUND_CHART "dc_not_found"

#define NETDATA_DIRECTORY_CACHE_SUBMENU "directory cache"

// configuration file
#define NETDATA_DIRECTORY_DCSTAT_CONFIG_FILE "dcstat.conf"

// Contexts
#define NETDATA_FS_DC_HIT_RATIO_CONTEXT "filesystem.dc_hit_ratio"
#define NETDATA_FS_DC_REFERENCE_CONTEXT "filesystem.dc_reference"

#define NETDATA_CGROUP_DC_HIT_RATIO_CONTEXT "cgroup.dc_ratio"
#define NETDATA_CGROUP_DC_REFERENCE_CONTEXT "cgroup.dc_reference"
#define NETDATA_CGROUP_DC_NOT_CACHE_CONTEXT "cgroup.dc_not_cache"
#define NETDATA_CGROUP_DC_NOT_FOUND_CONTEXT "cgroup.dc_not_found"

#define NETDATA_SYSTEMD_DC_HIT_RATIO_CONTEXT "systemd.service.dc_ratio"
#define NETDATA_SYSTEMD_DC_REFERENCE_CONTEXT "systemd.service.dc_reference"
#define NETDATA_SYSTEMD_DC_NOT_CACHE_CONTEXT "systemd.service.dc_not_cache"
#define NETDATA_SYSTEMD_DC_NOT_FOUND_CONTEXT "systemd.service.dc_not_found"

// ARAL name
#define NETDATA_EBPF_DCSTAT_ARAL_NAME "ebpf_dcstat"

// Unity
#define EBPF_COMMON_UNITS_FILES "files"

enum directory_cache_indexes {
    NETDATA_DCSTAT_IDX_RATIO,
    NETDATA_DCSTAT_IDX_REFERENCE,
    NETDATA_DCSTAT_IDX_SLOW,
    NETDATA_DCSTAT_IDX_MISS,

    // Keep this as last and don't skip numbers as it is used as element counter
    NETDATA_DCSTAT_IDX_END
};

enum directory_cache_tables { NETDATA_DCSTAT_GLOBAL_STATS, NETDATA_DCSTAT_PID_STATS, NETDATA_DCSTAT_CTRL };

// variables
enum directory_cache_counters {
    NETDATA_KEY_DC_REFERENCE,
    NETDATA_KEY_DC_SLOW,
    NETDATA_KEY_DC_MISS,

    // Keep this as last and don't skip numbers as it is used as element counter
    NETDATA_DIRECTORY_CACHE_END
};

enum directory_cache_targets { NETDATA_DC_TARGET_LOOKUP_FAST, NETDATA_DC_TARGET_D_LOOKUP };

typedef struct __attribute__((packed)) netdata_publish_dcstat_pid {
    uint64_t cache_access;
    uint32_t file_system;
    uint32_t not_found;
} netdata_publish_dcstat_pid_t;

typedef struct __attribute__((packed)) netdata_publish_dcstat {
    uint64_t ct;

    long long ratio;
    long long cache_access;

    netdata_publish_dcstat_pid_t curr;
    netdata_publish_dcstat_pid_t prev;
} netdata_publish_dcstat_t;

void *ebpf_dcstat_thread(void *ptr);
void ebpf_dcstat_create_apps_charts(struct ebpf_module *em, void *ptr);
void ebpf_dcstat_release(netdata_publish_dcstat_t *stat);
extern struct config dcstat_config;
extern netdata_ebpf_targets_t dc_targets[];
extern ebpf_local_maps_t dcstat_maps[];

#endif // NETDATA_EBPF_DCSTAT_H
