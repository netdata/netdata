// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_CACHESTAT_H
#define NETDATA_EBPF_CACHESTAT_H 1

// Module name & description
#define NETDATA_EBPF_MODULE_NAME_CACHESTAT "cachestat"
#define NETDATA_EBPF_CACHESTAT_MODULE_DESC                                                                             \
    "Monitor Linux page cache internal functions. This thread is integrated with apps and cgroup."

// charts
#define NETDATA_CACHESTAT_HIT_RATIO_CHART "cachestat_ratio"
#define NETDATA_CACHESTAT_DIRTY_CHART "cachestat_dirties"
#define NETDATA_CACHESTAT_HIT_CHART "cachestat_hits"
#define NETDATA_CACHESTAT_MISSES_CHART "cachestat_misses"

#define NETDATA_CACHESTAT_SUBMENU "page_cache"

#define EBPF_CACHESTAT_UNITS_PAGE "pages/s"
#define EBPF_CACHESTAT_UNITS_HITS "hits/s"
#define EBPF_CACHESTAT_UNITS_MISSES "misses/s"

// configuration file
#define NETDATA_CACHESTAT_CONFIG_FILE "cachestat.conf"

// Contexts
#define NETDATA_MEM_CACHESTAT_HIT_RATIO_CONTEXT "mem.cachestat_ratio"
#define NETDATA_MEM_CACHESTAT_MODIFIED_CACHE_CONTEXT "mem.cachestat_dirties"
#define NETDATA_MEM_CACHESTAT_HIT_FILES_CONTEXT "mem.cachestat_hits"
#define NETDATA_MEM_CACHESTAT_MISS_FILES_CONTEXT "mem.cachestat_misses"

#define NETDATA_CGROUP_CACHESTAT_HIT_RATIO_CONTEXT "cgroup.cachestat_ratio"
#define NETDATA_CGROUP_CACHESTAT_MODIFIED_CACHE_CONTEXT "cgroup.cachestat_dirties"
#define NETDATA_CGROUP_CACHESTAT_HIT_FILES_CONTEXT "cgroup.cachestat_hits"
#define NETDATA_CGROUP_CACHESTAT_MISS_FILES_CONTEXT "cgroup.cachestat_misses"

#define NETDATA_SYSTEMD_CACHESTAT_HIT_RATIO_CONTEXT "systemd.service.cachestat_ratio"
#define NETDATA_SYSTEMD_CACHESTAT_MODIFIED_CACHE_CONTEXT "systemd.service.cachestat_dirties"
#define NETDATA_SYSTEMD_CACHESTAT_HIT_FILE_CONTEXT "systemd.service.cachestat_hits"
#define NETDATA_SYSTEMD_CACHESTAT_MISS_FILES_CONTEXT "systemd.service.cachestat_misses"

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

enum cachestat_tables { NETDATA_CACHESTAT_GLOBAL_STATS, NETDATA_CACHESTAT_PID_STATS, NETDATA_CACHESTAT_CTRL };

void *ebpf_cachestat_thread(void *ptr);

extern struct config cachestat_config;
extern netdata_ebpf_targets_t cachestat_targets[];
extern ebpf_local_maps_t cachestat_maps[];

#endif // NETDATA_EBPF_CACHESTAT_H
