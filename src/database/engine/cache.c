// SPDX-License-Identifier: GPL-3.0-or-later
#include "cache.h"

/* STATES AND TRANSITIONS
 *
 *   entry     |       entry
 *     v                 v
 *    HOT -> DIRTY --> CLEAN --> EVICT
 *                 v    |     v
 *               flush  |   evict
 *                 v    |     v
 *               save   |   free
 *             callback | callback
 *
 */

// to use ARAL uncomment the following line:
#if !defined(FSANITIZE_ADDRESS)
#define PGC_WITH_ARAL 1
#endif

#define PGC_QUEUE_LOCK_AS_WAITING_QUEUE 1

typedef enum __attribute__ ((__packed__)) {
    // mutually exclusive flags
    PGC_PAGE_CLEAN                       = (1 << 0), // none of the following
    PGC_PAGE_DIRTY                       = (1 << 1), // contains unsaved data
    PGC_PAGE_HOT                         = (1 << 2), // currently being collected

    // flags related to various actions on each page
    PGC_PAGE_IS_BEING_DELETED            = (1 << 3),
    PGC_PAGE_IS_BEING_MIGRATED_TO_V2     = (1 << 4),
    PGC_PAGE_HAS_NO_DATA_IGNORE_ACCESSES = (1 << 5),
    PGC_PAGE_HAS_BEEN_ACCESSED           = (1 << 6),
} PGC_PAGE_FLAGS;

#define page_flag_check(page, flag) (__atomic_load_n(&((page)->flags), __ATOMIC_ACQUIRE) & (flag))
#define page_flag_set(page, flag)   __atomic_or_fetch(&((page)->flags), flag, __ATOMIC_RELEASE)
#define page_flag_clear(page, flag) __atomic_and_fetch(&((page)->flags), ~(flag), __ATOMIC_RELEASE)

#define page_get_status_flags(page) page_flag_check(page, PGC_PAGE_HOT | PGC_PAGE_DIRTY | PGC_PAGE_CLEAN)
#define is_page_hot(page) (page_get_status_flags(page) == PGC_PAGE_HOT)
#define is_page_dirty(page) (page_get_status_flags(page) == PGC_PAGE_DIRTY)
#define is_page_clean(page) (page_get_status_flags(page) == PGC_PAGE_CLEAN)

struct pgc_page {
    // indexing data
    Word_t section;
    Word_t metric_id;
    time_t start_time_s;
    time_t end_time_s;
    uint32_t update_every_s;
    uint32_t assumed_size;

    REFCOUNT refcount;
    uint16_t accesses;              // counts the number of accesses on this page
    PGC_PAGE_FLAGS flags;
    SPINLOCK transition_spinlock;   // when the page changes between HOT, DIRTY, CLEAN, we have to get this lock

    struct {
        struct pgc_page *next;
        struct pgc_page *prev;
    } link;

    void *data;
    uint8_t custom_data[];

    // IMPORTANT!
    // THIS STRUCTURE NEEDS TO BE INITIALIZED BY HAND!
};

struct pgc_queue {
#if defined(PGC_QUEUE_LOCK_AS_WAITING_QUEUE)
    WAITQ wq;
#else
    SPINLOCK spinlock;
#endif
    union {
        PGC_PAGE *base;
        Pvoid_t sections_judy;
    };
    PGC_PAGE_FLAGS flags;
    size_t version;
    size_t last_version_checked;
    bool linked_list_in_sections_judy; // when true, we use 'sections_judy', otherwise we use 'base'
    struct pgc_queue_statistics *stats;
};

struct pgc {
    struct {
        char name[PGC_NAME_MAX + 1];
        bool stats; // enable extended statistics
        bool use_all_ram;

        size_t partitions;
        int64_t clean_size;
        size_t max_dirty_pages_per_call;
        size_t max_pages_per_inline_eviction;
        size_t max_skip_pages_per_inline_eviction;
        size_t max_flushes_inline;
        size_t max_workers_evict_inline;
        size_t additional_bytes_per_page;
        int64_t out_of_memory_protection_bytes;
        free_clean_page_callback pgc_free_clean_cb;
        save_dirty_page_callback pgc_save_dirty_cb;
        save_dirty_init_callback pgc_save_init_cb;
        PGC_OPTIONS options;

        ssize_t severe_pressure_per1000;
        ssize_t aggressive_evict_per1000;
        ssize_t healthy_size_per1000;
        ssize_t evict_low_threshold_per1000;

        dynamic_target_cache_size_callback dynamic_target_size_cb;
        nominal_page_size_callback nominal_page_size_cb;
    } config;

    struct {
        ND_THREAD *thread;              // the thread
        struct completion completion;   // signal the thread to wake up
    } evictor;

    struct pgc_index {
        RW_SPINLOCK rw_spinlock;
        Pvoid_t sections_judy;
#ifdef PGC_WITH_ARAL
        ARAL *aral;
#endif
    } *index;

    struct {
        SPINLOCK spinlock;
        ssize_t per1000;
    } usage;

    struct pgc_queue clean;       // LRU is applied here to free memory from the cache
    struct pgc_queue dirty;       // in the dirty list, pages are ordered the way they were marked dirty
    struct pgc_queue hot;         // in the hot list, pages are order the way they were marked hot
    struct pgc_statistics stats;        // statistics

#ifdef NETDATA_PGC_POINTER_CHECK
    netdata_mutex_t global_pointer_registry_mutex;
    Pvoid_t global_pointer_registry;
#endif
};

// ----------------------------------------------------------------------------
// validate each pointer is indexed once - internal checks only

static inline void pointer_index_init(PGC *cache __maybe_unused) {
#ifdef NETDATA_PGC_POINTER_CHECK
    netdata_mutex_init(&cache->global_pointer_registry_mutex);
#else
    ;
#endif
}

static inline void pointer_destroy_index(PGC *cache __maybe_unused) {
#ifdef NETDATA_PGC_POINTER_CHECK
    netdata_mutex_lock(&cache->global_pointer_registry_mutex);
    JudyHSFreeArray(&cache->global_pointer_registry, PJE0);
    netdata_mutex_unlock(&cache->global_pointer_registry_mutex);
#else
    ;
#endif
}
static inline void pointer_add(PGC *cache __maybe_unused, PGC_PAGE *page __maybe_unused) {
#ifdef NETDATA_PGC_POINTER_CHECK
    netdata_mutex_lock(&cache->global_pointer_registry_mutex);
    Pvoid_t *PValue = JudyHSIns(&cache->global_pointer_registry, &page, sizeof(void *), PJE0);
    if(*PValue != NULL)
        fatal("pointer already exists in registry");
    *PValue = page;
    netdata_mutex_unlock(&cache->global_pointer_registry_mutex);
#else
    ;
#endif
}

static inline void pointer_check(PGC *cache __maybe_unused, PGC_PAGE *page __maybe_unused) {
#ifdef NETDATA_PGC_POINTER_CHECK
    netdata_mutex_lock(&cache->global_pointer_registry_mutex);
    Pvoid_t *PValue = JudyHSGet(cache->global_pointer_registry, &page, sizeof(void *));
    if(PValue == NULL)
        fatal("pointer is not found in registry");
    netdata_mutex_unlock(&cache->global_pointer_registry_mutex);
#else
    ;
#endif
}

static inline void pointer_del(PGC *cache __maybe_unused, PGC_PAGE *page __maybe_unused) {
#ifdef NETDATA_PGC_POINTER_CHECK
    netdata_mutex_lock(&cache->global_pointer_registry_mutex);
    int ret = JudyHSDel(&cache->global_pointer_registry, &page, sizeof(void *), PJE0);
    if(!ret)
        fatal("pointer to be deleted does not exist in registry");
    netdata_mutex_unlock(&cache->global_pointer_registry_mutex);
#else
    ;
#endif
}

// ----------------------------------------------------------------------------
// helpers

static inline size_t page_assumed_size(PGC *cache, size_t size) {
    return size + (sizeof(PGC_PAGE) + cache->config.additional_bytes_per_page + sizeof(Word_t) * 3);
}

static inline size_t page_size_from_assumed_size(PGC *cache, size_t assumed_size) {
    return assumed_size - (sizeof(PGC_PAGE) + cache->config.additional_bytes_per_page + sizeof(Word_t) * 3);
}

// ----------------------------------------------------------------------------
// locking

static inline size_t pgc_indexing_partition(PGC *cache, Word_t metric_id) {
    static __thread Word_t last_metric_id = 0;
    static __thread size_t last_partition = 0;

    if(metric_id == last_metric_id || cache->config.partitions == 1)
        return last_partition;

    last_metric_id = metric_id;
    last_partition = indexing_partition(metric_id, cache->config.partitions);

    return last_partition;
}

#define pgc_index_read_lock(cache, partition) rw_spinlock_read_lock(&(cache)->index[partition].rw_spinlock)
#define pgc_index_read_unlock(cache, partition) rw_spinlock_read_unlock(&(cache)->index[partition].rw_spinlock)
#define pgc_index_write_lock(cache, partition) rw_spinlock_write_lock(&(cache)->index[partition].rw_spinlock)
#define pgc_index_write_unlock(cache, partition) rw_spinlock_write_unlock(&(cache)->index[partition].rw_spinlock)
#define pgc_index_trywrite_lock(cache, partition, force) ({                             \
    bool _result;                                                                       \
    if (force) {                                                                        \
        rw_spinlock_write_lock(&(cache)->index[partition].rw_spinlock);                 \
        _result = true;                                                                 \
    } else                                                                              \
        _result = rw_spinlock_trywrite_lock(&(cache)->index[partition].rw_spinlock);    \
    _result;                                                                            \
})

#define PGC_QUEUE_LOCK_PRIO_COLLECTORS WAITQ_PRIO_URGENT
#define PGC_QUEUE_LOCK_PRIO_EVICTORS WAITQ_PRIO_HIGH
#define PGC_QUEUE_LOCK_PRIO_FLUSHERS WAITQ_PRIO_NORMAL
#define PGC_QUEUE_LOCK_PRIO_LOW WAITQ_PRIO_LOW

#if defined(PGC_QUEUE_LOCK_AS_WAITING_QUEUE)
#define pgc_queue_trylock(cache, ll, prio) waitq_try_acquire(&((ll)->wq), prio)
#define pgc_queue_lock(cache, ll, prio) waitq_acquire(&((ll)->wq), prio)
#define pgc_queue_unlock(cache, ll) waitq_release(&((ll)->wq))
#else
#define pgc_queue_trylock(cache, ll, prio) spinlock_trylock(&((ll)->spinlock))
#define pgc_queue_lock(cache, ll, prio) spinlock_lock(&((ll)->spinlock))
#define pgc_queue_unlock(cache, ll) spinlock_unlock(&((ll)->spinlock))
#endif

#define page_transition_trylock(cache, page) spinlock_trylock(&(page)->transition_spinlock)
#define page_transition_lock(cache, page) spinlock_lock(&(page)->transition_spinlock)
#define page_transition_unlock(cache, page) spinlock_unlock(&(page)->transition_spinlock)

// ----------------------------------------------------------------------------
// size histogram

static void pgc_size_histogram_init(struct pgc_size_histogram *h) {
    // the histogram needs to be all-inclusive for the possible sizes
    // so, we start from 0, and the last value is SIZE_MAX.

    size_t values[PGC_SIZE_HISTOGRAM_ENTRIES] = {
        0, 32, 64, 128, 256, 512, 1024, 2048,
        4096, 8192, 16384, 32768, 65536, 128 * 1024, SIZE_MAX
    };

    size_t last_value = 0;
    for(size_t i = 0; i < PGC_SIZE_HISTOGRAM_ENTRIES; i++) {
        if(i > 0 && values[i] == 0)
            fatal("only the first value in the array can be zero");

        if(i > 0 && values[i] <= last_value)
            fatal("the values need to be sorted");

        h->array[i].upto = values[i];
        last_value = values[i];
    }
}

static inline size_t pgc_size_histogram_slot(struct pgc_size_histogram *h, size_t size) {
    if(size <= h->array[0].upto)
        return 0;

    if(size >= h->array[_countof(h->array) - 1].upto)
        return _countof(h->array) - 1;

    // binary search for the right size
    size_t low = 0, high = _countof(h->array) - 1;
    while (low < high) {
        size_t mid = low + (high - low) / 2;
        if (size < h->array[mid].upto)
            high = mid;
        else
            low = mid + 1;
    }
    return low - 1;
}

static inline void pgc_size_histogram_add(PGC *cache, struct pgc_size_histogram *h, PGC_PAGE *page) {
    size_t size;
    if(cache->config.nominal_page_size_cb)
        size = cache->config.nominal_page_size_cb(page->data);
    else
        size = page_size_from_assumed_size(cache, page->assumed_size);

    size_t slot = pgc_size_histogram_slot(h, size);
    internal_fatal(slot >= _countof(h->array), "hey!");

    __atomic_add_fetch(&h->array[slot].count, 1, __ATOMIC_RELAXED);
}

static inline void pgc_size_histogram_del(PGC *cache, struct pgc_size_histogram *h, PGC_PAGE *page) {
    size_t size;
    if(cache->config.nominal_page_size_cb)
        size = cache->config.nominal_page_size_cb(page->data);
    else
        size = page_size_from_assumed_size(cache, page->assumed_size);

    size_t slot = pgc_size_histogram_slot(h, size);
    internal_fatal(slot >= _countof(h->array), "hey!");

    __atomic_sub_fetch(&h->array[slot].count, 1, __ATOMIC_RELAXED);
}

// ----------------------------------------------------------------------------
// evictions control

ALWAYS_INLINE
static int64_t pgc_threshold(ssize_t threshold, int64_t wanted, int64_t current, int64_t clean) {
    if(current < clean)
        current = clean;

    if(wanted < current - clean)
        wanted = current - clean;

    int64_t ret = wanted * threshold / 1000LL;
    if(ret < current - clean)
        ret = current - clean;

    return ret;
}

ALWAYS_INLINE
static int64_t pgc_wanted_size(const int64_t hot, const int64_t hot_max, const int64_t dirty_max, const int64_t index) {
    // our promise to users
    const int64_t max_size1 = MAX(hot_max, hot) * 2;

    // protection against slow flushing
    const int64_t max_size2 = hot_max + MAX(dirty_max * 2, hot_max * 2 / 3) + index;

    // the final wanted cache size
    return MIN(max_size1, max_size2);
}

static ssize_t cache_usage_per1000(PGC *cache, int64_t *size_to_evict) {

    if(size_to_evict)
        spinlock_lock(&cache->usage.spinlock);

    else if(!spinlock_trylock(&cache->usage.spinlock))
        return __atomic_load_n(&cache->usage.per1000, __ATOMIC_RELAXED);

    int64_t wanted_cache_size;

    const int64_t dirty = __atomic_load_n(&cache->dirty.stats->size, __ATOMIC_RELAXED);
    const int64_t hot = __atomic_load_n(&cache->hot.stats->size, __ATOMIC_RELAXED);
    const int64_t clean = __atomic_load_n(&cache->clean.stats->size, __ATOMIC_RELAXED);
    const int64_t evicting = __atomic_load_n(&cache->stats.evicting_size, __ATOMIC_RELAXED);
    const int64_t flushing = __atomic_load_n(&cache->stats.flushing_size, __ATOMIC_RELAXED);
    const int64_t current_cache_size = __atomic_load_n(&cache->stats.size, __ATOMIC_RELAXED);
    const int64_t all_pages_size = hot + dirty + clean + evicting + flushing;
    const int64_t index = current_cache_size > all_pages_size ? current_cache_size - all_pages_size : 0;
    const int64_t referenced_size = __atomic_load_n(&cache->stats.referenced_size, __ATOMIC_RELAXED);

    if(cache->config.options & PGC_OPTIONS_AUTOSCALE) {
        const int64_t dirty_max = __atomic_load_n(&cache->dirty.stats->max_size, __ATOMIC_RELAXED);
        const int64_t hot_max = __atomic_load_n(&cache->hot.stats->max_size, __ATOMIC_RELAXED);

        if(cache->config.dynamic_target_size_cb) {
            wanted_cache_size = pgc_wanted_size(hot, hot, dirty, index);

            const int64_t wanted_cache_size_cb = cache->config.dynamic_target_size_cb();
            if(wanted_cache_size_cb > wanted_cache_size)
                wanted_cache_size = wanted_cache_size_cb;
        }
        else
            wanted_cache_size = pgc_wanted_size(hot, hot_max, dirty_max, index);

        if (wanted_cache_size < hot + dirty + index + cache->config.clean_size)
            wanted_cache_size = hot + dirty + index + cache->config.clean_size;
    }
    else
        wanted_cache_size = hot + dirty + index + cache->config.clean_size;

    // calculate the absolute minimum we can go
    const int64_t min_cache_size1 = (referenced_size > hot ? referenced_size : hot) + dirty + index;
    const int64_t min_cache_size2 = (current_cache_size > clean) ? current_cache_size - clean : min_cache_size1;
    const int64_t min_cache_size = MAX(min_cache_size1, min_cache_size2);

    if(cache->config.out_of_memory_protection_bytes) {
        // out of memory protection
        OS_SYSTEM_MEMORY sm = os_system_memory(false);
        if(OS_SYSTEM_MEMORY_OK(sm)) {
            // when the total exists, ram_available_bytes is also right

            const int64_t ram_available_bytes = (int64_t)sm.ram_available_bytes;

            const int64_t min_available = cache->config.out_of_memory_protection_bytes;
            if (ram_available_bytes < min_available) {
                // we must shrink
                int64_t must_lose = min_available - ram_available_bytes;

                if(current_cache_size > must_lose)
                    wanted_cache_size = current_cache_size - must_lose;
                else
                    wanted_cache_size = min_cache_size;
            }
            else if(cache->config.use_all_ram) {
                // we can grow
                wanted_cache_size = current_cache_size + (ram_available_bytes - min_available);
            }
        }
    }

    // never go below our minimum
    if(unlikely(wanted_cache_size < min_cache_size))
        wanted_cache_size = min_cache_size;

    // protection for the case the cache is totally empty
    if(unlikely(wanted_cache_size < 65536))
        wanted_cache_size = 65536;

    const ssize_t per1000 = (ssize_t)(current_cache_size * 1000LL / wanted_cache_size);
    __atomic_store_n(&cache->usage.per1000, per1000, __ATOMIC_RELAXED);
    __atomic_store_n(&cache->stats.wanted_cache_size, wanted_cache_size, __ATOMIC_RELAXED);
    __atomic_store_n(&cache->stats.current_cache_size, current_cache_size, __ATOMIC_RELAXED);

    int64_t healthy_target = pgc_threshold(cache->config.healthy_size_per1000, wanted_cache_size, current_cache_size, clean);
    if(current_cache_size > healthy_target) {
        int64_t low_watermark_target = pgc_threshold(cache->config.evict_low_threshold_per1000, wanted_cache_size, current_cache_size, clean);

        int64_t size_to_evict_now = current_cache_size - low_watermark_target;
        if(size_to_evict_now > clean)
            size_to_evict_now = clean;

        if(size_to_evict)
            *size_to_evict = size_to_evict_now;

        bool signal = false;
        if(per1000 >= cache->config.severe_pressure_per1000) {
            __atomic_add_fetch(&cache->stats.events_cache_under_severe_pressure, 1, __ATOMIC_RELAXED);
            signal = true;
        }
        else if(per1000 >= cache->config.aggressive_evict_per1000) {
            __atomic_add_fetch(&cache->stats.events_cache_needs_space_aggressively, 1, __ATOMIC_RELAXED);
            signal = true;
        }

        if(signal) {
            completion_mark_complete_a_job(&cache->evictor.completion);
            p2_add_fetch(&cache->stats.p2_waste_evict_thread_signals, 1);
        }
    }

    spinlock_unlock(&cache->usage.spinlock);

    return per1000;
}

static inline bool cache_pressure(PGC *cache, ssize_t limit) {
    return (cache_usage_per1000(cache, NULL) >= limit);
}

#define cache_under_severe_pressure(cache) cache_pressure(cache, (cache)->config.severe_pressure_per1000)
#define cache_needs_space_aggressively(cache) cache_pressure(cache, (cache)->config.aggressive_evict_per1000)
#define cache_above_healthy_limit(cache) cache_pressure(cache, (cache)->config.healthy_size_per1000)

typedef bool (*evict_filter)(PGC_PAGE *page, void *data);
static bool evict_pages_with_filter(PGC *cache, size_t max_skip, size_t max_evict, bool wait, bool all_of_them, evict_filter filter, void *data);
#define evict_pages(cache, max_skip, max_evict, wait, all_of_them) evict_pages_with_filter(cache, max_skip, max_evict, wait, all_of_them, NULL, NULL)

static inline bool flushing_critical(PGC *cache);
static bool flush_pages(PGC *cache, size_t max_flushes, Word_t section, bool wait, bool all_of_them);

static ALWAYS_INLINE void evict_pages_inline(PGC *cache, bool on_release) {
    const ssize_t per1000 = cache_usage_per1000(cache, NULL);

    if(!(cache->config.options & PGC_OPTIONS_EVICT_PAGES_NO_INLINE)) {
        if (per1000 > cache->config.aggressive_evict_per1000 && !on_release) {
            // the threads that add pages, turn into evictors when the cache needs evictions aggressively
            p2_add_fetch(&cache->stats.p2_waste_evictions_inline_on_add, 1);
            evict_pages(cache,
                        cache->config.max_skip_pages_per_inline_eviction,
                        cache->config.max_pages_per_inline_eviction,
                        false, false);
        }
        else if (per1000 > cache->config.severe_pressure_per1000 && on_release) {
            // the threads that are releasing pages, turn into evictors when the cache is critical
            p2_add_fetch(&cache->stats.p2_waste_evictions_inline_on_release, 1);

            evict_pages(cache,
                        cache->config.max_skip_pages_per_inline_eviction,
                        cache->config.max_pages_per_inline_eviction,
                        false, false);
        }
    }
}

static ALWAYS_INLINE void evict_on_clean_page_added(PGC *cache) {
    evict_pages_inline(cache, false);
}

static ALWAYS_INLINE void evict_on_page_release_when_permitted(PGC *cache) {
    evict_pages_inline(cache, true);
}

static ALWAYS_INLINE void flush_inline(PGC *cache, bool on_release) {
    if(!(cache->config.options & PGC_OPTIONS_FLUSH_PAGES_NO_INLINE) && flushing_critical(cache)) {
        if (on_release)
            p2_add_fetch(&cache->stats.p2_waste_flush_on_release, 1);
        else
            p2_add_fetch(&cache->stats.p2_waste_flush_on_add, 1);

        flush_pages(cache, cache->config.max_flushes_inline, PGC_SECTION_ALL, false, false);
    }
}

static ALWAYS_INLINE void flush_on_page_add(PGC *cache) {
    flush_inline(cache, false);
}

static ALWAYS_INLINE void flush_on_page_hot_release(PGC *cache) {
    flush_inline(cache, true);
}


// ----------------------------------------------------------------------------
// flushing control

static inline bool flushing_critical(PGC *cache) {
    if(unlikely(__atomic_load_n(&cache->dirty.stats->size, __ATOMIC_RELAXED) > __atomic_load_n(&cache->hot.stats->max_size, __ATOMIC_RELAXED))) {
        __atomic_add_fetch(&cache->stats.events_flush_critical, 1, __ATOMIC_RELAXED);
        return true;
    }

    return false;
}

// ----------------------------------------------------------------------------
// Linked list management

static inline void atomic_set_max_size_t(size_t *max, size_t desired) {
    size_t expected;

    expected = __atomic_load_n(max, __ATOMIC_RELAXED);

    do {

        if(expected >= desired)
            return;

    } while(!__atomic_compare_exchange_n(max, &expected, desired,
                                          false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));
}

static inline void atomic_set_max_int64_t(int64_t *max, int64_t desired) {
    int64_t expected;

    expected = __atomic_load_n(max, __ATOMIC_RELAXED);

    do {

        if(expected >= desired)
            return;

    } while(!__atomic_compare_exchange_n(max, &expected, desired,
                                         false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));
}

struct section_pages {
    SPINLOCK migration_to_v2_spinlock;
    size_t entries;
    size_t size;
    PGC_PAGE *base;
};

static struct aral_statistics pgc_aral_statistics = { 0 };

static ARAL *pgc_sections_aral = NULL;

static void pgc_section_pages_static_aral_init(void) {
    static SPINLOCK spinlock = SPINLOCK_INITIALIZER;

    spinlock_lock(&spinlock);

    if(!pgc_sections_aral) {
        pgc_sections_aral = aral_create(
            "pgc-sections", sizeof(struct section_pages), 0, 0, &pgc_aral_statistics,
            NULL, NULL, false, false, false);

        pulse_aral_register_statistics(&pgc_aral_statistics, "pgc");
    }

    spinlock_unlock(&spinlock);
}

static ALWAYS_INLINE void pgc_stats_queue_judy_change(PGC *cache, struct pgc_queue *ll, int64_t delta) {
    __atomic_add_fetch(&ll->stats->size, delta, __ATOMIC_RELAXED);
    __atomic_add_fetch(&cache->stats.size, delta, __ATOMIC_RELAXED);
}

static ALWAYS_INLINE void pgc_stats_index_judy_change(PGC *cache, int64_t delta) {
    __atomic_add_fetch(&cache->stats.size, delta, __ATOMIC_RELAXED);
}

static ALWAYS_INLINE void pgc_queue_add(PGC *cache __maybe_unused, struct pgc_queue *q, PGC_PAGE *page, bool having_lock, WAITQ_PRIORITY prio __maybe_unused) {
    if(!having_lock)
        pgc_queue_lock(cache, q, prio);

    internal_fatal(page_get_status_flags(page) != 0,
                   "DBENGINE CACHE: invalid page flags, the page has %d, but it is should be %d",
                   page_get_status_flags(page),
                   0);

    if(q->linked_list_in_sections_judy) {
        // HOT and DIRTY pages end up here.

        JudyAllocThreadPulseReset();
        int64_t mem_delta = 0;

        Pvoid_t *section_pages_pptr = JudyLIns(&q->sections_judy, page->section, PJE0);
        if(section_pages_pptr == NULL || section_pages_pptr == PJERR)
            fatal("DBENGINE CACHE: JudyLIns(q->sections_judy, 0x%lx) failed, q->sections_judy = %p, result = %p",
                  (long unsigned)page->section, q->sections_judy, section_pages_pptr);

        struct section_pages *sp = *section_pages_pptr;
        if(!sp) {
            // sp = callocz(1, sizeof(struct section_pages));
            sp = aral_mallocz(pgc_sections_aral);
            memset(sp, 0, sizeof(struct section_pages));

            *section_pages_pptr = sp;

            mem_delta += sizeof(struct section_pages);
        }

        mem_delta += JudyAllocThreadPulseGetAndReset();
        pgc_stats_queue_judy_change(cache, q, mem_delta);

        sp->entries++;
        sp->size += page->assumed_size;
        DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(sp->base, page, link.prev, link.next);

        if((sp->entries % cache->config.max_dirty_pages_per_call) == 0)
            q->version++;
    }
    else {
        // CLEAN pages end up here.
        // - New pages created as CLEAN, always have 1 access.
        // - DIRTY pages made CLEAN, depending on their accesses may be appended (accesses > 0) or prepended (accesses = 0).

        if(page->accesses || page_flag_check(page, PGC_PAGE_HAS_BEEN_ACCESSED | PGC_PAGE_HAS_NO_DATA_IGNORE_ACCESSES) == PGC_PAGE_HAS_BEEN_ACCESSED) {
            DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(q->base, page, link.prev, link.next);
            page_flag_clear(page, PGC_PAGE_HAS_BEEN_ACCESSED);
        }
        else
            DOUBLE_LINKED_LIST_PREPEND_ITEM_UNSAFE(q->base, page, link.prev, link.next);

        q->version++;
    }

    page_flag_set(page, q->flags);

    if(!having_lock)
        pgc_queue_unlock(cache, q);

    size_t entries = __atomic_add_fetch(&q->stats->entries, 1, __ATOMIC_RELAXED);
    int64_t size   = __atomic_add_fetch(&q->stats->size, page->assumed_size, __ATOMIC_RELAXED);
    __atomic_add_fetch(&q->stats->added_entries, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&q->stats->added_size, page->assumed_size, __ATOMIC_RELAXED);

    atomic_set_max_size_t(&q->stats->max_entries, entries);
    atomic_set_max_int64_t(&q->stats->max_size, size);

    if(cache->config.stats)
        pgc_size_histogram_add(cache, &q->stats->size_histogram, page);
}

static ALWAYS_INLINE void pgc_queue_del(PGC *cache __maybe_unused, struct pgc_queue *q, PGC_PAGE *page, bool having_lock,
    WAITQ_PRIORITY prio __maybe_unused) {
    if(cache->config.stats)
        pgc_size_histogram_del(cache, &q->stats->size_histogram, page);

    __atomic_sub_fetch(&q->stats->entries, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&q->stats->size, page->assumed_size, __ATOMIC_RELAXED);
    __atomic_add_fetch(&q->stats->removed_entries, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&q->stats->removed_size, page->assumed_size, __ATOMIC_RELAXED);

    if(!having_lock)
        pgc_queue_lock(cache, q, prio);

    internal_fatal(page_get_status_flags(page) != q->flags,
                   "DBENGINE CACHE: invalid page flags, the page has %d, but it is should be %d",
                   page_get_status_flags(page),
        q->flags);

    page_flag_clear(page, q->flags);

    if(q->linked_list_in_sections_judy) {
        Pvoid_t *section_pages_pptr = JudyLGet(q->sections_judy, page->section, PJE0);
        if(section_pages_pptr == NULL || section_pages_pptr == PJERR)
            fatal("DBENGINE CACHE: JudyLGet(q->sections_judy, 0x%lx) failed, q->sections_judy = %p",
                  (long unsigned)page->section, q->sections_judy);

        struct section_pages *sp = *section_pages_pptr;
        sp->entries--;
        sp->size -= page->assumed_size;
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(sp->base, page, link.prev, link.next);

        if(!sp->base) {
            JudyAllocThreadPulseReset();
            int64_t mem_delta = 0;

            int rc = JudyLDel(&q->sections_judy, page->section, PJE0);

            if(!rc)
                fatal("DBENGINE CACHE: cannot delete section from Judy LL");

            // freez(sp);
            aral_freez(pgc_sections_aral, sp);

            mem_delta -= sizeof(struct section_pages);
            mem_delta += JudyAllocThreadPulseGetAndReset();

            pgc_stats_queue_judy_change(cache, q, mem_delta);
        }
    }
    else {
        DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(q->base, page, link.prev, link.next);
        q->version++;
    }

    if(!having_lock)
        pgc_queue_unlock(cache, q);
}

static ALWAYS_INLINE void page_has_been_accessed(PGC *cache, PGC_PAGE *page) {
    PGC_PAGE_FLAGS flags = page_flag_check(page, PGC_PAGE_CLEAN | PGC_PAGE_HAS_NO_DATA_IGNORE_ACCESSES);

    if (!(flags & PGC_PAGE_HAS_NO_DATA_IGNORE_ACCESSES)) {
        __atomic_add_fetch(&page->accesses, 1, __ATOMIC_RELAXED);

        if (flags & PGC_PAGE_CLEAN) {
            if(pgc_queue_trylock(cache, &cache->clean, PGC_QUEUE_LOCK_PRIO_EVICTORS)) {
                DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(cache->clean.base, page, link.prev, link.next);
                DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(cache->clean.base, page, link.prev, link.next);
                pgc_queue_unlock(cache, &cache->clean);
                page_flag_clear(page, PGC_PAGE_HAS_BEEN_ACCESSED);
            }
            else
                page_flag_set(page, PGC_PAGE_HAS_BEEN_ACCESSED);
        }
    }
}


// ----------------------------------------------------------------------------
// state transitions

static ALWAYS_INLINE void page_set_clean(PGC *cache, PGC_PAGE *page, bool having_transition_lock, bool having_clean_lock, WAITQ_PRIORITY prio) {
    if(!having_transition_lock)
        page_transition_lock(cache, page);

    PGC_PAGE_FLAGS flags = page_get_status_flags(page);

    if(flags & PGC_PAGE_CLEAN) {
        if(!having_transition_lock)
            page_transition_unlock(cache, page);
        return;
    }

    if(flags & PGC_PAGE_HOT)
        pgc_queue_del(cache, &cache->hot, page, false, prio);

    if(flags & PGC_PAGE_DIRTY)
        pgc_queue_del(cache, &cache->dirty, page, false, prio);

    // first add to linked list, the set the flag (required for move_page_last())
    pgc_queue_add(cache, &cache->clean, page, having_clean_lock, prio);

    if(!having_transition_lock)
        page_transition_unlock(cache, page);
}

static ALWAYS_INLINE void page_set_dirty(PGC *cache, PGC_PAGE *page, bool having_hot_lock, WAITQ_PRIORITY prio) {
    if(!having_hot_lock)
        // to avoid deadlocks, we have to get the hot lock before the page transition
        // since this is what all_hot_to_dirty() does
        pgc_queue_lock(cache, &cache->hot, prio);

    page_transition_lock(cache, page);

    PGC_PAGE_FLAGS flags = page_get_status_flags(page);

    if(flags & PGC_PAGE_DIRTY) {
        page_transition_unlock(cache, page);

        if(!having_hot_lock)
            // we don't need the hot lock anymore
            pgc_queue_unlock(cache, &cache->hot);

        return;
    }

    __atomic_add_fetch(&cache->stats.hot2dirty_entries, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&cache->stats.hot2dirty_size, page->assumed_size, __ATOMIC_RELAXED);

    if(likely(flags & PGC_PAGE_HOT))
        pgc_queue_del(cache, &cache->hot, page, true, prio);

    if(!having_hot_lock)
        // we don't need the hot lock anymore
        pgc_queue_unlock(cache, &cache->hot);

    if(unlikely(flags & PGC_PAGE_CLEAN))
        pgc_queue_del(cache, &cache->clean, page, false, prio);

    // first add to linked list, the set the flag (required for move_page_last())
    pgc_queue_add(cache, &cache->dirty, page, false, prio);

    __atomic_sub_fetch(&cache->stats.hot2dirty_entries, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&cache->stats.hot2dirty_size, page->assumed_size, __ATOMIC_RELAXED);

    page_transition_unlock(cache, page);
}

static ALWAYS_INLINE void page_set_hot(PGC *cache, PGC_PAGE *page, WAITQ_PRIORITY prio) {
    page_transition_lock(cache, page);

    PGC_PAGE_FLAGS flags = page_get_status_flags(page);

    if(flags & PGC_PAGE_HOT) {
        page_transition_unlock(cache, page);
        return;
    }

    if(flags & PGC_PAGE_DIRTY)
        pgc_queue_del(cache, &cache->dirty, page, false, prio);

    if(flags & PGC_PAGE_CLEAN)
        pgc_queue_del(cache, &cache->clean, page, false, prio);

    // first add to linked list, the set the flag (required for move_page_last())
    pgc_queue_add(cache, &cache->hot, page, false, prio);

    page_transition_unlock(cache, page);
}


// ----------------------------------------------------------------------------
// Referencing

static ALWAYS_INLINE size_t PGC_REFERENCED_PAGES(PGC *cache) {
    return __atomic_load_n(&cache->stats.referenced_entries, __ATOMIC_RELAXED);
}

static ALWAYS_INLINE void PGC_REFERENCED_PAGES_PLUS1(PGC *cache, PGC_PAGE *page) {
    __atomic_add_fetch(&cache->stats.referenced_entries, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&cache->stats.referenced_size, page->assumed_size, __ATOMIC_RELAXED);
}

static ALWAYS_INLINE void PGC_REFERENCED_PAGES_MINUS1(PGC *cache, int64_t assumed_size) {
    __atomic_sub_fetch(&cache->stats.referenced_entries, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&cache->stats.referenced_size, assumed_size, __ATOMIC_RELAXED);
}

// If the page is not already acquired,
// YOU HAVE TO HAVE THE QUEUE (hot, dirty, clean - the page is in), LOCKED!
// If you don't have it locked, NOTHING PREVENTS THIS PAGE FROM VANISHING WHILE THIS IS CALLED!
static ALWAYS_INLINE bool page_acquire(PGC *cache, PGC_PAGE *page) {
    __atomic_add_fetch(&cache->stats.acquires, 1, __ATOMIC_RELAXED);

    REFCOUNT rc = refcount_acquire_advanced(&page->refcount);
    if(REFCOUNT_ACQUIRED(rc)) {
        if(rc == 1)
            PGC_REFERENCED_PAGES_PLUS1(cache, page);

        return true;
    }

    return false;
}

static ALWAYS_INLINE void page_release(PGC *cache, PGC_PAGE *page, bool evict_if_necessary) {
    __atomic_add_fetch(&cache->stats.releases, 1, __ATOMIC_RELAXED);

    int64_t assumed_size = page->assumed_size; // take the size before we release it

    if(refcount_release(&page->refcount) == 0) {
        PGC_REFERENCED_PAGES_MINUS1(cache, assumed_size);

        if(evict_if_necessary)
            evict_on_page_release_when_permitted(cache);
    }
}

static ALWAYS_INLINE bool non_acquired_page_get_for_deletion___while_having_clean_locked(PGC *cache __maybe_unused, PGC_PAGE *page) {
    __atomic_add_fetch(&cache->stats.acquires_for_deletion, 1, __ATOMIC_RELAXED);

    internal_fatal(!is_page_clean(page),
                   "DBENGINE CACHE: only clean pages can be deleted");

    if(refcount_acquire_for_deletion(&page->refcount)) {
        // we can delete this page
        internal_fatal(page_flag_check(page, PGC_PAGE_IS_BEING_DELETED),
                       "DBENGINE CACHE: page is already being deleted");

        page_flag_set(page, PGC_PAGE_IS_BEING_DELETED);

        return true;
    }

    return false;
}

static ALWAYS_INLINE bool acquired_page_get_for_deletion_or_release_it(PGC *cache __maybe_unused, PGC_PAGE *page) {
    __atomic_add_fetch(&cache->stats.acquires_for_deletion, 1, __ATOMIC_RELAXED);

    int64_t assumed_size = page->assumed_size; // take the size before we release it

    if(refcount_release_and_acquire_for_deletion(&page->refcount)) {
        PGC_REFERENCED_PAGES_MINUS1(cache, assumed_size);

        // we can delete this page
        internal_fatal(page_flag_check(page, PGC_PAGE_IS_BEING_DELETED),
                       "DBENGINE CACHE: page is already being deleted");

        page_flag_set(page, PGC_PAGE_IS_BEING_DELETED);

        return true;
    }

    return false;
}


// ----------------------------------------------------------------------------
// Indexing

static inline void free_this_page(PGC *cache, PGC_PAGE *page, size_t partition __maybe_unused) {
    size_t size = page_size_from_assumed_size(cache, page->assumed_size);

    // call the callback to free the user supplied memory
    cache->config.pgc_free_clean_cb(cache, (PGC_ENTRY){
            .section = page->section,
            .metric_id = page->metric_id,
            .start_time_s = page->start_time_s,
            .end_time_s = __atomic_load_n(&page->end_time_s, __ATOMIC_RELAXED),
            .update_every_s = page->update_every_s,
            .size = size,
            .hot = (is_page_hot(page)) ? true : false,
            .data = page->data,
            .custom_data = (cache->config.additional_bytes_per_page) ? page->custom_data : NULL,
    });

    timing_dbengine_evict_step(TIMING_STEP_DBENGINE_EVICT_FREE_CB);

    // update statistics
    __atomic_add_fetch(&cache->stats.removed_entries, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&cache->stats.removed_size, page->assumed_size, __ATOMIC_RELAXED);

    __atomic_sub_fetch(&cache->stats.entries, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&cache->stats.size, page->assumed_size, __ATOMIC_RELAXED);

    timing_dbengine_evict_step(TIMING_STEP_DBENGINE_EVICT_FREE_ATOMICS2);

    // free our memory
#ifdef PGC_WITH_ARAL
    aral_freez(cache->index[partition].aral, page);
#else
    freez(page);
#endif

    timing_dbengine_evict_step(TIMING_STEP_DBENGINE_EVICT_FREE_ARAL);
}

static void remove_this_page_from_index_unsafe(PGC *cache, PGC_PAGE *page, size_t partition) {
    // remove it from the Judy arrays

    pointer_check(cache, page);

    internal_fatal(page_flag_check(page, PGC_PAGE_HOT | PGC_PAGE_DIRTY | PGC_PAGE_CLEAN),
                   "DBENGINE CACHE: page to be removed from the cache is still in the linked-list");

    internal_fatal(!page_flag_check(page, PGC_PAGE_IS_BEING_DELETED),
                   "DBENGINE CACHE: page to be removed from the index, is not marked for deletion");

    internal_fatal(partition != pgc_indexing_partition(cache, page->metric_id),
                   "DBENGINE CACHE: attempted to remove this page from the wrong partition of the cache");

    Pvoid_t *metrics_judy_pptr = JudyLGet(cache->index[partition].sections_judy, page->section, PJE0);
    if(unlikely(!metrics_judy_pptr))
        fatal("DBENGINE CACHE: section '%p' should exist, but it does not.", (void *)page->section);

    Pvoid_t *pages_judy_pptr = JudyLGet(*metrics_judy_pptr, page->metric_id, PJE0);
    if(unlikely(!pages_judy_pptr))
        fatal("DBENGINE CACHE: metric '%p' in section '%p' should exist, but it does not.",
              (void *)page->metric_id, (void *)page->section);

    Pvoid_t *page_ptr = JudyLGet(*pages_judy_pptr, page->start_time_s, PJE0);
    if(unlikely(!page_ptr))
        fatal("DBENGINE CACHE: page with start time '%ld' of metric '%p' in section '%p' should exist, but it does not.",
              page->start_time_s, (void *)page->metric_id, (void *)page->section);

    PGC_PAGE *found_page = *page_ptr;
    if(unlikely(found_page != page))
        fatal("DBENGINE CACHE: page with start time '%ld' of metric '%p' in section '%p' should exist, "
              "but the index returned a different address (expected %p, got %p).",
              page->start_time_s, (void *)page->metric_id, (void *)page->section,
              page, found_page);

    JudyAllocThreadPulseReset();

    if(unlikely(!JudyLDel(pages_judy_pptr, page->start_time_s, PJE0)))
        fatal("DBENGINE CACHE: page with start time '%ld' of metric '%p' in section '%p' exists, but cannot be deleted.",
              page->start_time_s, (void *)page->metric_id, (void *)page->section);

    if(!*pages_judy_pptr && !JudyLDel(metrics_judy_pptr, page->metric_id, PJE0))
        fatal("DBENGINE CACHE: metric '%p' in section '%p' exists and is empty, but cannot be deleted.",
              (void *)page->metric_id, (void *)page->section);

    if(!*metrics_judy_pptr && !JudyLDel(&cache->index[partition].sections_judy, page->section, PJE0))
        fatal("DBENGINE CACHE: section '%p' exists and is empty, but cannot be deleted.", (void *)page->section);

    pgc_stats_index_judy_change(cache, JudyAllocThreadPulseGetAndReset());

    pointer_del(cache, page);
}

static inline void remove_and_free_page_not_in_any_queue_and_acquired_for_deletion(PGC *cache, PGC_PAGE *page) {
    size_t partition = pgc_indexing_partition(cache, page->metric_id);
    pgc_index_write_lock(cache, partition);
    remove_this_page_from_index_unsafe(cache, page, partition);
    pgc_index_write_unlock(cache, partition);
    free_this_page(cache, page, partition);
}

static inline bool make_acquired_page_clean_and_evict_or_page_release(PGC *cache, PGC_PAGE *page) {
    pointer_check(cache, page);

    WAITQ_PRIORITY prio = is_page_clean(page) ? PGC_QUEUE_LOCK_PRIO_EVICTORS : PGC_QUEUE_LOCK_PRIO_COLLECTORS;

    page_transition_lock(cache, page);
    pgc_queue_lock(cache, &cache->clean, prio);

    // make it clean - it does not have any accesses, so it will be prepended
    page_set_clean(cache, page, true, true, prio);

    if(!acquired_page_get_for_deletion_or_release_it(cache, page)) {
        pgc_queue_unlock(cache, &cache->clean);
        page_transition_unlock(cache, page);
        return false;
    }

    // remove it from the linked list
    pgc_queue_del(cache, &cache->clean, page, true, prio);
    pgc_queue_unlock(cache, &cache->clean);
    page_transition_unlock(cache, page);

    remove_and_free_page_not_in_any_queue_and_acquired_for_deletion(cache, page);

    return true;
}

// returns true, when there is potentially more work to do
static bool evict_pages_with_filter(PGC *cache, size_t max_skip, size_t max_evict, bool wait, bool all_of_them, evict_filter filter, void *data) {
    ssize_t per1000 = cache_usage_per1000(cache, NULL);

    if(!all_of_them && per1000 < cache->config.healthy_size_per1000)
        // don't bother - not enough to do anything
        return false;

    bool under_sever_pressure = per1000 >= cache->config.severe_pressure_per1000;
    size_t workers_running = __atomic_add_fetch(&cache->stats.p0_workers_evict, 1, __ATOMIC_RELAXED);
    if(!wait && !all_of_them && workers_running > cache->config.max_workers_evict_inline && !under_sever_pressure) {
        __atomic_sub_fetch(&cache->stats.p0_workers_evict, 1, __ATOMIC_RELAXED);
        return false;
    }

    internal_fatal(cache->clean.linked_list_in_sections_judy,
                   "wrong clean pages configuration - clean pages need to have a linked list, not a judy array");

    if(unlikely(!max_skip))
        max_skip = SIZE_MAX;
    else if(unlikely(max_skip < 2))
        max_skip = 2;

    if(unlikely(!max_evict))
        max_evict = SIZE_MAX;
    else if(unlikely(max_evict < 2))
        max_evict = 2;

    size_t this_loop_evicted = 0;
    size_t total_pages_evicted = 0;
    size_t total_pages_relocated = 0;
    bool stopped_before_finishing = false;
    size_t spins = 0;
    size_t max_pages_to_evict = 0;

    do {
        int64_t max_size_to_evict = 0;
        if (unlikely(all_of_them)) {
            // evict them all
            max_size_to_evict = SIZE_MAX;
            max_pages_to_evict = SIZE_MAX;
            under_sever_pressure = true;
        }
        else if(unlikely(wait)) {
            // evict as many as necessary for the cache to go at the predefined threshold
            per1000 = cache_usage_per1000(cache, &max_size_to_evict);
            if(per1000 >= cache->config.severe_pressure_per1000) {
                under_sever_pressure = true;
                max_pages_to_evict = max_pages_to_evict ? max_pages_to_evict * 2 : 16;
                if(max_pages_to_evict > 64)
                    max_pages_to_evict = 64;
            }
            else if(per1000 >= cache->config.aggressive_evict_per1000) {
                under_sever_pressure = false;
                max_pages_to_evict = max_pages_to_evict ? max_pages_to_evict * 2 : 4;
                if(max_pages_to_evict > 16)
                    max_pages_to_evict = 16;
            }
            else {
                under_sever_pressure = false;
                max_pages_to_evict = 1;
            }
        }
        else {
            // this is an adder, so evict just 1 page
            max_size_to_evict = (cache_above_healthy_limit(cache)) ? 1 : 0;
            max_pages_to_evict = 1;
        }

        if (!max_size_to_evict || !max_pages_to_evict)
            break;

        // check if we have to stop
        if(total_pages_evicted >= max_evict && !all_of_them) {
            stopped_before_finishing = true;
            break;
        }

        if(++spins > 1 && !this_loop_evicted)
            p2_add_fetch(&cache->stats.p2_waste_evict_useless_spins, 1);

        this_loop_evicted = 0;

        timing_dbengine_evict_init();

        if(!all_of_them && !wait) {
            if(!pgc_queue_trylock(cache, &cache->clean, PGC_QUEUE_LOCK_PRIO_EVICTORS)) {
                stopped_before_finishing = true;
                goto premature_exit;
            }

            // at this point we have the clean lock
        }
        else
            pgc_queue_lock(cache, &cache->clean, PGC_QUEUE_LOCK_PRIO_EVICTORS);

        timing_dbengine_evict_step(TIMING_STEP_DBENGINE_EVICT_LOCK);

        // find a page to evict
        PGC_PAGE *pages_to_evict = NULL;
        int64_t pages_to_evict_size = 0;
        size_t pages_to_evict_count = 0;
        for(PGC_PAGE *page = cache->clean.base, *next = NULL, *first_page_we_relocated = NULL; page ; page = next) {
            next = page->link.next;

            if(unlikely(page == first_page_we_relocated))
                // we did a complete loop on all pages
                break;

            if(unlikely(page_flag_check(page, PGC_PAGE_HAS_BEEN_ACCESSED | PGC_PAGE_HAS_NO_DATA_IGNORE_ACCESSES) == PGC_PAGE_HAS_BEEN_ACCESSED)) {
                DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(cache->clean.base, page, link.prev, link.next);
                DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(cache->clean.base, page, link.prev, link.next);
                page_flag_clear(page, PGC_PAGE_HAS_BEEN_ACCESSED);
                continue;
            }

            if(unlikely(filter && !filter(page, data)))
                continue;

            if(non_acquired_page_get_for_deletion___while_having_clean_locked(cache, page)) {
                // we can delete this page

                // remove it from the clean list
                pgc_queue_del(cache, &cache->clean, page, true, PGC_QUEUE_LOCK_PRIO_EVICTORS);

                __atomic_add_fetch(&cache->stats.evicting_entries, 1, __ATOMIC_RELAXED);
                __atomic_add_fetch(&cache->stats.evicting_size, page->assumed_size, __ATOMIC_RELAXED);

                DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(pages_to_evict, page, link.prev, link.next);

                pages_to_evict_size += page->assumed_size;
                pages_to_evict_count++;

                timing_dbengine_evict_step(TIMING_STEP_DBENGINE_EVICT_SELECT_PAGE);

                if((pages_to_evict_count < max_pages_to_evict && pages_to_evict_size < max_size_to_evict) || all_of_them)
                    // get more pages
                    ;
                else
                    // one page at a time
                    break;
            }
            else {
                // we can't delete this page

                if(!first_page_we_relocated)
                    first_page_we_relocated = page;

                DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(cache->clean.base, page, link.prev, link.next);
                DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(cache->clean.base, page, link.prev, link.next);

                total_pages_relocated++;

                timing_dbengine_evict_step(TIMING_STEP_DBENGINE_EVICT_RELOCATE_PAGE);

                // check if we have to stop
                if(unlikely(total_pages_relocated >= max_skip && !all_of_them)) {
                    stopped_before_finishing = true;
                    break;
                }
            }
        }
        pgc_queue_unlock(cache, &cache->clean);

        timing_dbengine_evict_step(TIMING_STEP_DBENGINE_EVICT_SELECT);

        if(likely(pages_to_evict)) {
            // remove them from the index

            if(unlikely(pages_to_evict->link.next)) {
                // we have many pages, let's minimize the index locks we are going to get

                PGC_PAGE *pages_per_partition[cache->config.partitions];
                memset(pages_per_partition, 0, sizeof(PGC_PAGE *) * cache->config.partitions);

                bool partitions_done[cache->config.partitions];
                memset(partitions_done, 0, sizeof(bool) * cache->config.partitions);

                // sort them by partition
                for (PGC_PAGE *page = pages_to_evict, *next = NULL; page; page = next) {
                    next = page->link.next;

                    size_t partition = pgc_indexing_partition(cache, page->metric_id);
                    DOUBLE_LINKED_LIST_REMOVE_ITEM_UNSAFE(pages_to_evict, page, link.prev, link.next);
                    DOUBLE_LINKED_LIST_APPEND_ITEM_UNSAFE(pages_per_partition[partition], page, link.prev, link.next);
                }

                timing_dbengine_evict_step(TIMING_STEP_DBENGINE_EVICT_SORT);

                // remove them from the index
                size_t remaining_partitions = cache->config.partitions;
                size_t last_remaining_partitions = remaining_partitions + 1;
                while(remaining_partitions) {
                    bool force = remaining_partitions == last_remaining_partitions;
                    last_remaining_partitions = remaining_partitions;
                    remaining_partitions = 0;

                    for (size_t partition = 0; partition < cache->config.partitions; partition++) {
                        if (!pages_per_partition[partition] || partitions_done[partition])
                            continue;

                        if(pgc_index_trywrite_lock(cache, partition, force)) {
                            partitions_done[partition] = true;

                            for (PGC_PAGE *page = pages_per_partition[partition]; page; page = page->link.next)
                                remove_this_page_from_index_unsafe(cache, page, partition);

                            pgc_index_write_unlock(cache, partition);

                            timing_dbengine_evict_step(TIMING_STEP_DBENGINE_EVICT_DEINDEX_PAGE);
                        }
                        else
                            remaining_partitions++;
                    }
                }

                timing_dbengine_evict_step(TIMING_STEP_DBENGINE_EVICT_DEINDEX);

                // free them
                for (size_t partition = 0; partition < cache->config.partitions; partition++) {
                    if (!pages_per_partition[partition]) continue;

                    for (PGC_PAGE *page = pages_per_partition[partition], *next = NULL; page; page = next) {
                        next = page->link.next;

                        timing_dbengine_evict_step(TIMING_STEP_DBENGINE_EVICT_FREE_LOOP);

                        int64_t page_size = page->assumed_size;
                        free_this_page(cache, page, partition);

                        timing_dbengine_evict_step(TIMING_STEP_DBENGINE_EVICT_FREE_PAGE);

                        __atomic_sub_fetch(&cache->stats.evicting_entries, 1, __ATOMIC_RELAXED);
                        __atomic_sub_fetch(&cache->stats.evicting_size, page_size, __ATOMIC_RELAXED);

                        total_pages_evicted++;
                        this_loop_evicted++;

                        timing_dbengine_evict_step(TIMING_STEP_DBENGINE_EVICT_FREE_ATOMICS);
                    }
                }

                timing_dbengine_evict_report();
            }
            else {
                // just one page to be evicted
                PGC_PAGE *page = pages_to_evict;

                int64_t page_size = page->assumed_size;

                size_t partition = pgc_indexing_partition(cache, page->metric_id);
                pgc_index_write_lock(cache, partition);
                remove_this_page_from_index_unsafe(cache, page, partition);
                pgc_index_write_unlock(cache, partition);
                free_this_page(cache, page, partition);

                __atomic_sub_fetch(&cache->stats.evicting_entries, 1, __ATOMIC_RELAXED);
                __atomic_sub_fetch(&cache->stats.evicting_size, page_size, __ATOMIC_RELAXED);

                total_pages_evicted++;
                this_loop_evicted++;
            }
        }
        else
            break;

    } while(all_of_them || (total_pages_evicted < max_evict && total_pages_relocated < max_skip));

    if(all_of_them && !filter) {
        pgc_queue_lock(cache, &cache->clean, PGC_QUEUE_LOCK_PRIO_EVICTORS);
        size_t entries = __atomic_load_n(&cache->clean.stats->entries, __ATOMIC_RELAXED);
        if(entries) {
            nd_log_limit_static_global_var(erl, 1, 0);
            nd_log_limit(&erl, NDLS_DAEMON, NDLP_NOTICE,
                         "DBENGINE CACHE: cannot free all clean pages, %zu are still in the clean queue",
                         entries);
        }
        pgc_queue_unlock(cache, &cache->clean);
    }

premature_exit:
    if(unlikely(total_pages_relocated))
        p2_add_fetch(&cache->stats.p2_waste_evict_relocated, total_pages_relocated);

    __atomic_sub_fetch(&cache->stats.p0_workers_evict, 1, __ATOMIC_RELAXED);

    return stopped_before_finishing;
}

static PGC_PAGE *pgc_page_add(PGC *cache, PGC_ENTRY *entry, bool *added) {
    internal_fatal(entry->start_time_s < 0 || entry->end_time_s < 0,
                   "DBENGINE CACHE: timestamps are negative");

    p2_add_fetch(&cache->stats.p2_workers_add, 1);

    size_t partition = pgc_indexing_partition(cache, entry->metric_id);

#ifdef PGC_WITH_ARAL
    PGC_PAGE *allocation = aral_mallocz(cache->index[partition].aral);
#else
    PGC_PAGE *allocation = mallocz(sizeof(PGC_PAGE) + cache->config.additional_bytes_per_page);
#endif
    
    allocation->refcount = 1;
    allocation->accesses = (entry->hot) ? 0 : 1;
    allocation->flags = 0;
    allocation->section = entry->section;
    allocation->metric_id = entry->metric_id;
    allocation->start_time_s = entry->start_time_s;
    allocation->end_time_s = entry->end_time_s,
    allocation->update_every_s = entry->update_every_s,
    allocation->data = entry->data;
    allocation->assumed_size = page_assumed_size(cache, entry->size);
    spinlock_init(&allocation->transition_spinlock);
    allocation->link.prev = NULL;
    allocation->link.next = NULL;
    
    if(cache->config.additional_bytes_per_page) {
        if(entry->custom_data)
            memcpy(allocation->custom_data, entry->custom_data, cache->config.additional_bytes_per_page);
        else
            memset(allocation->custom_data, 0, cache->config.additional_bytes_per_page);
    }
    
    PGC_PAGE *page;
    size_t spins = 0;

    if(unlikely(entry->start_time_s < 0))
        entry->start_time_s = 0;

    if(unlikely(entry->end_time_s < 0))
        entry->end_time_s = 0;

    do {
        spins++;

        pgc_index_write_lock(cache, partition);

        JudyAllocThreadPulseReset();

        Pvoid_t *metrics_judy_pptr = JudyLIns(&cache->index[partition].sections_judy, entry->section, PJE0);
        if(unlikely(!metrics_judy_pptr || metrics_judy_pptr == PJERR))
            fatal("DBENGINE CACHE: JudyLIns(sections_judy, 0x%lx) failed, sections_judy = %p, result = %p",
                  (long unsigned)entry->section, cache->index[partition].sections_judy, metrics_judy_pptr);

        Pvoid_t *pages_judy_pptr = JudyLIns(metrics_judy_pptr, entry->metric_id, PJE0);
        if(unlikely(!pages_judy_pptr || pages_judy_pptr == PJERR))
            fatal("DBENGINE CACHE: JudyLIns(metrics_judy, 0x%lx) failed, metrics_judy = %p, result = %p",
                  (long unsigned)entry->metric_id, metrics_judy_pptr, pages_judy_pptr);

        Pvoid_t *page_ptr = JudyLIns(pages_judy_pptr, entry->start_time_s, PJE0);
        if(unlikely(!page_ptr || page_ptr == PJERR))
            fatal("DBENGINE CACHE: JudyLIns(pages_judy, %ld) failed, pages_judy = %p, result = %p",
                  (long)entry->start_time_s, pages_judy_pptr, page_ptr);

        pgc_stats_index_judy_change(cache, JudyAllocThreadPulseGetAndReset());

        page = *page_ptr;

        if (likely(!page)) {
            // consume it
            page = allocation;
            allocation = NULL;
            
            // put it in the index
            *page_ptr = page;
            pointer_add(cache, page);
            pgc_index_write_unlock(cache, partition);

            if (entry->hot)
                page_set_hot(cache, page, PGC_QUEUE_LOCK_PRIO_COLLECTORS);
            else
                page_set_clean(cache, page, false, false, PGC_QUEUE_LOCK_PRIO_EVICTORS);

            PGC_REFERENCED_PAGES_PLUS1(cache, page);

            // update statistics
            __atomic_add_fetch(&cache->stats.added_entries, 1, __ATOMIC_RELAXED);
            __atomic_add_fetch(&cache->stats.added_size, page->assumed_size, __ATOMIC_RELAXED);

            __atomic_add_fetch(&cache->stats.entries, 1, __ATOMIC_RELAXED);
            __atomic_add_fetch(&cache->stats.size, page->assumed_size, __ATOMIC_RELAXED);

            if(added)
                *added = true;
        }
        else {
            if (!page_acquire(cache, page))
                page = NULL;

            else if(added)
                *added = false;

            pgc_index_write_unlock(cache, partition);

            if(unlikely(!page)) {
                // now that we don't have the lock,
                // give it some time for the old page to go away
                tinysleep();
            }
        }

    } while(!page);

    if(allocation) {
#ifdef PGC_WITH_ARAL
        aral_freez(cache->index[partition].aral, allocation);
#else
        freez(allocation);
#endif
    }

    if(spins > 1)
        p2_add_fetch(&cache->stats.p2_waste_insert_spins, spins - 1);

    p2_sub_fetch(&cache->stats.p2_workers_add, 1);

    if(!entry->hot)
        evict_on_clean_page_added(cache);

    flush_on_page_add(cache);

    return page;
}

static ALWAYS_INLINE PGC_PAGE *page_find_and_acquire_exact_unsafe(PGC *cache, Pvoid_t *pages_judy_pptr, time_t start_time_s) {
    Pvoid_t *page_ptr = JudyLGet(*pages_judy_pptr, start_time_s, PJE0);
    if(!page_ptr)
        return NULL;

    if (unlikely(page_ptr == PJERR))
        fatal("DBENGINE CACHE: corrupted page in pages judy array");

    PGC_PAGE *page = *page_ptr;
    if(page && page_acquire(cache, page))
        // we have our page acquired
        return page;

    return NULL;
}

static ALWAYS_INLINE PGC_PAGE *page_find_and_acquire_first_unsafe(PGC *cache, Pvoid_t *pages_judy_pptr, time_t start_time_s) {
    Word_t time = start_time_s;
    for(Pvoid_t *page_ptr = JudyLFirst(*pages_judy_pptr, &time, PJE0);
         page_ptr ;
         page_ptr = JudyLNext(*pages_judy_pptr, &time, PJE0)) {

        if (unlikely(page_ptr == PJERR))
            fatal("DBENGINE CACHE: corrupted page in pages judy array");

        PGC_PAGE *page = *page_ptr;
        if(page && page_acquire(cache, page))
            // we have our page acquired
            return page;
    }

    return NULL;
}

static ALWAYS_INLINE PGC_PAGE *page_find_and_acquire_next_unsafe(PGC *cache, Pvoid_t *pages_judy_pptr, time_t start_time_s) {
    Word_t time = start_time_s;
    for(Pvoid_t *page_ptr = JudyLNext(*pages_judy_pptr, &time, PJE0);
         page_ptr ;
         page_ptr = JudyLNext(*pages_judy_pptr, &time, PJE0)) {

        if (unlikely(page_ptr == PJERR))
            fatal("DBENGINE CACHE: corrupted page in pages judy array");

        PGC_PAGE *page = *page_ptr;
        if(page && page_acquire(cache, page))
            // we have our page acquired
            return page;
    }

    return NULL;
}

static ALWAYS_INLINE PGC_PAGE *page_find_and_acquire_last_unsafe(PGC *cache, Pvoid_t *pages_judy_pptr, time_t start_time_s) {
    Word_t time = start_time_s;
    for(Pvoid_t *page_ptr = JudyLLast(*pages_judy_pptr, &time, PJE0);
         page_ptr ;
         page_ptr = JudyLPrev(*pages_judy_pptr, &time, PJE0)) {

        if (unlikely(page_ptr == PJERR))
            fatal("DBENGINE CACHE: corrupted page in pages judy array");

        PGC_PAGE *page = *page_ptr;
        if(page && page_acquire(cache, page))
            // we have our page acquired
            return page;
    }

    return NULL;
}

static ALWAYS_INLINE PGC_PAGE *page_find_and_acquire_prev_unsafe(PGC *cache, Pvoid_t *pages_judy_pptr, time_t start_time_s) {
    Word_t time = start_time_s;
    for(Pvoid_t *page_ptr = JudyLPrev(*pages_judy_pptr, &time, PJE0);
         page_ptr ;
         page_ptr = JudyLPrev(*pages_judy_pptr, &time, PJE0)) {

        if (unlikely(page_ptr == PJERR))
            fatal("DBENGINE CACHE: corrupted page in pages judy array");

        PGC_PAGE *page = *page_ptr;
        if(page && page_acquire(cache, page))
            // we have our page acquired
            return page;
    }

    return NULL;
}

static ALWAYS_INLINE PGC_PAGE *page_find_and_acquire_once(PGC *cache, Word_t section, Word_t metric_id, time_t start_time_s, PGC_SEARCH method) {
    PGC_PAGE *page = NULL;
    size_t partition = pgc_indexing_partition(cache, metric_id);

    pgc_index_read_lock(cache, partition);

    Pvoid_t *metrics_judy_pptr = JudyLGet(cache->index[partition].sections_judy, section, PJE0);
    if(unlikely(metrics_judy_pptr == PJERR))
        fatal("DBENGINE CACHE: corrupted sections judy array");

    if(unlikely(!metrics_judy_pptr)) {
        // section does not exist
        goto cleanup;
    }

    Pvoid_t *pages_judy_pptr = JudyLGet(*metrics_judy_pptr, metric_id, PJE0);
    if(unlikely(pages_judy_pptr == PJERR))
        fatal("DBENGINE CACHE: corrupted pages judy array");

    if(unlikely(!pages_judy_pptr)) {
        // metric does not exist
        goto cleanup;
    }

    switch(method) {
        default:
        case PGC_SEARCH_CLOSEST: {
            page = page_find_and_acquire_exact_unsafe(cache, pages_judy_pptr, start_time_s);
            if(!page) {
                page = page_find_and_acquire_prev_unsafe(cache, pages_judy_pptr, start_time_s);
                if(page && start_time_s > page->end_time_s) {
                    // found a page starting before our timestamp
                    // but our timestamp is not included in it
                    page_release(cache, page, false);
                    page = NULL;
                }

                if(!page)
                    page = page_find_and_acquire_next_unsafe(cache, pages_judy_pptr, start_time_s);
            }
        }
        break;

        case PGC_SEARCH_EXACT:
            page = page_find_and_acquire_exact_unsafe(cache, pages_judy_pptr, start_time_s);
            break;

        case PGC_SEARCH_FIRST:
            page = page_find_and_acquire_first_unsafe(cache, pages_judy_pptr, start_time_s);
            break;

        case PGC_SEARCH_NEXT:
            page = page_find_and_acquire_next_unsafe(cache, pages_judy_pptr, start_time_s);
            break;

        case PGC_SEARCH_LAST:
            page = page_find_and_acquire_last_unsafe(cache, pages_judy_pptr, start_time_s);
            break;

        case PGC_SEARCH_PREV:
            page = page_find_and_acquire_prev_unsafe(cache, pages_judy_pptr, start_time_s);
            break;
    }

#ifdef NETDATA_PGC_POINTER_CHECK
    if(page)
        pointer_check(cache, page);
#endif

cleanup:
    pgc_index_read_unlock(cache, partition);
    return page;
}

static void all_hot_pages_to_dirty(PGC *cache, Word_t section) {
    pgc_queue_lock(cache, &cache->hot, PGC_QUEUE_LOCK_PRIO_COLLECTORS);

    bool first = true;
    Word_t last_section = (section == PGC_SECTION_ALL) ? 0 : section;
    Pvoid_t *section_pages_pptr;
    while ((section_pages_pptr = JudyLFirstThenNext(cache->hot.sections_judy, &last_section, &first))) {
        if(section != PGC_SECTION_ALL && last_section != section)
            break;

        struct section_pages *sp = *section_pages_pptr;

        PGC_PAGE *page = sp->base;
        while(page) {
            PGC_PAGE *next = page->link.next;

            if(page_acquire(cache, page)) {
                page_set_dirty(cache, page, true, PGC_QUEUE_LOCK_PRIO_COLLECTORS);
                page_release(cache, page, false);
                // page ptr may be invalid now
            }

            page = next;
        }
    }
    pgc_queue_unlock(cache, &cache->hot);
}

// returns true when there is more work to do
static bool flush_pages(PGC *cache, size_t max_flushes, Word_t section, bool wait, bool all_of_them) {
    internal_fatal(!cache->dirty.linked_list_in_sections_judy,
                   "wrong dirty pages configuration - dirty pages need to have a judy array, not a linked list");

    if(!all_of_them && !wait) {
        // we have been called from a data collection thread
        // let's not waste its time...

        if(!pgc_queue_trylock(cache, &cache->dirty, PGC_QUEUE_LOCK_PRIO_FLUSHERS)) {
            // we would block, so give up...
            return false;
        }

        // we got the lock at this point
    }
    else
        pgc_queue_lock(cache, &cache->dirty, PGC_QUEUE_LOCK_PRIO_FLUSHERS);

    size_t optimal_flush_size = cache->config.max_dirty_pages_per_call;
    size_t dirty_version_at_entry = cache->dirty.version;
    size_t entries = __atomic_load_n(&cache->dirty.stats->entries, __ATOMIC_RELAXED);
    if(!all_of_them && (entries < optimal_flush_size || cache->dirty.last_version_checked == dirty_version_at_entry)) {
        pgc_queue_unlock(cache, &cache->dirty);
        return false;
    }

    p2_add_fetch(&cache->stats.p2_workers_flush, 1);

    bool have_dirty_lock = true;

    if(all_of_them || !max_flushes)
        max_flushes = SIZE_MAX;

    Word_t last_section = (section == PGC_SECTION_ALL) ? 0 : section;
    size_t flushes_so_far = 0;
    Pvoid_t *section_pages_pptr;
    bool stopped_before_finishing = false;
    bool first = true;

    while (have_dirty_lock && (section_pages_pptr = JudyLFirstThenNext(cache->dirty.sections_judy, &last_section, &first))) {
        if(section != PGC_SECTION_ALL && last_section != section)
            break;

        struct section_pages *sp = *section_pages_pptr;
        if(!all_of_them && sp->entries < optimal_flush_size)
            continue;

        if(!all_of_them && flushes_so_far > max_flushes) {
            stopped_before_finishing = true;
            break;
        }

        PGC_ENTRY array[optimal_flush_size];
        PGC_PAGE *pages[optimal_flush_size];

        size_t pages_added = 0,
               pages_removed_dirty = 0,
               pages_cancelled = 0,
               pages_made_clean = 0;

        int64_t pages_added_size = 0,
                pages_removed_dirty_size = 0,
                pages_cancelled_size = 0,
                pages_made_clean_size = 0;

        PGC_PAGE *page = sp->base;
        while (page && pages_added < optimal_flush_size) {
            PGC_PAGE *next = page->link.next;

            internal_fatal(page_get_status_flags(page) != PGC_PAGE_DIRTY,
                           "DBENGINE CACHE: page should be in the dirty list before saved");

            if (page_acquire(cache, page)) {
                internal_fatal(page_get_status_flags(page) != PGC_PAGE_DIRTY,
                               "DBENGINE CACHE: page should be in the dirty list before saved");

                internal_fatal(page->section != last_section,
                               "DBENGINE CACHE: dirty page is not in the right section (tier)");

                if(!page_transition_trylock(cache, page)) {
                    page_release(cache, page, false);
                    // page ptr may be invalid now
                }
                else {
                    pages[pages_added] = page;
                    array[pages_added] = (PGC_ENTRY) {
                            .section = page->section,
                            .metric_id = page->metric_id,
                            .start_time_s = page->start_time_s,
                            .end_time_s = __atomic_load_n(&page->end_time_s, __ATOMIC_RELAXED),
                            .update_every_s = page->update_every_s,
                            .size = page_size_from_assumed_size(cache, page->assumed_size),
                            .data = page->data,
                            .custom_data = (cache->config.additional_bytes_per_page) ? page->custom_data : NULL,
                            .hot = false,
                    };

                    pages_added_size += page->assumed_size;
                    pages_added++;
                }
            }

            page = next;
        }

        // do we have enough to save?
        if(all_of_them || pages_added == optimal_flush_size) {
            // we should do it

            for (size_t i = 0; i < pages_added; i++) {
                PGC_PAGE *tpg = pages[i];

                internal_fatal(page_get_status_flags(tpg) != PGC_PAGE_DIRTY,
                               "DBENGINE CACHE: page should be in the dirty list before saved");

                __atomic_add_fetch(&cache->stats.flushing_entries, 1, __ATOMIC_RELAXED);
                __atomic_add_fetch(&cache->stats.flushing_size, tpg->assumed_size, __ATOMIC_RELAXED);

                // remove it from the dirty list
                pgc_queue_del(cache, &cache->dirty, tpg, true, PGC_QUEUE_LOCK_PRIO_FLUSHERS);

                pages_removed_dirty_size += tpg->assumed_size;
                pages_removed_dirty++;
            }

            // next time, repeat the same section (tier)
            first = true;
        }
        else {
            // we can't do it

            for (size_t i = 0; i < pages_added; i++) {
                PGC_PAGE *tpg = pages[i];

                internal_fatal(page_get_status_flags(tpg) != PGC_PAGE_DIRTY,
                               "DBENGINE CACHE: page should be in the dirty list before saved");

                pages_cancelled_size += tpg->assumed_size;
                pages_cancelled++;

                page_transition_unlock(cache, tpg);
                page_release(cache, tpg, false);
                // page ptr may be invalid now
            }

            p2_add_fetch(&cache->stats.p2_waste_flushes_cancelled, pages_cancelled);
            p2_add_fetch(&cache->stats.flushes_cancelled_size, pages_cancelled_size);

            internal_fatal(pages_added != pages_cancelled || pages_added_size != pages_cancelled_size,
                           "DBENGINE CACHE: flushing cancel pages mismatch");

            // next time, continue to the next section (tier)
            first = false;
            continue;
        }

        if(cache->config.pgc_save_init_cb)
            cache->config.pgc_save_init_cb(cache, last_section);

        pgc_queue_unlock(cache, &cache->dirty);
        have_dirty_lock = false;

        // call the callback to save them
        // it may take some time, so let's release the lock
        if(cache->config.pgc_save_dirty_cb)
            cache->config.pgc_save_dirty_cb(cache, array, pages, pages_added);

        flushes_so_far++;

        __atomic_add_fetch(&cache->stats.flushes_completed, pages_added, __ATOMIC_RELAXED);
        __atomic_add_fetch(&cache->stats.flushes_completed_size, pages_added_size, __ATOMIC_RELAXED);

        size_t pages_to_evict = 0; (void)pages_to_evict;
        for (size_t i = 0; i < pages_added; i++) {
            PGC_PAGE *tpg = pages[i];

            internal_fatal(page_get_status_flags(tpg) != 0,
                           "DBENGINE CACHE: page should not be in any list while it is being saved");

            __atomic_sub_fetch(&cache->stats.flushing_entries, 1, __ATOMIC_RELAXED);
            __atomic_sub_fetch(&cache->stats.flushing_size, tpg->assumed_size, __ATOMIC_RELAXED);

            pages_made_clean_size += tpg->assumed_size;
            pages_made_clean++;

            if(!tpg->accesses)
                pages_to_evict++;

            page_set_clean(cache, tpg, true, false, PGC_QUEUE_LOCK_PRIO_FLUSHERS);
            page_transition_unlock(cache, tpg);
            page_release(cache, tpg, false);
            // tpg ptr may be invalid now
        }

        internal_fatal(pages_added != pages_made_clean || pages_added != pages_removed_dirty ||
                       pages_added_size != pages_made_clean_size || pages_added_size != pages_removed_dirty_size
                       , "DBENGINE CACHE: flushing pages mismatch");

        if(!all_of_them && !wait) {
            if(pgc_queue_trylock(cache, &cache->dirty, PGC_QUEUE_LOCK_PRIO_FLUSHERS))
                have_dirty_lock = true;

            else {
                stopped_before_finishing = true;
                have_dirty_lock = false;
            }
        }
        else {
            pgc_queue_lock(cache, &cache->dirty, PGC_QUEUE_LOCK_PRIO_FLUSHERS);
            have_dirty_lock = true;
        }
    }

    if(have_dirty_lock) {
        if(!stopped_before_finishing && dirty_version_at_entry > cache->dirty.last_version_checked)
            cache->dirty.last_version_checked = dirty_version_at_entry;

        pgc_queue_unlock(cache, &cache->dirty);
    }

    p2_sub_fetch(&cache->stats.p2_workers_flush, 1);

    return stopped_before_finishing;
}

void free_all_unreferenced_clean_pages(PGC *cache) {
    evict_pages(cache, 0, 0, true, true);
}

static void pgc_evict_thread(void *ptr) {
    static usec_t last_malloc_release_ut = 0;

    PGC *cache = ptr;

    worker_register("PGCEVICT");
    worker_register_job_name(0, "signaled");
    worker_register_job_name(1, "scheduled");
    worker_register_job_name(2, "cleanup");

    unsigned job_id = 0;

    while (true) {
        worker_is_idle();
        unsigned new_job_id = completion_wait_for_a_job_with_timeout(
            &cache->evictor.completion, job_id, 1000);

        worker_is_busy(new_job_id > job_id ? 1 : 0);
        job_id = new_job_id;

        if (nd_thread_signaled_to_cancel())
            break;

        int64_t size_to_evict = 0;
        bool system_cleanup = false;
        if(cache_usage_per1000(cache, &size_to_evict) > cache->config.aggressive_evict_per1000)
            system_cleanup = true;

        evict_pages(cache, 0, 0, true, false);

        if(system_cleanup) {
            usec_t now_ut = now_monotonic_usec();

            if(__atomic_load_n(&last_malloc_release_ut, __ATOMIC_RELAXED) + USEC_PER_SEC <= now_ut) {
                __atomic_store_n(&last_malloc_release_ut, now_ut, __ATOMIC_RELAXED);
                worker_is_busy(2);
                mallocz_release_as_much_memory_to_the_system();
            }
        }
    }

    worker_unregister();
}

// ----------------------------------------------------------------------------
// public API

PGC *pgc_create(const char *name,
                size_t clean_size_bytes,
                free_clean_page_callback pgc_free_cb,
                size_t max_dirty_pages_per_flush,
                save_dirty_init_callback pgc_save_init_cb,
                save_dirty_page_callback pgc_save_dirty_cb,
                size_t max_pages_per_inline_eviction,
                size_t max_inline_evictors,
                size_t max_skip_pages_per_inline_eviction,
                size_t max_flushes_inline,
                PGC_OPTIONS options,
                size_t partitions,
                size_t additional_bytes_per_page) {

    if(max_pages_per_inline_eviction < 1)
        max_pages_per_inline_eviction = 1;

    if(max_dirty_pages_per_flush < 1)
        max_dirty_pages_per_flush = 1;

    if(max_flushes_inline * max_dirty_pages_per_flush < 2)
        max_flushes_inline = 2;

    PGC *cache = callocz(1, sizeof(PGC));
    strncpyz(cache->config.name, name, PGC_NAME_MAX);

    cache->config.options = options;
    cache->config.additional_bytes_per_page = additional_bytes_per_page;
    cache->config.stats = pulse_enabled;

    // flushing
    cache->config.max_flushes_inline            = (max_flushes_inline == 0) ? 2 : max_flushes_inline;
    cache->config.max_dirty_pages_per_call      = max_dirty_pages_per_flush;
    cache->config.pgc_save_init_cb              = pgc_save_init_cb;
    cache->config.pgc_save_dirty_cb             = pgc_save_dirty_cb;

    // eviction strategy
    cache->config.clean_size                    = (clean_size_bytes < 1 * 1024 * 1024) ? 1 * 1024 * 1024 : (int64_t)clean_size_bytes;
    cache->config.pgc_free_clean_cb             = pgc_free_cb;
    cache->config.max_workers_evict_inline      = max_inline_evictors;
    cache->config.max_pages_per_inline_eviction = max_pages_per_inline_eviction;
    cache->config.max_skip_pages_per_inline_eviction = (max_skip_pages_per_inline_eviction < 2) ? 2 : max_skip_pages_per_inline_eviction;
    cache->config.severe_pressure_per1000       = 1010; // INLINE: use releasers to evict pages (up to max_pages_per_inline_eviction)
    cache->config.aggressive_evict_per1000      =  990; // INLINE: use adders to evict pages (up to max_pages_per_inline_eviction)
    cache->config.healthy_size_per1000          =  980; // no evictions happen below this threshold
    cache->config.evict_low_threshold_per1000   =  970; // when evicting, bring the size down to this threshold
                                                        // the eviction thread is signaled ONLY if we run out of memory
                                                        // otherwise, it runs by itself every 100ms

    // use all ram and protection from out of memory
    cache->config.use_all_ram                       = dbengine_use_all_ram_for_caches;
    cache->config.out_of_memory_protection_bytes    = (int64_t)dbengine_out_of_memory_protection;

    // partitions
    if(partitions == 0) partitions  = netdata_conf_cpus() * 2;
    if(partitions <= 4) partitions  = 4;
    if(partitions > 256) partitions = 256;
    cache->config.partitions        = partitions;
    cache->index                    = callocz(cache->config.partitions, sizeof(struct pgc_index));

    pgc_section_pages_static_aral_init();

    for(size_t part = 0; part < cache->config.partitions ; part++) {
        rw_spinlock_init(&cache->index[part].rw_spinlock);
#ifdef PGC_WITH_ARAL
        {
            char buf[100];
            snprintfz(buf, sizeof(buf), "%s", name);
            cache->index[part].aral = aral_create(
                buf,
                sizeof(PGC_PAGE) + cache->config.additional_bytes_per_page,
                0,
                0,
                &pgc_aral_statistics,
                NULL, NULL,
                false, false, false);
        }
#endif
    }


#if defined(PGC_QUEUE_LOCK_AS_WAITING_QUEUE)
    waitq_init(&cache->hot.wq);
    waitq_init(&cache->dirty.wq);
    waitq_init(&cache->clean.wq);
#else
    spinlock_init(&cache->hot.spinlock);
    spinlock_init(&cache->dirty.spinlock);
    spinlock_init(&cache->clean.spinlock);
#endif

    cache->hot.flags = PGC_PAGE_HOT;
    cache->hot.linked_list_in_sections_judy = true;
    cache->hot.stats = &cache->stats.queues[PGC_QUEUE_HOT];

    cache->dirty.flags = PGC_PAGE_DIRTY;
    cache->dirty.linked_list_in_sections_judy = true;
    cache->dirty.stats = &cache->stats.queues[PGC_QUEUE_DIRTY];

    cache->clean.flags = PGC_PAGE_CLEAN;
    cache->clean.linked_list_in_sections_judy = false;
    cache->clean.stats = &cache->stats.queues[PGC_QUEUE_CLEAN];

    pointer_index_init(cache);
    pgc_size_histogram_init(&cache->hot.stats->size_histogram);
    pgc_size_histogram_init(&cache->dirty.stats->size_histogram);
    pgc_size_histogram_init(&cache->clean.stats->size_histogram);

    // last create the eviction thread
    {
        completion_init(&cache->evictor.completion);
        cache->evictor.thread = nd_thread_create(name, NETDATA_THREAD_OPTION_DEFAULT, pgc_evict_thread, cache);
    }

    return cache;
}

struct aral_statistics *pgc_aral_stats(void) {
    return &pgc_aral_statistics;
}

void pgc_flush_dirty_pages(PGC *cache, Word_t section) {
    flush_pages(cache, 0, section, true, true);
}

void pgc_flush_all_hot_and_dirty_pages(PGC *cache, Word_t section) {
    all_hot_pages_to_dirty(cache, section);

    // save all dirty pages to make them clean
    flush_pages(cache, 0, section, true, true);
}

void pgc_destroy(PGC *cache, bool flush) {
    if(!cache)
        return;

    if(!flush) {
        cache->config.pgc_save_init_cb = NULL;
        cache->config.pgc_save_dirty_cb = NULL;
    }

    // convert all hot pages to dirty
    all_hot_pages_to_dirty(cache, PGC_SECTION_ALL);

    // save all dirty pages to make them clean
    flush_pages(cache, 0, PGC_SECTION_ALL, true, true);

    // free all unreferenced clean pages
    free_all_unreferenced_clean_pages(cache);

    // stop the eviction thread
    nd_thread_signal_cancel(cache->evictor.thread);
    completion_mark_complete_a_job(&cache->evictor.completion);
    nd_thread_join(cache->evictor.thread);
    completion_destroy(&cache->evictor.completion);

    if(PGC_REFERENCED_PAGES(cache))
        netdata_log_error("DBENGINE CACHE: there are %zu referenced cache pages - leaving the cache allocated", PGC_REFERENCED_PAGES(cache));
    else {
        pointer_destroy_index(cache);

        for(size_t part = 0; part < cache->config.partitions ;part++) {
            //  netdata_rwlock_destroy(&cache->index[part].rw_spinlock);
#ifdef PGC_WITH_ARAL
            aral_destroy(cache->index[part].aral);
#endif
        }

#if defined(PGC_QUEUE_LOCK_AS_WAITING_QUEUE)
        waitq_destroy(&cache->hot.wq);
        waitq_destroy(&cache->dirty.wq);
        waitq_destroy(&cache->clean.wq);
#endif
        freez(cache->index);
        freez(cache);
    }
}

ALWAYS_INLINE PGC_PAGE *pgc_page_add_and_acquire(PGC *cache, PGC_ENTRY entry, bool *added) {
    return pgc_page_add(cache, &entry, added);
}

ALWAYS_INLINE PGC_PAGE *pgc_page_dup(PGC *cache, PGC_PAGE *page) {
    if(!page_acquire(cache, page))
        fatal("DBENGINE CACHE: tried to dup a page that is not acquired!");

    return page;
}

ALWAYS_INLINE void pgc_page_release(PGC *cache, PGC_PAGE *page) {
    page_release(cache, page, is_page_clean(page));
}

ALWAYS_INLINE void pgc_page_hot_to_dirty_and_release(PGC *cache, PGC_PAGE *page, bool never_flush) {
    p2_add_fetch(&cache->stats.p2_workers_hot2dirty, 1);

//#ifdef NETDATA_INTERNAL_CHECKS
//    page_transition_lock(cache, page);
//    internal_fatal(!is_page_hot(page), "DBENGINE CACHE: called %s() but page is not hot", __FUNCTION__ );
//    page_transition_unlock(cache, page);
//#endif

    // make page dirty
    page_set_dirty(cache, page, false, PGC_QUEUE_LOCK_PRIO_COLLECTORS);

    // release the page
    page_release(cache, page, true);
    // page ptr may be invalid now

    p2_sub_fetch(&cache->stats.p2_workers_hot2dirty, 1);

    // flush, if we have to
    if(!never_flush)
        flush_on_page_hot_release(cache);
}

bool pgc_page_to_clean_evict_or_release(PGC *cache, PGC_PAGE *page) {
    bool ret;

    p2_add_fetch(&cache->stats.p2_workers_hot2dirty, 1);

    // prevent accesses from increasing the accesses counter
    page_flag_set(page, PGC_PAGE_HAS_NO_DATA_IGNORE_ACCESSES);

    // zero the accesses counter
    __atomic_store_n(&page->accesses, 0, __ATOMIC_RELEASE);

    // if there are no other references to it, evict it immediately
    if(make_acquired_page_clean_and_evict_or_page_release(cache, page)) {
        __atomic_add_fetch(&cache->stats.hot_empty_pages_evicted_immediately, 1, __ATOMIC_RELAXED);
        ret = true;
    }
    else {
        __atomic_add_fetch(&cache->stats.hot_empty_pages_evicted_later, 1, __ATOMIC_RELAXED);
        ret = false;
    }

    p2_sub_fetch(&cache->stats.p2_workers_hot2dirty, 1);

    return ret;
}

Word_t pgc_page_section(PGC_PAGE *page) {
    return page->section;
}

Word_t pgc_page_metric(PGC_PAGE *page) {
    return page->metric_id;
}

time_t pgc_page_start_time_s(PGC_PAGE *page) {
    return page->start_time_s;
}

time_t pgc_page_end_time_s(PGC_PAGE *page) {
    return page->end_time_s;
}

uint32_t pgc_page_update_every_s(PGC_PAGE *page) {
    return page->update_every_s;
}

uint32_t pgc_page_fix_update_every(PGC_PAGE *page, uint32_t update_every_s) {
    if(page->update_every_s == 0)
        page->update_every_s = update_every_s;

    return page->update_every_s;
}

time_t pgc_page_fix_end_time_s(PGC_PAGE *page, time_t end_time_s) {
    page->end_time_s = end_time_s;
    return page->end_time_s;
}

void *pgc_page_data(PGC_PAGE *page) {
    return page->data;
}

void *pgc_page_custom_data(PGC *cache, PGC_PAGE *page) {
    if(cache->config.additional_bytes_per_page)
        return page->custom_data;

    return NULL;
}

size_t pgc_page_data_size(PGC *cache, PGC_PAGE *page) {
    return page_size_from_assumed_size(cache, page->assumed_size);
}

bool pgc_is_page_hot(PGC_PAGE *page) {
    return is_page_hot(page);
}

bool pgc_is_page_dirty(PGC_PAGE *page) {
    return is_page_dirty(page);
}

bool pgc_is_page_clean(PGC_PAGE *page) {
    return is_page_clean(page);
}

void pgc_reset_hot_max(PGC *cache) {
    size_t entries = __atomic_load_n(&cache->hot.stats->entries, __ATOMIC_RELAXED);
    int64_t size = __atomic_load_n(&cache->hot.stats->size, __ATOMIC_RELAXED);

    __atomic_store_n(&cache->hot.stats->max_entries, entries, __ATOMIC_RELAXED);
    __atomic_store_n(&cache->hot.stats->max_size, size, __ATOMIC_RELAXED);

    int64_t size_to_evict = 0;
    cache_usage_per1000(cache, &size_to_evict);
    evict_pages(cache, 0, 0, true, false);
}

void pgc_set_dynamic_target_cache_size_callback(PGC *cache, dynamic_target_cache_size_callback callback) {
    cache->config.dynamic_target_size_cb = callback;
    cache->config.out_of_memory_protection_bytes = 0;
    cache->config.use_all_ram = false;

    int64_t size_to_evict = 0;
    cache_usage_per1000(cache, &size_to_evict);
    evict_pages(cache, 0, 0, true, false);
}

void pgc_set_nominal_page_size_callback(PGC *cache, nominal_page_size_callback callback) {
    cache->config.nominal_page_size_cb = callback;
}

int64_t pgc_get_current_cache_size(PGC *cache) {
    return __atomic_load_n(&cache->stats.current_cache_size, __ATOMIC_RELAXED);
}

int64_t pgc_get_wanted_cache_size(PGC *cache) {
    return __atomic_load_n(&cache->stats.wanted_cache_size, __ATOMIC_RELAXED);
}

bool pgc_evict_pages(PGC *cache, size_t max_skip, size_t max_evict) {
    bool under_pressure = cache_needs_space_aggressively(cache);
    return evict_pages(cache,
                       under_pressure ? 0 : max_skip,
                       under_pressure ? 0 : max_evict,
                       true, false);
}

bool pgc_flush_pages(PGC *cache) {
    return flush_pages(cache, 0, PGC_SECTION_ALL, true, false);
}

void pgc_page_hot_set_end_time_s(PGC *cache __maybe_unused, PGC_PAGE *page, time_t end_time_s, size_t additional_bytes) {
    internal_fatal(!is_page_hot(page) && !exit_initiated_get(),
                   "DBENGINE CACHE: end_time_s update on non-hot page");

    internal_fatal(end_time_s < __atomic_load_n(&page->end_time_s, __ATOMIC_RELAXED),
                   "DBENGINE CACHE: end_time_s is not bigger than existing");

    __atomic_store_n(&page->end_time_s, end_time_s, __ATOMIC_RELAXED);

    if(additional_bytes) {
        page_transition_lock(cache, page);

        struct pgc_queue_statistics *queue_stats = NULL;
        if(page->flags & PGC_PAGE_HOT)
            queue_stats = cache->hot.stats;
        else if(page->flags & PGC_PAGE_DIRTY)
            queue_stats = cache->dirty.stats;
        else if(page->flags & PGC_PAGE_CLEAN)
            queue_stats = cache->clean.stats;

        if(queue_stats && cache->config.stats)
            pgc_size_histogram_del(cache, &queue_stats->size_histogram, page);

        int64_t old_assumed_size = page->assumed_size;

        size_t old_size = page_size_from_assumed_size(cache, old_assumed_size);
        size_t size = old_size + additional_bytes;
        page->assumed_size = page_assumed_size(cache, size);

        int64_t delta = page->assumed_size - old_assumed_size;
        __atomic_add_fetch(&cache->stats.size, delta, __ATOMIC_RELAXED);
        __atomic_add_fetch(&cache->stats.added_size, delta, __ATOMIC_RELAXED);
        __atomic_add_fetch(&cache->stats.referenced_size, delta, __ATOMIC_RELAXED);

        if(queue_stats) {
            __atomic_add_fetch(&queue_stats->size, delta, __ATOMIC_RELAXED);
            __atomic_add_fetch(&queue_stats->added_size, delta, __ATOMIC_RELAXED);

            if(cache->config.stats)
                pgc_size_histogram_add(cache, &queue_stats->size_histogram, page);
        }

        page_transition_unlock(cache, page);
    }

#ifdef PGC_COUNT_POINTS_COLLECTED
    __atomic_add_fetch(&cache->stats.points_collected, 1, __ATOMIC_RELAXED);
#endif
}

PGC_PAGE *pgc_page_get_and_acquire(PGC *cache, Word_t section, Word_t metric_id, time_t start_time_s, PGC_SEARCH method) {
    PGC_PAGE *page = NULL;

    p2_add_fetch(&cache->stats.p2_workers_search, 1);

    size_t *stats_hit_ptr, *stats_miss_ptr;

    if(method == PGC_SEARCH_CLOSEST) {
        __atomic_add_fetch(&cache->stats.searches_closest, 1, __ATOMIC_RELAXED);
        stats_hit_ptr = &cache->stats.searches_closest_hits;
        stats_miss_ptr = &cache->stats.searches_closest_misses;
    }
    else {
        __atomic_add_fetch(&cache->stats.searches_exact, 1, __ATOMIC_RELAXED);
        stats_hit_ptr = &cache->stats.searches_exact_hits;
        stats_miss_ptr = &cache->stats.searches_exact_misses;
    }

    page = page_find_and_acquire_once(cache, section, metric_id, start_time_s, method);
    if(page) {
        __atomic_add_fetch(stats_hit_ptr, 1, __ATOMIC_RELAXED);
        page_has_been_accessed(cache, page);
    }
    else
        __atomic_add_fetch(stats_miss_ptr, 1, __ATOMIC_RELAXED);

    p2_sub_fetch(&cache->stats.p2_workers_search, 1);

    return page;
}

struct pgc_statistics pgc_get_statistics(PGC *cache) {
    // FIXME - get the statistics atomically
    return cache->stats;
}

size_t pgc_hot_and_dirty_entries(PGC *cache) {
    size_t entries = 0;

    entries += __atomic_load_n(&cache->hot.stats->entries, __ATOMIC_RELAXED);
    entries += __atomic_load_n(&cache->dirty.stats->entries, __ATOMIC_RELAXED);
    entries += __atomic_load_n(&cache->stats.flushing_entries, __ATOMIC_RELAXED);
    entries += __atomic_load_n(&cache->stats.hot2dirty_entries, __ATOMIC_RELAXED);

    return entries;
}

void pgc_open_cache_to_journal_v2(
    PGC *cache,
    Word_t section,
    unsigned datafile_fileno,
    uint8_t type,
    migrate_to_v2_callback cb,
    void *data,
    bool startup)
{
    __atomic_add_fetch(&rrdeng_cache_efficiency_stats.journal_v2_indexing_started, 1, __ATOMIC_RELAXED);
    p2_add_fetch(&cache->stats.p2_workers_jv2_flush, 1);

    pgc_queue_lock(cache, &cache->hot, PGC_QUEUE_LOCK_PRIO_LOW);

    Pvoid_t JudyL_metrics = NULL;
    Pvoid_t JudyL_extents_pos = NULL;

    size_t count_of_unique_extents = 0;
    size_t count_of_unique_metrics = 0;
    size_t count_of_unique_pages = 0;

    size_t master_extent_index_id = 0;

    Pvoid_t *section_pages_pptr = JudyLGet(cache->hot.sections_judy, section, PJE0);
    if(!section_pages_pptr) {
        pgc_queue_unlock(cache, &cache->hot);
        return;
    }

    struct section_pages *sp = *section_pages_pptr;
    if(!spinlock_trylock(&sp->migration_to_v2_spinlock)) {
        netdata_log_info("DBENGINE: migration to journal v2 for datafile %u is postponed, another jv2 indexer is already running for this section", datafile_fileno);
        pgc_queue_unlock(cache, &cache->hot);
        return;
    }

    ARAL *ar_mi = aral_by_size_acquire(sizeof(struct jv2_metrics_info));
    ARAL *ar_pi = aral_by_size_acquire(sizeof(struct jv2_page_info));
    ARAL *ar_ei = aral_by_size_acquire(sizeof(struct jv2_extents_info));

    for(PGC_PAGE *page = sp->base; page ; page = page->link.next) {
        struct extent_io_data *xio = (struct extent_io_data *)page->custom_data;
        if(xio->fileno != datafile_fileno) continue;

        if(page_flag_check(page, PGC_PAGE_IS_BEING_MIGRATED_TO_V2)) {
            internal_fatal(true, "Migration to journal v2: page has already been migrated to v2");
            continue;
        }

        if(!page_transition_trylock(cache, page)) {
            internal_fatal(true, "Migration to journal v2: cannot get page transition lock");
            continue;
        }

        if(!page_acquire(cache, page)) {
            internal_fatal(true, "Migration to journal v2: cannot acquire page for migration to v2");
            page_transition_unlock(cache, page);
            continue;
        }

        page_flag_set(page, PGC_PAGE_IS_BEING_MIGRATED_TO_V2);

        pgc_queue_unlock(cache, &cache->hot);

        // update the extents JudyL

        size_t current_extent_index_id;
        Pvoid_t *PValue = JudyLIns(&JudyL_extents_pos, xio->block, PJE0);
        if(!PValue || PValue == PJERR)
            fatal("CACHE: JudyLIns(JudyL_extents_pos, %" PRIu64 ") failed, JudyL_extents_pos = %p, result = %p",
                  BLOCK_TO_OFFSET(xio->block), JudyL_extents_pos, PValue);

        struct jv2_extents_info *ei;
        if(!*PValue) {
            ei = aral_mallocz(ar_ei); // callocz(1, sizeof(struct jv2_extents_info));
            ei->block = xio->block;
            ei->bytes = xio->bytes;
            ei->number_of_pages = 1;
            ei->index = master_extent_index_id++;
            *PValue = ei;

            count_of_unique_extents++;
        }
        else {
            ei = *PValue;
            ei->number_of_pages++;
        }

        current_extent_index_id = ei->index;

        // update the metrics JudyL

        PValue = JudyLIns(&JudyL_metrics, page->metric_id, PJE0);
        if(!PValue || PValue == PJERR)
            fatal("CACHE: JudyLIns(JudyL_metrics, 0x%lx) failed, JudyL_metrics = %p, result = %p",
                  (long unsigned)page->metric_id, JudyL_metrics, PValue);

        struct jv2_metrics_info *mi;
        if(!*PValue) {
            mi = aral_mallocz(ar_mi); // callocz(1, sizeof(struct jv2_metrics_info));
            mi->uuid = mrg_metric_uuid(main_mrg, (METRIC *)page->metric_id);
            mi->first_time_s = page->start_time_s;
            mi->last_time_s = page->end_time_s;
            mi->number_of_pages = 1;
            mi->page_list_header = 0;
            mi->JudyL_pages_by_start_time = NULL;
            *PValue = mi;

            count_of_unique_metrics++;
        }
        else {
            mi = *PValue;
            mi->number_of_pages++;
            if(page->start_time_s < mi->first_time_s)
                mi->first_time_s = page->start_time_s;
            if(page->end_time_s > mi->last_time_s)
                mi->last_time_s = page->end_time_s;
        }

        PValue = JudyLIns(&mi->JudyL_pages_by_start_time, page->start_time_s, PJE0);
        if(!PValue || PValue == PJERR)
            fatal("CACHE: JudyLIns(JudyL_pages_by_start_time, %ld) failed, JudyL_pages_by_start_time = %p, result = %p",
                  (long)page->start_time_s, mi->JudyL_pages_by_start_time, PValue);

        if(!*PValue) {
            struct jv2_page_info *pi = aral_mallocz(ar_pi); // callocz(1, (sizeof(struct jv2_page_info)));
            pi->start_time_s = page->start_time_s;
            pi->end_time_s = page->end_time_s;
            pi->update_every_s = page->update_every_s;
            pi->page_length = page_size_from_assumed_size(cache, page->assumed_size);
            pi->page = page;
            pi->extent_index = current_extent_index_id;
            pi->custom_data = (cache->config.additional_bytes_per_page) ? page->custom_data : NULL;
            *PValue = pi;

            count_of_unique_pages++;
        }
        else {
            // impossible situation
            internal_fatal(true, "Page is already in JudyL metric pages");
            page_flag_clear(page, PGC_PAGE_IS_BEING_MIGRATED_TO_V2);
            page_transition_unlock(cache, page);
            page_release(cache, page, false);
        }

        if (likely(false == startup))
            yield_the_processor(); // do not lock too aggressively
        pgc_queue_lock(cache, &cache->hot, PGC_QUEUE_LOCK_PRIO_LOW);
    }

    spinlock_unlock(&sp->migration_to_v2_spinlock);
    pgc_queue_unlock(cache, &cache->hot);

    // callback
    bool success = cb(section, datafile_fileno, type, JudyL_metrics, JudyL_extents_pos, count_of_unique_extents, count_of_unique_metrics, count_of_unique_pages, data);

    {
        Pvoid_t *PValue1;
        bool metric_id_first = true;
        Word_t metric_id = 0;
        while ((PValue1 = JudyLFirstThenNext(JudyL_metrics, &metric_id, &metric_id_first))) {
            struct jv2_metrics_info *mi = *PValue1;

            Pvoid_t *PValue2;
            bool start_time_first = true;
            Word_t start_time = 0;
            while ((PValue2 = JudyLFirstThenNext(mi->JudyL_pages_by_start_time, &start_time, &start_time_first))) {
                struct jv2_page_info *pi = *PValue2;

                if (likely(false == startup))
                    yield_the_processor(); // do not lock too aggressively
                if (likely(success))
                    page_set_clean(cache, pi->page, true, false, PGC_QUEUE_LOCK_PRIO_LOW);
                else
                    page_flag_clear(pi->page, PGC_PAGE_IS_BEING_MIGRATED_TO_V2);

                page_transition_unlock(cache, pi->page);
                page_release(cache, pi->page, success);
                // before balance-parents:
                // page_transition_unlock(cache, pi->page);
                // pgc_page_hot_to_dirty_and_release(cache, pi->page, true);

                // old test - don't enable:
                // make_acquired_page_clean_and_evict_or_page_release(cache, pi->page);
                aral_freez(ar_pi, pi);
            }

            JudyLFreeArray(&mi->JudyL_pages_by_start_time, PJE0);
            aral_freez(ar_mi, mi);
        }
        JudyLFreeArray(&JudyL_metrics, PJE0);
    }

    {
        Pvoid_t *PValue;
        bool extent_pos_first = true;
        Word_t extent_pos = 0;
        while ((PValue = JudyLFirstThenNext(JudyL_extents_pos, &extent_pos, &extent_pos_first))) {
            struct jv2_extents_info *ei = *PValue;
            aral_freez(ar_ei, ei);
        }
        JudyLFreeArray(&JudyL_extents_pos, PJE0);
    }

    aral_by_size_release(ar_ei);
    aral_by_size_release(ar_pi);
    aral_by_size_release(ar_mi);

    p2_sub_fetch(&cache->stats.p2_workers_jv2_flush, 1);

    // balance-parents: do not flush, there is nothing dirty
    // flush_pages(cache, cache->config.max_flushes_inline, PGC_SECTION_ALL, false, false);
}

static bool match_page_data(PGC_PAGE *page, void *data) {
    return (page->data == data);
}

void pgc_open_evict_clean_pages_of_datafile(PGC *cache, struct rrdengine_datafile *datafile) {
    evict_pages_with_filter(cache, 0, 0, true, true, match_page_data, datafile);
}

size_t pgc_count_clean_pages_having_data_ptr(PGC *cache, Word_t section, void *ptr) {
    size_t found = 0;

    pgc_queue_lock(cache, &cache->clean, PGC_QUEUE_LOCK_PRIO_LOW);
    for(PGC_PAGE *page = cache->clean.base; page ;page = page->link.next)
        found += (page->data == ptr && page->section == section) ? 1 : 0;
    pgc_queue_unlock(cache, &cache->clean);

    return found;
}

size_t pgc_count_hot_pages_having_data_ptr(PGC *cache, Word_t section, void *ptr) {
    size_t found = 0;

    pgc_queue_lock(cache, &cache->hot, PGC_QUEUE_LOCK_PRIO_LOW);
    Pvoid_t *section_pages_pptr = JudyLGet(cache->hot.sections_judy, section, PJE0);
    if(section_pages_pptr) {
        struct section_pages *sp = *section_pages_pptr;
        for(PGC_PAGE *page = sp->base; page ;page = page->link.next)
            found += (page->data == ptr) ? 1 : 0;
    }
    pgc_queue_unlock(cache, &cache->hot);

    return found;
}

// ----------------------------------------------------------------------------
// unittest

static void unittest_free_clean_page_callback(PGC *cache __maybe_unused, PGC_ENTRY entry __maybe_unused) {
    ;
}

static void unittest_save_dirty_page_callback(PGC *cache __maybe_unused, PGC_ENTRY *entries_array __maybe_unused, PGC_PAGE **pages_array __maybe_unused, size_t entries __maybe_unused) {
    ;
}

#ifdef PGC_STRESS_TEST

struct {
    bool stop;
    PGC *cache;
    PGC_PAGE **metrics;
    size_t clean_metrics;
    size_t hot_metrics;
    time_t first_time_t;
    time_t last_time_t;
    size_t cache_size;
    size_t query_threads;
    size_t collect_threads;
    size_t partitions;
    size_t points_per_page;
    time_t time_per_collection_ut;
    time_t time_per_query_ut;
    time_t time_per_flush_ut;
    PGC_OPTIONS options;
    char rand_statebufs[1024];
    struct random_data *random_data;
} pgc_uts = {
        .stop            = false,
        .metrics         = NULL,
        .clean_metrics   =    100000,
        .hot_metrics     =   1000000,
        .first_time_t    = 100000000,
        .last_time_t     = 0,
        .cache_size      = 0, // get the default (8MB)
        .collect_threads = 16,
        .query_threads   = 16,
        .partitions      = 0, // get the default (system cpus)
        .options         = PGC_OPTIONS_AUTOSCALE,/* PGC_OPTIONS_FLUSH_PAGES_INLINE | PGC_OPTIONS_EVICT_PAGES_INLINE,*/
        .points_per_page = 10,
        .time_per_collection_ut = 1000000,
        .time_per_query_ut = 250,
        .time_per_flush_ut = 100,
        .rand_statebufs  = {},
        .random_data     = NULL,
};

void *unittest_stress_test_collector(void *ptr) {
    size_t id = *((size_t *)ptr);

    size_t metric_start = pgc_uts.clean_metrics;
    size_t metric_end = pgc_uts.clean_metrics + pgc_uts.hot_metrics;
    size_t number_of_metrics = metric_end - metric_start;
    size_t per_collector_metrics = number_of_metrics / pgc_uts.collect_threads;
    metric_start = metric_start + per_collector_metrics * id + 1;
    metric_end = metric_start + per_collector_metrics - 1;

    time_t start_time_t = pgc_uts.first_time_t + 1;

    heartbeat_t hb;
    heartbeat_init(&hb, pgc_uts.time_per_collection_ut);

    while(!__atomic_load_n(&pgc_uts.stop, __ATOMIC_RELAXED)) {
        // netdata_log_info("COLLECTOR %zu: collecting metrics %zu to %zu, from %ld to %lu", id, metric_start, metric_end, start_time_t, start_time_t + pgc_uts.points_per_page);

        for (size_t i = metric_start; i < metric_end; i++) {
            bool added;

            pgc_uts.metrics[i] = pgc_page_add_and_acquire(pgc_uts.cache, (PGC_ENTRY) {
                    .section = 1,
                    .metric_id = i,
                    .start_time_t = start_time_t,
                    .end_time_t = start_time_t,
                    .update_every = 1,
                    .size = 4096,
                    .data = NULL,
                    .hot = true,
            }, &added);

            if(!pgc_is_page_hot(pgc_uts.metrics[i]) || !added) {
                pgc_page_release(pgc_uts.cache, pgc_uts.metrics[i]);
                pgc_uts.metrics[i] = NULL;
            }
        }

        time_t end_time_t = start_time_t + (time_t)pgc_uts.points_per_page;
        while(++start_time_t <= end_time_t && !__atomic_load_n(&pgc_uts.stop, __ATOMIC_RELAXED)) {
            heartbeat_next(&hb);

            for (size_t i = metric_start; i < metric_end; i++) {
                if(pgc_uts.metrics[i])
                    pgc_page_hot_set_end_time_t(pgc_uts.cache, pgc_uts.metrics[i], start_time_t);
            }

            __atomic_store_n(&pgc_uts.last_time_t, start_time_t, __ATOMIC_RELAXED);
        }

        for (size_t i = metric_start; i < metric_end; i++) {
            if (pgc_uts.metrics[i]) {
                if(i % 10 == 0)
                    pgc_page_to_clean_evict_or_release(pgc_uts.cache, pgc_uts.metrics[i]);
                else
                    pgc_page_hot_to_dirty_and_release(pgc_uts.cache, pgc_uts.metrics[i], false);
            }
        }
    }

    return ptr;
}

void *unittest_stress_test_queries(void *ptr) {
    size_t id = *((size_t *)ptr);
    struct random_data *random_data = &pgc_uts.random_data[id];

    size_t start = 0;
    size_t end = pgc_uts.clean_metrics + pgc_uts.hot_metrics;

    while(!__atomic_load_n(&pgc_uts.stop, __ATOMIC_RELAXED)) {
        int32_t random_number;
        random_r(random_data, &random_number);

        size_t metric_id = random_number % (end - start);
        time_t start_time_t = pgc_uts.first_time_t;
        time_t end_time_t = __atomic_load_n(&pgc_uts.last_time_t, __ATOMIC_RELAXED);
        if(end_time_t <= start_time_t)
            end_time_t = start_time_t + 1;
        size_t pages = (end_time_t - start_time_t) / pgc_uts.points_per_page + 1;

        PGC_PAGE *array[pages];
        for(size_t i = 0; i < pages ;i++)
            array[i] = NULL;

        // find the pages the cache has
        for(size_t i = 0; i < pages ;i++) {
            time_t page_start_time = start_time_t + (time_t)(i * pgc_uts.points_per_page);
            array[i] = pgc_page_get_and_acquire(pgc_uts.cache, 1, metric_id,
                                                page_start_time, (i < pages - 1)?PGC_SEARCH_EXACT:PGC_SEARCH_CLOSEST);
        }

        // load the rest of the pages
        for(size_t i = 0; i < pages ;i++) {
            if(array[i]) continue;

            time_t page_start_time = start_time_t + (time_t)(i * pgc_uts.points_per_page);
            array[i] = pgc_page_add_and_acquire(pgc_uts.cache, (PGC_ENTRY) {
                    .section = 1,
                    .metric_id = metric_id,
                    .start_time_t = page_start_time,
                    .end_time_t = page_start_time + (time_t)pgc_uts.points_per_page,
                    .update_every = 1,
                    .size = 4096,
                    .data = NULL,
                    .hot = false,
            }, NULL);
        }

        // do the query
        // ...
        struct timespec work_duration = {.tv_sec = 0, .tv_nsec = pgc_uts.time_per_query_ut * NSEC_PER_USEC };
        nanosleep(&work_duration, NULL);

        // release the pages
        for(size_t i = 0; i < pages ;i++) {
            if(!array[i]) continue;
            pgc_page_release(pgc_uts.cache, array[i]);
            array[i] = NULL;
        }
    }

    return ptr;
}

void *unittest_stress_test_service(void *ptr) {
    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);
    while(!__atomic_load_n(&pgc_uts.stop, __ATOMIC_RELAXED)) {
        heartbeat_next(&hb);

        pgc_flush_pages(pgc_uts.cache, 1000);
        pgc_evict_pages(pgc_uts.cache, 0, 0);
    }
    return ptr;
}

static void unittest_stress_test_save_dirty_page_callback(PGC *cache __maybe_unused, PGC_ENTRY *entries_array __maybe_unused, PGC_PAGE **pages_array __maybe_unused, size_t entries __maybe_unused) {
    // netdata_log_info("SAVE %zu pages", entries);
    if(!pgc_uts.stop) {
        usec_t t = pgc_uts.time_per_flush_ut;

        if(t > 0) {
            struct timespec work_duration = {
                    .tv_sec = t / USEC_PER_SEC,
                    .tv_nsec = (long) ((t % USEC_PER_SEC) * NSEC_PER_USEC)
            };

            nanosleep(&work_duration, NULL);
        }
    }
}

void unittest_stress_test(void) {
    pgc_uts.cache = pgc_create(pgc_uts.cache_size * 1024 * 1024,
                               unittest_free_clean_page_callback,
                               64, unittest_stress_test_save_dirty_page_callback,
                               1000, 10000, 1,
                               pgc_uts.options, pgc_uts.partitions, 0);

    pgc_uts.metrics = callocz(pgc_uts.clean_metrics + pgc_uts.hot_metrics, sizeof(PGC_PAGE *));

    pthread_t service_thread;
    nd_thread_create(&service_thread, "SERVICE",
                          NETDATA_THREAD_OPTION_DONT_LOG,
                          unittest_stress_test_service, NULL);

    pthread_t collect_threads[pgc_uts.collect_threads];
    size_t collect_thread_ids[pgc_uts.collect_threads];
    for(size_t i = 0; i < pgc_uts.collect_threads ;i++) {
        collect_thread_ids[i] = i;
        char buffer[100 + 1];
        snprintfz(buffer, sizeof(buffer) - 1, "COLLECT_%zu", i);
        nd_thread_create(&collect_threads[i], buffer,
                              NETDATA_THREAD_OPTION_DONT_LOG,
                              unittest_stress_test_collector, &collect_thread_ids[i]);
    }

    pthread_t queries_threads[pgc_uts.query_threads];
    size_t query_thread_ids[pgc_uts.query_threads];
    pgc_uts.random_data = callocz(pgc_uts.query_threads, sizeof(struct random_data));
    for(size_t i = 0; i < pgc_uts.query_threads ;i++) {
        query_thread_ids[i] = i;
        char buffer[100 + 1];
        snprintfz(buffer, sizeof(buffer) - 1, "QUERY_%zu", i);
        initstate_r(1, pgc_uts.rand_statebufs, 1024, &pgc_uts.random_data[i]);
        nd_thread_create(&queries_threads[i], buffer,
                              NETDATA_THREAD_OPTION_DONT_LOG,
                              unittest_stress_test_queries, &query_thread_ids[i]);
    }

    heartbeat_t hb;
    heartbeat_init(&hb, USEC_PER_SEC);

    struct {
        size_t entries;
        size_t added;
        size_t deleted;
        size_t referenced;

        size_t hot_entries;
        size_t hot_added;
        size_t hot_deleted;

        size_t dirty_entries;
        size_t dirty_added;
        size_t dirty_deleted;

        size_t clean_entries;
        size_t clean_added;
        size_t clean_deleted;

        size_t searches_exact;
        size_t searches_exact_hits;
        size_t searches_closest;
        size_t searches_closest_hits;

        size_t collections;

        size_t events_cache_under_severe_pressure;
        size_t events_cache_needs_space_90;
        size_t events_flush_critical;
    } stats = {}, old_stats = {};

    for(int i = 0; i < 86400 ;i++) {
        heartbeat_next(&hb);

        old_stats = stats;
        stats.entries       = __atomic_load_n(&pgc_uts.cache->stats.entries, __ATOMIC_RELAXED);
        stats.added         = __atomic_load_n(&pgc_uts.cache->stats.added_entries, __ATOMIC_RELAXED);
        stats.deleted       = __atomic_load_n(&pgc_uts.cache->stats.removed_entries, __ATOMIC_RELAXED);
        stats.referenced    = __atomic_load_n(&pgc_uts.cache->stats.referenced_entries, __ATOMIC_RELAXED);

        stats.hot_entries   = __atomic_load_n(&pgc_uts.cache->hot.stats->entries, __ATOMIC_RELAXED);
        stats.hot_added     = __atomic_load_n(&pgc_uts.cache->hot.stats->added_entries, __ATOMIC_RELAXED);
        stats.hot_deleted   = __atomic_load_n(&pgc_uts.cache->hot.stats->removed_entries, __ATOMIC_RELAXED);

        stats.dirty_entries = __atomic_load_n(&pgc_uts.cache->dirty.stats->entries, __ATOMIC_RELAXED);
        stats.dirty_added   = __atomic_load_n(&pgc_uts.cache->dirty.stats->added_entries, __ATOMIC_RELAXED);
        stats.dirty_deleted = __atomic_load_n(&pgc_uts.cache->dirty.stats->removed_entries, __ATOMIC_RELAXED);

        stats.clean_entries = __atomic_load_n(&pgc_uts.cache->clean.stats->entries, __ATOMIC_RELAXED);
        stats.clean_added   = __atomic_load_n(&pgc_uts.cache->clean.stats->added_entries, __ATOMIC_RELAXED);
        stats.clean_deleted = __atomic_load_n(&pgc_uts.cache->clean.stats->removed_entries, __ATOMIC_RELAXED);

        stats.searches_exact = __atomic_load_n(&pgc_uts.cache->stats.searches_exact, __ATOMIC_RELAXED);
        stats.searches_exact_hits = __atomic_load_n(&pgc_uts.cache->stats.searches_exact_hits, __ATOMIC_RELAXED);

        stats.searches_closest = __atomic_load_n(&pgc_uts.cache->stats.searches_closest, __ATOMIC_RELAXED);
        stats.searches_closest_hits = __atomic_load_n(&pgc_uts.cache->stats.searches_closest_hits, __ATOMIC_RELAXED);

        stats.events_cache_under_severe_pressure = __atomic_load_n(&pgc_uts.cache->stats.events_cache_under_severe_pressure, __ATOMIC_RELAXED);
        stats.events_cache_needs_space_90 = __atomic_load_n(&pgc_uts.cache->stats.events_cache_needs_space_aggressively, __ATOMIC_RELAXED);
        stats.events_flush_critical = __atomic_load_n(&pgc_uts.cache->stats.events_flush_critical, __ATOMIC_RELAXED);

        size_t searches_exact = stats.searches_exact - old_stats.searches_exact;
        size_t searches_closest = stats.searches_closest - old_stats.searches_closest;

        size_t hit_exact = stats.searches_exact_hits - old_stats.searches_exact_hits;
        size_t hit_closest = stats.searches_closest_hits - old_stats.searches_closest_hits;

        double hit_exact_pc = (searches_exact > 0) ? (double)hit_exact * 100.0 / (double)searches_exact : 0.0;
        double hit_closest_pc = (searches_closest > 0) ? (double)hit_closest * 100.0 / (double)searches_closest : 0.0;

#ifdef PGC_COUNT_POINTS_COLLECTED
        stats.collections = __atomic_load_n(&pgc_uts.cache->stats.points_collected, __ATOMIC_RELAXED);
#endif

        char *cache_status = "N";
        if(stats.events_cache_under_severe_pressure > old_stats.events_cache_under_severe_pressure)
            cache_status = "F";
        else if(stats.events_cache_needs_space_90 > old_stats.events_cache_needs_space_90)
            cache_status = "f";

        char *flushing_status = "N";
        if(stats.events_flush_critical > old_stats.events_flush_critical)
            flushing_status = "F";

        netdata_log_info("PGS %5zuk +%4zuk/-%4zuk "
             "| RF %5zuk "
             "| HOT %5zuk +%4zuk -%4zuk "
             "| DRT %s %5zuk +%4zuk -%4zuk "
             "| CLN %s %5zuk +%4zuk -%4zuk "
             "| SRCH %4zuk %4zuk, HIT %4.1f%% %4.1f%% "
#ifdef PGC_COUNT_POINTS_COLLECTED
             "| CLCT %8.4f Mps"
#endif
             , stats.entries / 1000
             , (stats.added - old_stats.added) / 1000, (stats.deleted - old_stats.deleted) / 1000
             , stats.referenced / 1000
             , stats.hot_entries / 1000, (stats.hot_added - old_stats.hot_added) / 1000, (stats.hot_deleted - old_stats.hot_deleted) / 1000
             , flushing_status
             , stats.dirty_entries / 1000
             , (stats.dirty_added - old_stats.dirty_added) / 1000, (stats.dirty_deleted - old_stats.dirty_deleted) / 1000
             , cache_status
             , stats.clean_entries / 1000
             , (stats.clean_added - old_stats.clean_added) / 1000, (stats.clean_deleted - old_stats.clean_deleted) / 1000
             , searches_exact / 1000, searches_closest / 1000
             , hit_exact_pc, hit_closest_pc
#ifdef PGC_COUNT_POINTS_COLLECTED
             , (double)(stats.collections - old_stats.collections) / 1000.0 / 1000.0
#endif
             );
    }
    netdata_log_info("Waiting for threads to stop...");
    __atomic_store_n(&pgc_uts.stop, true, __ATOMIC_RELAXED);

    nd_thread_join(service_thread, NULL);

    for(size_t i = 0; i < pgc_uts.collect_threads ;i++)
        nd_thread_join(collect_threads[i],NULL);

    for(size_t i = 0; i < pgc_uts.query_threads ;i++)
        nd_thread_join(queries_threads[i],NULL);

    pgc_destroy(pgc_uts.cache);

    freez(pgc_uts.metrics);
    freez(pgc_uts.random_data);
}
#endif

int pgc_unittest(void) {
    PGC *cache = pgc_create("test",
                            32 * 1024 * 1024, unittest_free_clean_page_callback,
                            64, NULL, unittest_save_dirty_page_callback,
                            10, 10, 1000, 10,
                            PGC_OPTIONS_DEFAULT, 1, 11);

    // FIXME - unit tests
    // - add clean page
    // - add clean page again (should not add it)
    // - release page (should decrement counters)
    // - add hot page
    // - add hot page again (should not add it)
    // - turn hot page to dirty, with and without a reference counter to it
    // - dirty pages are saved once there are enough of them
    // - find page exact
    // - find page (should return last)
    // - find page (should return next)
    // - page cache full (should evict)
    // - on destroy, turn hot pages to dirty and save them

    PGC_PAGE *page1 = pgc_page_add_and_acquire(cache, (PGC_ENTRY){
        .section = 1,
        .metric_id = 10,
        .start_time_s = 100,
        .end_time_s = 1000,
        .size = 4096,
        .data = NULL,
        .hot = false,
        .custom_data = (uint8_t *)"0123456789",
    }, NULL);

    if(strcmp(pgc_page_custom_data(cache, page1), "0123456789") != 0)
        fatal("custom data do not work");

    memcpy(pgc_page_custom_data(cache, page1), "ABCDEFGHIJ", 11);
    if(strcmp(pgc_page_custom_data(cache, page1), "ABCDEFGHIJ") != 0)
        fatal("custom data do not work");

    pgc_page_release(cache, page1);

    PGC_PAGE *page2 = pgc_page_add_and_acquire(cache, (PGC_ENTRY){
            .section = 2,
            .metric_id = 10,
            .start_time_s = 1001,
            .end_time_s = 2000,
            .size = 4096,
            .data = NULL,
            .hot = true,
    }, NULL);

    pgc_page_hot_set_end_time_s(cache, page2, 2001, 0);
    pgc_page_hot_to_dirty_and_release(cache, page2, false);

    PGC_PAGE *page3 = pgc_page_add_and_acquire(cache, (PGC_ENTRY){
            .section = 3,
            .metric_id = 10,
            .start_time_s = 1001,
            .end_time_s = 2000,
            .size = 4096,
            .data = NULL,
            .hot = true,
    }, NULL);

    pgc_page_hot_set_end_time_s(cache, page3, 2001, 0);
    pgc_page_hot_to_dirty_and_release(cache, page3, false);

    pgc_destroy(cache, true);

#ifdef PGC_STRESS_TEST
    unittest_stress_test();
#endif

    return 0;
}
