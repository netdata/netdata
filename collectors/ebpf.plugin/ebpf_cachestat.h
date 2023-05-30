// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_CACHESTAT_H
#define NETDATA_EBPF_CACHESTAT_H 1

// Module name
#define NETDATA_EBPF_MODULE_NAME_CACHESTAT "cachestat"

// charts
#define NETDATA_CACHESTAT_HIT_RATIO_CHART "cachestat_ratio"
#define NETDATA_CACHESTAT_DIRTY_CHART "cachestat_dirties"
#define NETDATA_CACHESTAT_HIT_CHART "cachestat_hits"
#define NETDATA_CACHESTAT_MISSES_CHART "cachestat_misses"

#define NETDATA_CACHESTAT_SUBMENU "page_cache"
#define NETDATA_CACHESTAT_CGROUP_SUBMENU "page cache (eBPF)"

#define EBPF_CACHESTAT_DIMENSION_PAGE "pages/s"
#define EBPF_CACHESTAT_DIMENSION_HITS "hits/s"
#define EBPF_CACHESTAT_DIMENSION_MISSES "misses/s"

// configuration file
#define NETDATA_CACHESTAT_CONFIG_FILE "cachestat.conf"

// Contexts
#define NETDATA_CGROUP_CACHESTAT_HIT_RATIO_CONTEXT "cgroup.cachestat_ratio"
#define NETDATA_CGROUP_CACHESTAT_MODIFIED_CACHE_CONTEXT "cgroup.cachestat_dirties"
#define NETDATA_CGROUP_CACHESTAT_HIT_FILES_CONTEXT "cgroup.cachestat_hits"
#define NETDATA_CGROUP_CACHESTAT_MISS_FILES_CONTEXT "cgroup.cachestat_misses"

#define NETDATA_SYSTEMD_CACHESTAT_HIT_RATIO_CONTEXT "services.cachestat_ratio"
#define NETDATA_SYSTEMD_CACHESTAT_MODIFIED_CACHE_CONTEXT "services.cachestat_dirties"
#define NETDATA_SYSTEMD_CACHESTAT_HIT_FILE_CONTEXT "services.cachestat_hits"
#define NETDATA_SYSTEMD_CACHESTAT_MISS_FILES_CONTEXT "services.cachestat_misses"

// ARAL Name
#define NETDATA_EBPF_CACHESTAT_ARAL_NAME "ebpf_cachestat"

// variables
enum cachestat_counters {
    NETDATA_KEY_CALLS_ADD_TO_PAGE_CACHE_LRU,
    NETDATA_KEY_CALLS_MARK_PAGE_ACCESSED,
    NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED,
    NETDATA_KEY_CALLS_MARK_BUFFER_DIRTY,

    NETDATA_CACHESTAT_END
};

enum cachestat_account_dirty_pages {
    NETDATA_CACHESTAT_ACCOUNT_PAGE_DIRTY,
    NETDATA_CACHESTAT_SET_PAGE_DIRTY,
    NETDATA_CACHESTAT_FOLIO_DIRTY,

    NETDATA_CACHESTAT_ACCOUNT_DIRTY_END
};

enum cachestat_indexes {
    NETDATA_CACHESTAT_IDX_RATIO,
    NETDATA_CACHESTAT_IDX_DIRTY,
    NETDATA_CACHESTAT_IDX_HIT,
    NETDATA_CACHESTAT_IDX_MISS
};

enum cachestat_tables {
    NETDATA_CACHESTAT_GLOBAL_STATS,
    NETDATA_CACHESTAT_PID_STATS,
    NETDATA_CACHESTAT_CTRL
};

typedef struct netdata_publish_cachestat_pid {
    uint64_t add_to_page_cache_lru;
    uint64_t mark_page_accessed;
    uint64_t account_page_dirtied;
    uint64_t mark_buffer_dirty;
} netdata_cachestat_pid_t;

typedef struct netdata_publish_cachestat {
    long long ratio;
    long long dirty;
    long long hit;
    long long miss;

    netdata_cachestat_pid_t current;
    netdata_cachestat_pid_t prev;
} netdata_publish_cachestat_t;

void *ebpf_cachestat_thread(void *ptr);
void ebpf_cachestat_release(netdata_publish_cachestat_t *stat);

extern struct config cachestat_config;
extern netdata_ebpf_targets_t cachestat_targets[];
extern ebpf_local_maps_t cachestat_maps[];

#endif // NETDATA_EBPF_CACHESTAT_H
