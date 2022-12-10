// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef NETDATA_PAGECACHE_H
#define NETDATA_PAGECACHE_H

#include "rrdengine.h"

extern void *main_mrg;
extern void *main_cache;
extern pthread_key_t query_key;
void query_key_release(void *data);

/* Forward declarations */
struct rrdengine_instance;
struct extent_info;
struct rrdeng_page_descr;

#define INVALID_TIME (0)
#define MAX_PAGE_CACHE_FETCH_RETRIES (3)
#define PAGE_CACHE_FETCH_WAIT_TIMEOUT (3)

struct rrdeng_page_descr {
    uuid_t *id; /* never changes */
    struct extent_info *extent;

    /* page information */
    usec_t start_time_ut;
    usec_t end_time_ut;
    uint32_t update_every_s:24;
    uint8_t type;
    uint32_t page_length;
    uv_file file;               // This is the datafile this descriptor belongs
    void *page;
    void *extent_entry;
};

#define PAGE_INFO_SCRATCH_SZ (8)
struct rrdeng_page_info {
    uint8_t scratch[PAGE_INFO_SCRATCH_SZ]; /* scratch area to be used by page-cache users */

    usec_t start_time_ut;
    usec_t end_time_ut;
    uint32_t page_length;
};

struct pg_alignment {
    uint32_t page_length;
    uint32_t refcount;
};

/* maps time ranges to pages */
struct pg_cache_page_index {
    uuid_t id;
    /*
     * care: JudyL_array indices are converted from useconds to seconds to fit in one word in 32-bit architectures
     * TODO: examine if we want to support better granularity than seconds
     */
    Pvoid_t JudyL_array;
    unsigned short refcount;
    unsigned short writers;
    uv_rwlock_t lock;

    /*
     * Only one effective writer, data deletion workqueue.
     * It's also written during the DB loading phase.
     */
    usec_t oldest_time_ut;

    /*
     * Only one effective writer, data collection thread.
     * It's also written by the data deletion workqueue when data collection is disabled for this metric.
     */
    usec_t latest_time_ut;

    struct rrdengine_instance *ctx;
    uint32_t latest_update_every_s;
};

/* maps UUIDs to page indices */
struct pg_cache_metrics_index {
    uv_rwlock_t lock;
    Pvoid_t JudyHS_array;
};

struct page_cache { /* TODO: add statistics */
    uv_rwlock_t pg_cache_rwlock; /* page cache lock */

    struct pg_cache_metrics_index metrics_index;
    unsigned page_descriptors;
    unsigned active_descriptors;
    unsigned populated_pages;
};

struct rrdeng_page_descr *pg_cache_create_descr(void);
void pg_cache_insert(struct rrdengine_instance *ctx, struct pg_cache_page_index *index, struct rrdeng_page_descr *descr);
unsigned pg_cache_preload(struct rrdengine_instance *ctx, void *handle, time_t start_time_t, time_t end_time_t);
void *pg_cache_lookup_next(struct rrdengine_instance *ctx, void *data, time_t start_time_t, time_t end_time_t);
struct pg_cache_page_index *create_page_index(uuid_t *id, struct rrdengine_instance *ctx);
void init_page_cache(struct rrdengine_instance *ctx);
void free_page_cache(struct rrdengine_instance *ctx);
void pg_cache_add_new_metric_time(struct pg_cache_page_index *page_index, struct rrdeng_page_descr *descr);

void rrdeng_page_descr_aral_go_singlethreaded(void);
void rrdeng_page_descr_aral_go_multithreaded(void);
void rrdeng_page_descr_use_malloc(void);
void rrdeng_page_descr_use_mmap(void);
bool rrdeng_page_descr_is_mmap(void);
struct rrdeng_page_descr *rrdeng_page_descr_mallocz(void);
void rrdeng_page_descr_freez(struct rrdeng_page_descr *descr);


/* The caller must hold a reference to the page and must have already set the new data */
//static inline void pg_cache_atomic_set_pg_info(struct rrdeng_collect_handle *handle, usec_t end_time_ut, uint32_t page_length)
//{
//    fatal_assert(!(end_time_ut & 1));
//    __sync_synchronize();
//    handle->end_time_ut |= 1; /* mark start of uncertainty period by adding 1 microsecond */
//    __sync_synchronize();
//    handle->page_length = page_length;
//    __sync_synchronize();
//    handle->end_time_ut = end_time_ut; /* mark end of uncertainty period */
//}

#endif /* NETDATA_PAGECACHE_H */
