// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_EBPF_CACHESTAT_H
#define NETDATA_EBPF_CACHESTAT_H 1

// variables
enum cachestat_counters {
    NETDATA_KEY_CALLS_ADD_TO_PAGE_CACHE_LRU,
    NETDATA_KEY_CALLS_MARK_PAGE_ACCESSED,
    NETDATA_KEY_CALLS_ACCOUNT_PAGE_DIRTIED,
    NETDATA_KEY_CALLS_MARK_BUFFER_DIRTY,

    NETDATA_CACHESTAT_END
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

extern void *ebpf_cachestat_thread(void *ptr);

#endif // NETDATA_EBPF_CACHESTAT_H