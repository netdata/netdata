// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_DCSTAT_H
#define NETDATA_EBPF_DCSTAT_H 1


// charts
#define NETDATA_DC_HIT_CHART "dc_hit_ratio"
#define NETDATA_DC_REQUEST_CHART "dc_reference"
#define NETDATA_DC_REQUEST_NOT_CACHE_CHART "dc_not_cache"
#define NETDATA_DC_REQUEST_NOT_FOUND_CHART "dc_not_found"

#define NETDATA_DIRECTORY_CACHE_SUBMENU "directory cache (eBPF)"

// configuration file
#define NETDATA_DIRECTORY_DCSTAT_CONFIG_FILE "dcstat.conf"

enum directory_cache_indexes {
    NETDATA_DCSTAT_IDX_RATIO,
    NETDATA_DCSTAT_IDX_REFERENCE,
    NETDATA_DCSTAT_IDX_SLOW,
    NETDATA_DCSTAT_IDX_MISS,

    // Keep this as last and don't skip numbers as it is used as element counter
    NETDATA_DCSTAT_IDX_END
};

typedef struct netdata_publish_dcstat_pid {
    uint64_t reference;
    uint64_t slow;
    uint64_t miss;
} netdata_dcstat_pid_t;

typedef struct netdata_publish_dcstat {
    long long ratio;

    netdata_dcstat_pid_t curr;
} netdata_publish_dcstat_t;

extern void *ebpf_dcstat_thread(void *ptr);
extern void ebpf_dcstat_create_apps_charts(struct ebpf_module *em, void *ptr);

#endif // NETDATA_EBPF_DCSTAT_H
