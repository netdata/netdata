// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_DCSTAT_H
#define NETDATA_EBPF_DCSTAT_H 1


// charts
#define NETDATA_DC_HIT_CHART "dc_hit_ratio"
#define NETDATA_DC_REFERENCE_CHART "dc_reference"
#define NETDATA_DC_REQUEST_NOT_CACHE_CHART "dc_not_cache"
#define NETDATA_DC_REQUEST_NOT_FOUND_CHART "dc_not_found"

#define NETDATA_DIRECTORY_CACHE_SUBMENU "directory cache (eBPF)"
#define NETDATA_DIRECTORY_FILESYSTEM_SUBMENU "Directory Cache (eBPF)"

// configuration file
#define NETDATA_DIRECTORY_DCSTAT_CONFIG_FILE "dcstat.conf"

#define NETDATA_LATENCY_DCSTAT_SLEEP_MS 700000ULL

enum directory_cache_indexes {
    NETDATA_DCSTAT_IDX_RATIO,
    NETDATA_DCSTAT_IDX_REFERENCE,
    NETDATA_DCSTAT_IDX_SLOW,
    NETDATA_DCSTAT_IDX_MISS,

    // Keep this as last and don't skip numbers as it is used as element counter
    NETDATA_DCSTAT_IDX_END
};

enum directory_cache_tables {
    NETDATA_DCSTAT_GLOBAL_STATS,
    NETDATA_DCSTAT_PID_STATS
};

// variables
enum directory_cache_counters {
    NETDATA_KEY_DC_REFERENCE,
    NETDATA_KEY_DC_SLOW,
    NETDATA_KEY_DC_MISS,

    // Keep this as last and don't skip numbers as it is used as element counter
    NETDATA_DIRECTORY_CACHE_END
};

typedef struct netdata_publish_dcstat_pid {
    uint64_t cache_access;
    uint64_t file_system;
    uint64_t not_found;
} netdata_dcstat_pid_t;

typedef struct netdata_publish_dcstat {
    long long ratio;
    long long cache_access;

    netdata_dcstat_pid_t curr;
    netdata_dcstat_pid_t prev;
} netdata_publish_dcstat_t;

extern void *ebpf_dcstat_thread(void *ptr);
extern void ebpf_dcstat_create_apps_charts(struct ebpf_module *em, void *ptr);
extern void clean_dcstat_pid_structures();
extern struct config dcstat_config;

#endif // NETDATA_EBPF_DCSTAT_H
