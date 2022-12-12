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

typedef int32_t REFCOUNT;
#define REFCOUNT_DELETING (-100)

// to use arrayalloc uncomment the following line:
// #define PGC_WITH_ARAL 1

typedef enum __attribute__ ((__packed__)) {
    // mutually exclusive flags
    PGC_PAGE_CLEAN     = (1 << 0), // none of the following
    PGC_PAGE_DIRTY     = (1 << 1), // contains unsaved data
    PGC_PAGE_HOT       = (1 << 2), // currently being collected

    // flags related to various actions on each page
    PGC_PAGE_IS_BEING_CREATED = (1 << 3),
    PGC_PAGE_IS_BEING_DELETED = (1 << 4),
    PGC_PAGE_IS_BEING_SAVED   = (1 << 5),

    PGC_PAGE_HAS_NO_DATA_IGNORE_ACCESSES = (1 << 6),
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
    time_t start_time_t;
    time_t end_time_t;
    uint32_t update_every;
    uint32_t accesses;              // counts the number of accesses on this page

    void *data;
    size_t assumed_size;
    REFCOUNT refcount;
    PGC_PAGE_FLAGS flags;
    SPINLOCK transition_spinlock;   // when the page changes between HOT, DIRTY, CLEAN, we have to get this lock

    struct {
        struct pgc_page *next;
        struct pgc_page *prev;
    } link;
};

struct pgc_linked_list {
    SPINLOCK spinlock;
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
        size_t partitions;
        size_t clean_size;
        size_t max_dirty_pages_per_call;
        size_t max_pages_per_inline_eviction;
        size_t max_skip_pages_per_inline_eviction;
        size_t max_flushes_inline;
        free_clean_page_callback pgc_free_clean_cb;
        save_dirty_page_callback pgc_save_dirty_cb;
        PGC_OPTIONS options;
    } config;

#ifdef PGC_WITH_ARAL
    ARAL *aral;
#endif

    struct pgc_index {
        netdata_rwlock_t rwlock;
        Pvoid_t sections_judy;
    } *index;

    struct pgc_linked_list clean;       // LRU is applied here to free memory from the cache
    struct pgc_linked_list dirty;       // in the dirty list, pages are ordered the way they were marked dirty
    struct pgc_linked_list hot;         // in the hot list, pages are order the way they were marked hot

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
// locking

static size_t indexing_partition(PGC *cache, Word_t metric_id) {
    static __thread Word_t last_metric_id = 0;
    static __thread size_t last_partition = 0;

    if(metric_id == last_metric_id || cache->config.partitions == 1)
        return last_partition;

    size_t total = 0;
    total += (metric_id & 0xff) >> 0;
    total += (metric_id & 0xff00) >> 8;
    total += (metric_id & 0xff0000) >> 16;
    total += (metric_id & 0xff000000) >> 24;

    if(sizeof(Word_t) > 4) {
        total += (metric_id & 0xff00000000) >> 32;
        total += (metric_id & 0xff0000000000) >> 40;
        total += (metric_id & 0xff000000000000) >> 48;
        total += (metric_id & 0xff00000000000000) >> 56;
    }

    last_metric_id = metric_id;
    last_partition = total % cache->config.partitions;

    return last_partition;
}

static void pgc_index_read_lock(PGC *cache, size_t partition) {
    netdata_rwlock_rdlock(&cache->index[partition].rwlock);
}
static void pgc_index_read_unlock(PGC *cache, size_t partition) {
    netdata_rwlock_unlock(&cache->index[partition].rwlock);
}
static bool pgc_index_write_trylock(PGC *cache, size_t partition) {
    return !netdata_rwlock_trywrlock(&cache->index[partition].rwlock);
}
static void pgc_index_write_lock(PGC *cache, size_t partition) {
    netdata_rwlock_wrlock(&cache->index[partition].rwlock);
}
static void pgc_index_write_unlock(PGC *cache, size_t partition) {
    netdata_rwlock_unlock(&cache->index[partition].rwlock);
}

static inline bool pgc_ll_trylock(PGC *cache __maybe_unused, struct pgc_linked_list *ll) {
    return netdata_spinlock_trylock(&ll->spinlock);
}

static inline void pgc_ll_lock(PGC *cache __maybe_unused, struct pgc_linked_list *ll) {
    netdata_spinlock_lock(&ll->spinlock);
}

static inline void pgc_ll_unlock(PGC *cache __maybe_unused, struct pgc_linked_list *ll) {
    netdata_spinlock_unlock(&ll->spinlock);
}

static inline bool page_transition_trylock(PGC *cache __maybe_unused, PGC_PAGE *page) {
    return netdata_spinlock_trylock(&page->transition_spinlock);
}

static inline void page_transition_lock(PGC *cache __maybe_unused, PGC_PAGE *page) {
    netdata_spinlock_lock(&page->transition_spinlock);
}

static inline void page_transition_unlock(PGC *cache __maybe_unused, PGC_PAGE *page) {
    netdata_spinlock_unlock(&page->transition_spinlock);
}

// ----------------------------------------------------------------------------
// evictions control

static inline size_t cache_usage_percent(PGC *cache) {
    if(cache->config.options & PGC_OPTIONS_AUTOSCALE) {
        size_t clean   = __atomic_load_n(&cache->clean.stats->size, __ATOMIC_RELAXED);
        size_t dirty   = __atomic_load_n(&cache->dirty.stats->size, __ATOMIC_RELAXED);
        size_t hot     = __atomic_load_n(&cache->hot.stats->size, __ATOMIC_RELAXED);
        size_t hot_max = __atomic_load_n(&cache->hot.stats->max_size, __ATOMIC_RELAXED);

        size_t wanted_cache_size = hot_max * 2;

        if(wanted_cache_size < cache->config.clean_size + hot_max)
            wanted_cache_size = cache->config.clean_size + hot_max;

        size_t max_for_clean;
        if(wanted_cache_size < hot + dirty + cache->config.clean_size)
            max_for_clean = cache->config.clean_size;
        else
            max_for_clean = wanted_cache_size - hot - dirty;

        size_t percent = clean * 100 / max_for_clean;
        return percent;
    }
    else {
        size_t clean = __atomic_load_n(&cache->clean.stats->size, __ATOMIC_RELAXED);
        size_t max = cache->config.clean_size;
        size_t percent = clean * 100 / max;
        return percent;
    }
}

static inline bool cache_under_severe_pressure(PGC *cache) {
    if(unlikely(cache_usage_percent(cache) >= 95)) {
        __atomic_add_fetch(&cache->stats.events_cache_under_severe_pressure, 1, __ATOMIC_RELAXED);
        return true;
    }

    return false;
}

static inline bool cache_needs_space_90(PGC *cache) {
    if(unlikely(cache_usage_percent(cache) >= 90)) {
        __atomic_add_fetch(&cache->stats.events_cache_needs_space_90, 1, __ATOMIC_RELAXED);
        return true;
    }

    return false;
}

#define cache_above_healthy_limit_85(cache) (cache_usage_percent(cache) >= 85)

static bool make_acquired_page_clean_and_evict_or_page_release(PGC *cache, PGC_PAGE *page);
static bool evict_pages(PGC *cache, size_t max_skip, size_t max_evict, bool wait, bool all_of_them);

static void evict_on_clean_page_added(PGC *cache __maybe_unused) {
    if((cache->config.options & PGC_OPTIONS_EVICT_PAGES_INLINE) || cache_needs_space_90(cache)) {
        bool under_pressure = cache_under_severe_pressure(cache);
        evict_pages(cache,
                    under_pressure ? 0 : cache->config.max_skip_pages_per_inline_eviction,
                    under_pressure ? 0 : cache->config.max_pages_per_inline_eviction,
                    under_pressure, false);
    }
}

static void evict_on_hot_page_added(PGC *cache __maybe_unused) {
    ;
}

static void evict_on_page_searched_and_found(PGC *cache __maybe_unused) {
    ;
}

static void evict_on_page_searched_and_not_found(PGC *cache __maybe_unused) {
    ;
}

static void evict_on_page_release_when_permitted(PGC *cache __maybe_unused) {
    if (unlikely((cache->config.options & PGC_OPTIONS_EVICT_PAGES_INLINE) || cache_needs_space_90(cache))) {
        bool under_pressure = cache_under_severe_pressure(cache);
        evict_pages(cache,
                    under_pressure ? 0 : cache->config.max_skip_pages_per_inline_eviction,
                    under_pressure ? 0 : cache->config.max_pages_per_inline_eviction,
                    under_pressure, false);
    }
}

// ----------------------------------------------------------------------------
// flushing control

static bool flush_pages(PGC *cache, size_t max_flushes, bool wait, bool all_of_them);

static inline bool flushing_critical(PGC *cache) {
    if(unlikely(__atomic_load_n(&cache->dirty.stats->size, __ATOMIC_RELAXED) > __atomic_load_n(&cache->hot.stats->max_size, __ATOMIC_RELAXED))) {
        __atomic_add_fetch(&cache->stats.events_flush_critical, 1, __ATOMIC_RELAXED);
        return true;
    }

    return false;
}

// ----------------------------------------------------------------------------
// helpers

static size_t page_assumed_size(size_t size) {
    return size + sizeof(PGC_PAGE) + sizeof(Word_t) * 3;
}

static size_t page_size_from_assumed_size(size_t assumed_size) {
    return assumed_size - sizeof(PGC_PAGE) - sizeof(Word_t) * 3;
}

// ----------------------------------------------------------------------------
// Linked list management

static inline void atomic_set_max(size_t *max, size_t desired) {
    size_t expected;

    expected = __atomic_load_n(max, __ATOMIC_RELAXED);

    do {

        if(expected >= desired)
            return;

    } while(!__atomic_compare_exchange_n(max, &expected, desired,
                                         false, __ATOMIC_RELAXED, __ATOMIC_RELAXED));
}

struct section_dirty_pages {
    size_t entries;
    size_t size;
    PGC_PAGE *base;
};

static void pgc_stats_ll_judy_change(PGC *cache, struct pgc_linked_list *ll, size_t mem_before_judyl, size_t mem_after_judyl) {
    if(mem_after_judyl > mem_before_judyl) {
        __atomic_add_fetch(&ll->stats->size, mem_after_judyl - mem_before_judyl, __ATOMIC_RELAXED);
        __atomic_add_fetch(&cache->stats.size, mem_after_judyl - mem_before_judyl, __ATOMIC_RELAXED);
    }
    else if(mem_after_judyl < mem_before_judyl) {
        __atomic_sub_fetch(&ll->stats->size, mem_before_judyl - mem_after_judyl, __ATOMIC_RELAXED);
        __atomic_sub_fetch(&cache->stats.size, mem_before_judyl - mem_after_judyl, __ATOMIC_RELAXED);
    }
}

static void pgc_stats_index_judy_change(PGC *cache, size_t mem_before_judyl, size_t mem_after_judyl) {
    if(mem_after_judyl > mem_before_judyl) {
        __atomic_add_fetch(&cache->stats.size, mem_after_judyl - mem_before_judyl, __ATOMIC_RELAXED);
    }
    else if(mem_after_judyl < mem_before_judyl) {
        __atomic_sub_fetch(&cache->stats.size, mem_before_judyl - mem_after_judyl, __ATOMIC_RELAXED);
    }
}

static void pgc_ll_add(PGC *cache __maybe_unused, struct pgc_linked_list *ll, PGC_PAGE *page, bool having_lock) {
    if(!having_lock)
        pgc_ll_lock(cache, ll);

    internal_fatal(page_get_status_flags(page) != 0,
                   "DBENGINE CACHE: invalid page flags, the page has %d, but it is should be %d",
                   page_get_status_flags(page),
                   0);

    if(ll->linked_list_in_sections_judy) {
        size_t mem_before_judyl, mem_after_judyl;

        mem_before_judyl = JudyLMemUsed(ll->sections_judy);
        Pvoid_t *dirty_pages_pptr = JudyLIns(&ll->sections_judy, page->section, PJE0);
        mem_after_judyl = JudyLMemUsed(ll->sections_judy);

        struct section_dirty_pages *sdp = *dirty_pages_pptr;
        if(!sdp) {
            sdp = callocz(1, sizeof(struct section_dirty_pages));
            *dirty_pages_pptr = sdp;

            mem_after_judyl += sizeof(struct section_dirty_pages);
        }
        pgc_stats_ll_judy_change(cache, ll, mem_before_judyl, mem_after_judyl);

        sdp->entries++;
        sdp->size += page->assumed_size;
        DOUBLE_LINKED_LIST_APPEND_UNSAFE(sdp->base, page, link.prev, link.next);
    }
    else {
        // HOT and CLEAN pages end up here.
        // HOT pages never have accesses when they are created, so they are always prepended.
        // CLEAN pages may or may not have accesses, depending on how they have been created:
        // - New pages created as CLEAN, always have 1 access.
        // - DIRTY pages made CLEAN, depending on their accesses may be appended (accesses > 0) or prepended (accesses = 0).

        if(!page->accesses)
            DOUBLE_LINKED_LIST_PREPEND_UNSAFE(ll->base, page, link.prev, link.next);
        else
            DOUBLE_LINKED_LIST_APPEND_UNSAFE(ll->base, page, link.prev, link.next);
    }

    ll->version++;

    page_flag_set(page, ll->flags);

    if(!having_lock)
        pgc_ll_unlock(cache, ll);

    size_t entries = __atomic_add_fetch(&ll->stats->entries, 1, __ATOMIC_RELAXED);
    size_t size    = __atomic_add_fetch(&ll->stats->size, page->assumed_size, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ll->stats->added_entries, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ll->stats->added_size, page->assumed_size, __ATOMIC_RELAXED);

    atomic_set_max(&ll->stats->max_entries, entries);
    atomic_set_max(&ll->stats->max_size, size);
}

static void pgc_ll_del(PGC *cache __maybe_unused, struct pgc_linked_list *ll, PGC_PAGE *page, bool having_lock) {
    __atomic_sub_fetch(&ll->stats->entries, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&ll->stats->size, page->assumed_size, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ll->stats->removed_entries, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ll->stats->removed_size, page->assumed_size, __ATOMIC_RELAXED);

    if(!having_lock)
        pgc_ll_lock(cache, ll);

    internal_fatal(page_get_status_flags(page) != ll->flags,
                   "DBENGINE CACHE: invalid page flags, the page has %d, but it is should be %d",
                   page_get_status_flags(page),
                   ll->flags);

    page_flag_clear(page, ll->flags);

    if(ll->linked_list_in_sections_judy) {
        Pvoid_t *dirty_pages_pptr = JudyLGet(ll->sections_judy, page->section, PJE0);
        internal_fatal(!dirty_pages_pptr, "DBENGINE CACHE: page should be in Judy LL, but it is not");

        struct section_dirty_pages *sdp = *dirty_pages_pptr;
        sdp->entries--;
        sdp->size -= page->assumed_size;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(sdp->base, page, link.prev, link.next);

        if(!sdp->base) {
            size_t mem_before_judyl, mem_after_judyl;

            mem_before_judyl = JudyLMemUsed(ll->sections_judy);
            int rc = JudyLDel(&ll->sections_judy, page->section, PJE0);
            mem_after_judyl = JudyLMemUsed(ll->sections_judy);

            if(!rc)
                fatal("DBENGINE CACHE: cannot delete section from Judy LL");

            freez(sdp);
            mem_after_judyl -= sizeof(struct section_dirty_pages);
            pgc_stats_ll_judy_change(cache, ll, mem_before_judyl, mem_after_judyl);
        }
    }
    else {
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(ll->base, page, link.prev, link.next);
    }

    ll->version++;

    if(!having_lock)
        pgc_ll_unlock(cache, ll);
}

static void page_has_been_accessed(PGC *cache, PGC_PAGE *page) {
    PGC_PAGE_FLAGS flags = page_flag_check(page, PGC_PAGE_CLEAN | PGC_PAGE_HAS_NO_DATA_IGNORE_ACCESSES);

    if (!(flags & PGC_PAGE_HAS_NO_DATA_IGNORE_ACCESSES)) {
        __atomic_add_fetch(&page->accesses, 1, __ATOMIC_RELAXED);

        if (flags & PGC_PAGE_CLEAN) {
            pgc_ll_lock(cache, &cache->clean);
            DOUBLE_LINKED_LIST_REMOVE_UNSAFE(cache->clean.base, page, link.prev, link.next);
            DOUBLE_LINKED_LIST_APPEND_UNSAFE(cache->clean.base, page, link.prev, link.next);
            pgc_ll_unlock(cache, &cache->clean);
        }
    }
}


// ----------------------------------------------------------------------------
// state transitions

static inline void page_set_clean(PGC *cache, PGC_PAGE *page, bool having_transition_lock, bool having_clean_lock) {
    if(!having_transition_lock)
        page_transition_lock(cache, page);

    PGC_PAGE_FLAGS flags = page_get_status_flags(page);

    if(flags & PGC_PAGE_CLEAN) {
        if(!having_transition_lock)
            page_transition_unlock(cache, page);
        return;
    }

    if(flags & PGC_PAGE_HOT)
        pgc_ll_del(cache, &cache->hot, page, false);

    if(flags & PGC_PAGE_DIRTY)
        pgc_ll_del(cache, &cache->dirty, page, false);

    // first add to linked list, the set the flag (required for move_page_last())
    pgc_ll_add(cache, &cache->clean, page, having_clean_lock);

    if(!having_transition_lock)
        page_transition_unlock(cache, page);
}

static void page_set_dirty(PGC *cache, PGC_PAGE *page, bool having_hot_lock) {
    page_transition_lock(cache, page);

    PGC_PAGE_FLAGS flags = page_get_status_flags(page);

    if(flags & PGC_PAGE_DIRTY) {
        page_transition_unlock(cache, page);
        return;
    }

    if(flags & PGC_PAGE_HOT)
        pgc_ll_del(cache, &cache->hot, page, having_hot_lock);

    if(flags & PGC_PAGE_CLEAN)
        pgc_ll_del(cache, &cache->clean, page, false);

    // first add to linked list, the set the flag (required for move_page_last())
    pgc_ll_add(cache, &cache->dirty, page, false);

    page_transition_unlock(cache, page);
}

static inline void page_set_hot(PGC *cache, PGC_PAGE *page) {
    page_transition_lock(cache, page);

    PGC_PAGE_FLAGS flags = page_get_status_flags(page);

    if(flags & PGC_PAGE_HOT) {
        page_transition_unlock(cache, page);
        return;
    }

    if(flags & PGC_PAGE_DIRTY)
        pgc_ll_del(cache, &cache->dirty, page, false);

    if(flags & PGC_PAGE_CLEAN)
        pgc_ll_del(cache, &cache->clean, page, false);

    // first add to linked list, the set the flag (required for move_page_last())
    pgc_ll_add(cache, &cache->hot, page, false);

    page_transition_unlock(cache, page);
}


// ----------------------------------------------------------------------------
// Referencing

static inline size_t PGC_REFERENCED_PAGES(PGC *cache) {
    return __atomic_load_n(&cache->stats.referenced_entries, __ATOMIC_RELAXED);
}

static inline void PGC_REFERENCED_PAGES_PLUS1(PGC *cache, PGC_PAGE *page) {
    __atomic_add_fetch(&cache->stats.referenced_entries, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&cache->stats.referenced_size, page->assumed_size, __ATOMIC_RELAXED);
}

static inline void PGC_REFERENCED_PAGES_MINUS1(PGC *cache, size_t assumed_size) {
    __atomic_sub_fetch(&cache->stats.referenced_entries, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&cache->stats.referenced_size, assumed_size, __ATOMIC_RELAXED);
}

static inline bool page_acquire___while_having_some_lock(PGC *cache, PGC_PAGE *page) {
    REFCOUNT expected, desired;

    expected = __atomic_load_n(&page->refcount, __ATOMIC_RELAXED);
    size_t spins = 0;

    do {
        spins++;

        if(unlikely(expected < 0))
            return false;

        desired = expected + 1;

    } while(!__atomic_compare_exchange_n(&page->refcount, &expected, desired, false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED));

    if(unlikely(spins > 1))
        __atomic_add_fetch(&cache->stats.acquire_spins, spins - 1, __ATOMIC_RELAXED);

    if(desired == 1)
        PGC_REFERENCED_PAGES_PLUS1(cache, page);

    return true;
}

static inline void page_release(PGC *cache, PGC_PAGE *page, bool evict_if_necessary) {
    size_t assumed_size = page->assumed_size; // take the size before we release it
    REFCOUNT expected, desired;

    expected = __atomic_load_n(&page->refcount, __ATOMIC_RELAXED);

    size_t spins = 0;
    do {
        spins++;

        internal_fatal(expected <= 0,
                       "DBENGINE CACHE: trying to release a page with reference counter %d", expected);

        desired = expected - 1;

    } while(!__atomic_compare_exchange_n(&page->refcount, &expected, desired, false, __ATOMIC_RELEASE, __ATOMIC_RELAXED));

    if(unlikely(spins > 1))
        __atomic_add_fetch(&cache->stats.release_spins, spins - 1, __ATOMIC_RELAXED);

    if(desired == 0) {
        PGC_REFERENCED_PAGES_MINUS1(cache, assumed_size);

        if(evict_if_necessary)
            evict_on_page_release_when_permitted(cache);
    }
}

static inline bool acquired_page_get_for_deletion_or_release_it(PGC *cache __maybe_unused, PGC_PAGE *page) {
    size_t assumed_size = page->assumed_size; // take the size before we release it

    internal_fatal(!is_page_clean(page),
                   "DBENGINE CACHE: only clean pages can be deleted");

    REFCOUNT expected, desired;

    expected = __atomic_load_n(&page->refcount, __ATOMIC_RELAXED);
    size_t spins = 0;
    bool delete_it;

    do {
        spins++;

        internal_fatal(expected < 1,
                       "DBENGINE CACHE: page to be deleted should be acquired by the caller.");

        if (expected == 1) {
            // we are the only one having this page referenced
            desired = REFCOUNT_DELETING;
            delete_it = true;
        }
        else {
            // this page cannot be deleted
            desired = expected - 1;
            delete_it = false;
        }

    } while(!__atomic_compare_exchange_n(&page->refcount, &expected, desired, false, __ATOMIC_RELEASE, __ATOMIC_RELAXED));

    if(delete_it) {
        PGC_REFERENCED_PAGES_MINUS1(cache, assumed_size);

        // we can delete this page
        internal_fatal(page_flag_check(page, PGC_PAGE_IS_BEING_DELETED),
                       "DBENGINE CACHE: page is already being deleted");

        page_flag_set(page, PGC_PAGE_IS_BEING_DELETED);
    }

    if(unlikely(spins > 1))
        __atomic_add_fetch(&cache->stats.delete_spins, spins - 1, __ATOMIC_RELAXED);

    return delete_it;
}


// ----------------------------------------------------------------------------
// Indexing

static void free_this_page(PGC *cache, PGC_PAGE *page) {
    // call the callback to free the user supplied memory
    cache->config.pgc_free_clean_cb(cache, (PGC_ENTRY){
            .section = page->section,
            .metric_id = page->metric_id,
            .start_time_t = page->start_time_t,
            .end_time_t = __atomic_load_n(&page->end_time_t, __ATOMIC_RELAXED),
            .update_every = page->update_every,
            .size = page_size_from_assumed_size(page->assumed_size),
            .hot = (is_page_hot(page)) ? true : false,
    });

    // update statistics
    __atomic_add_fetch(&cache->stats.removed_entries, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&cache->stats.removed_size, page->assumed_size, __ATOMIC_RELAXED);

    __atomic_sub_fetch(&cache->stats.entries, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&cache->stats.size, page->assumed_size, __ATOMIC_RELAXED);

    // free our memory
#ifdef PGC_WITH_ARAL
    arrayalloc_freez(cache->aral, page);
#else
    freez(page);
#endif
}

static void remove_this_page_from_index_unsafe(PGC *cache, PGC_PAGE *page, size_t partition) {
    // remove it from the Judy arrays

    pointer_check(cache, page);

    internal_fatal(page_flag_check(page, PGC_PAGE_HOT | PGC_PAGE_DIRTY | PGC_PAGE_CLEAN),
                   "DBENGINE CACHE: page to be removed from the cache is still in the linked-list");

    internal_fatal(!page_flag_check(page, PGC_PAGE_IS_BEING_DELETED),
                   "DBENGINE CACHE: page to be removed from the index, is not marked for deletion");

    internal_fatal(partition != indexing_partition(cache, page->metric_id),
                   "DBENGINE CACHE: attempted to remove this page from the wrong partition of the cache");

    Pvoid_t *metrics_judy_pptr = JudyLGet(cache->index[partition].sections_judy, page->section, PJE0);
    if(unlikely(!metrics_judy_pptr))
        fatal("DBENGINE CACHE: section '%lu' should exist, but it does not.", page->section);

    Pvoid_t *pages_judy_pptr = JudyLGet(*metrics_judy_pptr, page->metric_id, PJE0);
    if(unlikely(!pages_judy_pptr))
        fatal("DBENGINE CACHE: metric '%lu' in section '%lu' should exist, but it does not.",
              page->metric_id, page->section);

    Pvoid_t *page_ptr = JudyLGet(*pages_judy_pptr, page->start_time_t, PJE0);
    if(unlikely(!page_ptr))
        fatal("DBENGINE CACHE: page with start time '%ld' of metric '%lu' in section '%lu' should exist, but it does not.",
              page->start_time_t, page->metric_id, page->section);

    PGC_PAGE *found_page = *page_ptr;
    if(unlikely(found_page != page))
        fatal("DBENGINE CACHE: page with start time '%ld' of metric '%lu' in section '%lu' should exist, but the index returned a different address.",
              page->start_time_t, page->metric_id, page->section);

    size_t mem_before_judyl = 0, mem_after_judyl = 0;

    mem_before_judyl += JudyLMemUsed(*pages_judy_pptr);
    if(unlikely(!JudyLDel(pages_judy_pptr, page->start_time_t, PJE0)))
        fatal("DBENGINE CACHE: page with start time '%ld' of metric '%lu' in section '%lu' exists, but cannot be deleted.",
              page->start_time_t, page->metric_id, page->section);
    mem_after_judyl += JudyLMemUsed(*pages_judy_pptr);

    mem_before_judyl += JudyLMemUsed(*metrics_judy_pptr);
    if(!*pages_judy_pptr && !JudyLDel(metrics_judy_pptr, page->metric_id, PJE0))
        fatal("DBENGINE CACHE: metric '%lu' in section '%lu' exists and is empty, but cannot be deleted.",
              page->metric_id, page->section);
    mem_after_judyl += JudyLMemUsed(*metrics_judy_pptr);

    mem_before_judyl += JudyLMemUsed(cache->index[partition].sections_judy);
    if(!*metrics_judy_pptr && !JudyLDel(&cache->index[partition].sections_judy, page->section, PJE0))
        fatal("DBENGINE CACHE: section '%lu' exists and is empty, but cannot be deleted.", page->section);
    mem_after_judyl += JudyLMemUsed(cache->index[partition].sections_judy);

    pgc_stats_index_judy_change(cache, mem_before_judyl, mem_after_judyl);

    pointer_del(cache, page);
}

static bool make_acquired_page_clean_and_evict_or_page_release(PGC *cache, PGC_PAGE *page) {
    pointer_check(cache, page);

    page_transition_lock(cache, page);
    pgc_ll_lock(cache, &cache->clean);

    // make it clean - it does not have any accesses, so it will be prepended
    page_set_clean(cache, page, true, true);

    if(!acquired_page_get_for_deletion_or_release_it(cache, page)) {
        pgc_ll_unlock(cache, &cache->clean);
        page_transition_unlock(cache, page);
        return false;
    }

    // remove it from the linked list
    pgc_ll_del(cache, &cache->clean, page, true);
    pgc_ll_unlock(cache, &cache->clean);
    page_transition_unlock(cache, page);

    size_t partition = indexing_partition(cache, page->metric_id);
    pgc_index_write_lock(cache, partition);
    remove_this_page_from_index_unsafe(cache, page, partition);
    pgc_index_write_unlock(cache, partition);
    free_this_page(cache, page);

    return true;
}

// returns true, when there is more work to do
static bool evict_pages(PGC *cache, size_t max_skip, size_t max_evict, bool wait, bool all_of_them) {
    if(!all_of_them && !cache_above_healthy_limit_85(cache))
        // don't bother - not enough to do anything
        return false;

    internal_fatal(cache->clean.linked_list_in_sections_judy,
                   "wrong clean pages configuration - clean pages need to have a linked list, not a judy array");

    if(unlikely(!max_skip))
        max_skip = SIZE_MAX;

    if(unlikely(!max_evict))
        max_evict = SIZE_MAX;

    PGC_PAGE *to_evict[cache->config.partitions];
    size_t pages_to_evict;
    size_t total_pages_evicted = 0;
    size_t total_pages_skipped = 0;
    bool stopped_before_finishing = false;
    size_t spins = 0;
    size_t pages_to_evict_per_run = cache->config.partitions * 1000;

    do {
        spins++;

        // zero our partition linked lists
        for (size_t partition = 0; partition < cache->config.partitions; partition++)
            to_evict[partition] = NULL;

        pages_to_evict = 0;

        if(!all_of_them && !wait) {
            if(!pgc_ll_trylock(cache, &cache->clean)) {
                stopped_before_finishing = true;
                goto premature_exit;
            }

            // at this point we have the clean lock
        }
        else
            pgc_ll_lock(cache, &cache->clean);

        for(PGC_PAGE *page = cache->clean.base; page ; ) {
            PGC_PAGE *next = page->link.next;

            if(page_acquire___while_having_some_lock(cache, page) &&
               acquired_page_get_for_deletion_or_release_it(cache, page)) {
                // we can delete this page

                // remove it from the clean list
                pgc_ll_del(cache, &cache->clean, page, true);

                // append it to our eviction list
                size_t partition = indexing_partition(cache, page->metric_id);
                DOUBLE_LINKED_LIST_APPEND_UNSAFE(to_evict[partition], page, link.prev, link.next);

                // check if we have to stop
                if(++total_pages_evicted >= max_evict && !all_of_them) {
                    stopped_before_finishing = true;
                    break;
                }

                if(++pages_to_evict >= pages_to_evict_per_run)
                    break;
            }
            else {
                // we can't delete this page

                // check if we have to stop
                if(++total_pages_skipped >= max_skip && !all_of_them) {
                    stopped_before_finishing = true;
                    break;
                }
            }

            page = next;
        }
        pgc_ll_unlock(cache, &cache->clean);

        if(pages_to_evict) {
            // remove them from the index

            // we don't want to just get the lock,
            // we want to try to get it, if we can
            // and repeat until we do get it
            // so that query and collection threads
            // will not be stopped because of us

            bool partition_waiting[cache->config.partitions];
            memset(partition_waiting, 0, sizeof(partition_waiting));

            // fill-in the status for each partition
            for (size_t partition = 0; partition < cache->config.partitions; partition++) {
                if (!to_evict[partition])
                    partition_waiting[partition] = false;
                else
                    partition_waiting[partition] = true;
            }

            // repeat until all partitions have been cleaned up
            size_t repeats = cache->config.partitions * 2;
            size_t waiting = cache->config.partitions;
            bool force = false;
            while(waiting) {
                if(--repeats == 0 || waiting == 1) force = true;
                waiting = 0;

                for (size_t partition = 0; partition < cache->config.partitions; partition++) {
                    if (!partition_waiting[partition]) continue;

                    if(force)
                        pgc_index_write_lock(cache, partition);
                    else if(!pgc_index_write_trylock(cache, partition)) {
                        waiting++;
                        continue;
                    }

                    for (PGC_PAGE *page = to_evict[partition]; page; page = page->link.next)
                        remove_this_page_from_index_unsafe(cache, page, partition);

                    pgc_index_write_unlock(cache, partition);
                    partition_waiting[partition] = false;
                }
            }

            // free memory, while we don't hold any locks
            for (size_t partition = 0; partition < cache->config.partitions; partition++) {
                if (!to_evict[partition]) continue;

                while (to_evict[partition]) {
                    PGC_PAGE *page = to_evict[partition];
                    DOUBLE_LINKED_LIST_REMOVE_UNSAFE(to_evict[partition], page, link.prev, link.next);
                    free_this_page(cache, page);
                }
            }
        }

    } while(pages_to_evict && (all_of_them || (cache_above_healthy_limit_85(cache) && total_pages_evicted < max_evict && total_pages_skipped < max_skip)));

    if(all_of_them && PGC_REFERENCED_PAGES(cache)) {
        error_limit_static_global_var(erl, 1, 0);
        error_limit(&erl, "DBENGINE CACHE: cannot free all clean pages, some are still referenced");
    }
    else if(!total_pages_evicted && cache_under_severe_pressure(cache)) {
        error_limit_static_global_var(erl, 1, 0);
        error_limit(&erl,
                    "DBENGINE CACHE: cache is %zu %% full, but all the data in it are currently referenced and cannot be evicted",
                    cache_usage_percent(cache));
    }

premature_exit:
    if(unlikely(total_pages_skipped))
        __atomic_add_fetch(&cache->stats.evict_skipped, total_pages_skipped, __ATOMIC_RELAXED);

    if(unlikely(spins > 1))
        __atomic_add_fetch(&cache->stats.evict_spins, spins - 1, __ATOMIC_RELAXED);

    return stopped_before_finishing;
}

static PGC_PAGE *page_add(PGC *cache, PGC_ENTRY *entry, bool *added) {
    PGC_PAGE *page;
    size_t spins = 0;

    do {
        spins++;

        size_t partition = indexing_partition(cache, entry->metric_id);
        pgc_index_write_lock(cache, partition);

        size_t mem_before_judyl = 0, mem_after_judyl = 0;

        mem_before_judyl += JudyLMemUsed(cache->index[partition].sections_judy);
        Pvoid_t *metrics_judy_pptr = JudyLIns(&cache->index[partition].sections_judy, entry->section, PJE0);
        if(unlikely(!metrics_judy_pptr || metrics_judy_pptr == PJERR))
            fatal("DBENGINE CACHE: corrupted sections judy array");
        mem_after_judyl += JudyLMemUsed(cache->index[partition].sections_judy);

        mem_before_judyl += JudyLMemUsed(*metrics_judy_pptr);
        Pvoid_t *pages_judy_pptr = JudyLIns(metrics_judy_pptr, entry->metric_id, PJE0);
        if(unlikely(!pages_judy_pptr || pages_judy_pptr == PJERR))
            fatal("DBENGINE CACHE: corrupted pages judy array");
        mem_after_judyl += JudyLMemUsed(*metrics_judy_pptr);

        mem_before_judyl += JudyLMemUsed(*pages_judy_pptr);
        Pvoid_t *page_ptr = JudyLIns(pages_judy_pptr, entry->start_time_t, PJE0);
        if(unlikely(!page_ptr || page_ptr == PJERR))
            fatal("DBENGINE CACHE: corrupted page in judy array");
        mem_after_judyl += JudyLMemUsed(*pages_judy_pptr);

        pgc_stats_index_judy_change(cache, mem_before_judyl, mem_after_judyl);

        page = *page_ptr;

        if (likely(!page)) {
#ifdef PGC_WITH_ARAL
            page = arrayalloc_mallocz(cache->aral);
#else
            page = callocz(1, sizeof(PGC_PAGE));
#endif
            page->refcount = 1;
            page->accesses = (entry->hot) ? 0 : 1;
            page->flags = PGC_PAGE_IS_BEING_CREATED;
            page->section = entry->section;
            page->metric_id = entry->metric_id;
            page->start_time_t = entry->start_time_t;
            page->end_time_t = entry->end_time_t,
            page->update_every = entry->update_every,
            page->data = entry->data;
            page->assumed_size = page_assumed_size(entry->size);
            netdata_spinlock_init(&page->transition_spinlock);
            page->link.prev = page->link.next = NULL;

            // put it in the index
            *page_ptr = page;
            pointer_add(cache, page);
            pgc_index_write_unlock(cache, partition);

            if (entry->hot)
                page_set_hot(cache, page);
            else
                page_set_clean(cache, page, false, false);

            page_flag_clear(page, PGC_PAGE_IS_BEING_CREATED);

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
            if (!page_acquire___while_having_some_lock(cache, page))
                page = NULL;

            else if(added)
                *added = false;

            pgc_index_write_unlock(cache, partition);

        }

    } while(!page);

    if(unlikely(spins > 1))
        __atomic_add_fetch(&cache->stats.insert_spins, spins - 1, __ATOMIC_RELAXED);

    if(entry->hot)
        evict_on_hot_page_added(cache);
    else
        evict_on_clean_page_added(cache);

    if((cache->config.options & PGC_OPTIONS_FLUSH_PAGES_INLINE) || flushing_critical(cache)) {
        flush_pages(cache, cache->config.max_flushes_inline,
                    false, false);
    }

    return page;
}

static PGC_PAGE *page_find_and_acquire(PGC *cache, Word_t section, Word_t metric_id, time_t start_time_t, bool exact) {
    size_t *stats_hit_ptr, *stats_miss_ptr;

    if(exact) {
        __atomic_add_fetch(&cache->stats.searches_exact, 1, __ATOMIC_RELAXED);
        stats_hit_ptr = &cache->stats.searches_exact_hits;
        stats_miss_ptr = &cache->stats.searches_exact_misses;
    }
    else {
        __atomic_add_fetch(&cache->stats.searches_closest, 1, __ATOMIC_RELAXED);
        stats_hit_ptr = &cache->stats.searches_closest_hits;
        stats_miss_ptr = &cache->stats.searches_closest_misses;
    }

    PGC_PAGE *page = NULL;
    size_t partition = indexing_partition(cache, metric_id);

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

    Pvoid_t *page_ptr = JudyLGet(*pages_judy_pptr, start_time_t, PJE0);
    if(unlikely(page_ptr == PJERR))
        fatal("DBENGINE CACHE: corrupted page in pages judy array");

    if(page_ptr) {
        // exact match on the timestamp
        page = *page_ptr;
    }
    else if(!exact) {
        Word_t time = start_time_t;

        // find the previous page
        page_ptr = JudyLLast(*pages_judy_pptr, &time, PJE0);
        if(unlikely(page_ptr == PJERR))
            fatal("DBENGINE CACHE: corrupted page in pages judy array #2");

        if(page_ptr) {
            // found a page starting before our timestamp
            // check if our timestamp is included
            page = *page_ptr;
            if(start_time_t > page->end_time_t)
                // it is not good for us
                page = NULL;
        }

        if(!page) {
            // find the next page then...
            time = start_time_t;
            page_ptr = JudyLNext(*pages_judy_pptr, &time, PJE0);
            if(page_ptr)
                page = *page_ptr;
        }
    }

    if(page) {
        pointer_check(cache, page);

        if(!page_acquire___while_having_some_lock(cache, page)) {
            // this page is not good to use
            page = NULL;
        }
    }

cleanup:
    pgc_index_read_unlock(cache, partition);

    if(page) {
        page_has_been_accessed(cache, page);

        __atomic_add_fetch(stats_hit_ptr, 1, __ATOMIC_RELAXED);
        evict_on_page_searched_and_found(cache);
    }
    else {
        __atomic_add_fetch(stats_miss_ptr, 1, __ATOMIC_RELAXED);
        evict_on_page_searched_and_not_found(cache);
    }

    return page;
}

static void all_hot_pages_to_dirty(PGC *cache) {
    pgc_ll_lock(cache, &cache->hot);

    PGC_PAGE *page = cache->hot.base;
    while(page) {
        PGC_PAGE *next = page->link.next;

        if(page_acquire___while_having_some_lock(cache, page)) {
            page_set_dirty(cache, page, true);
            page_release(cache, page, false);
            // page ptr may be invalid now
        }

        page = next;
    }

    pgc_ll_unlock(cache, &cache->hot);
}

// returns true when there is more work to do
static bool flush_pages(PGC *cache, size_t max_flushes, bool wait, bool all_of_them) {
    internal_fatal(!cache->dirty.linked_list_in_sections_judy,
                   "wrong dirty pages configuration - dirty pages need to have a judy array, not a linked list");

    if(!all_of_them && !wait) {
        // we have been called from a data collection thread
        // let's not waste its time...

        if(!pgc_ll_trylock(cache, &cache->dirty)) {
            // we would block, so give up...
            return true;
        }

        // we got the lock at this point
    }
    else
        pgc_ll_lock(cache, &cache->dirty);

    size_t optimal_flush_size = cache->config.max_dirty_pages_per_call;
    size_t dirty_version_at_entry = cache->dirty.version;
    if(!all_of_them && (cache->dirty.stats->entries < optimal_flush_size || cache->dirty.last_version_checked == dirty_version_at_entry)) {
        pgc_ll_unlock(cache, &cache->dirty);
        return false;
    }

    bool have_dirty_lock = true;

    if(all_of_them || !max_flushes)
        max_flushes = SIZE_MAX;

    Word_t last_section = 0;
    size_t flushes_so_far = 0;
    Pvoid_t *dirty_pages_pptr;
    bool stopped_before_finishing = false;
    bool first = true;

    while (have_dirty_lock && (dirty_pages_pptr = JudyLFirstThenNext(cache->dirty.sections_judy, &last_section, &first))) {

        if(!all_of_them && flushes_so_far > max_flushes) {
            stopped_before_finishing = true;
            break;
        }

        struct section_dirty_pages *sdp = *dirty_pages_pptr;

        PGC_ENTRY array[optimal_flush_size];
        PGC_PAGE *pages[optimal_flush_size];
        size_t added = 0, added_size = 0;

        PGC_PAGE *page = sdp->base;
        while (page && added < optimal_flush_size) {
            PGC_PAGE *next = page->link.next;

            internal_fatal(page_get_status_flags(page) != PGC_PAGE_DIRTY,
                           "DBENGINE CACHE: page should be in the dirty list before saved");

            if (page_acquire___while_having_some_lock(cache, page)) {
                internal_fatal(page_get_status_flags(page) != PGC_PAGE_DIRTY,
                               "DBENGINE CACHE: page should be in the dirty list before saved");

                internal_fatal(page->section != last_section,
                               "DBENGINE CACHE: dirty page is not in the right section (tier)");

                if(!page_transition_trylock(cache, page)) {
                    page_release(cache, page, false);
                    // page ptr may be invalid now
                }
                else {
                    pages[added] = page;
                    array[added] = (PGC_ENTRY) {
                            .section = page->section,
                            .metric_id = page->metric_id,
                            .start_time_t = page->start_time_t,
                            .end_time_t = __atomic_load_n(&page->end_time_t, __ATOMIC_RELAXED),
                            .update_every = page->update_every,
                            .size = page_size_from_assumed_size(page->assumed_size),
                            .data = page->data,
                            .hot = false,
                    };

                    added_size += array[added].size;
                    added++;
                }
            }

            page = next;
        }

        // do we have enough to save?
        if(all_of_them || added == optimal_flush_size) {
            // we should do it

            for (size_t i = 0; i < added; i++) {
                PGC_PAGE *tpg = pages[i];

                internal_fatal(page_get_status_flags(tpg) != PGC_PAGE_DIRTY,
                               "DBENGINE CACHE: page should be in the dirty list before saved");

                // remove it from the dirty list
                pgc_ll_del(cache, &cache->dirty, tpg, true);

                // mark it as being saved
                page_flag_set(tpg, PGC_PAGE_IS_BEING_SAVED);
            }

            // next time, repeat the same section (tier)
            first = true;
        }
        else {
            // we can't do it

            for (size_t i = 0; i < added; i++) {
                PGC_PAGE *tpg = pages[i];

                internal_fatal(page_get_status_flags(tpg) != PGC_PAGE_DIRTY,
                               "DBENGINE CACHE: page should be in the dirty list before saved");

                page_transition_unlock(cache, tpg);
                page_release(cache, tpg, false);
                // page ptr may be invalid now
            }

            __atomic_add_fetch(&cache->stats.flushes_cancelled, added, __ATOMIC_RELAXED);
            __atomic_add_fetch(&cache->stats.flushes_cancelled_size, added_size, __ATOMIC_RELAXED);

            added = 0;

            // next time, continue to the next section (tier)
            first = false;
            continue;
        }

        pgc_ll_unlock(cache, &cache->dirty);
        have_dirty_lock = false;

        // call the callback to save them
        // it may take some time, so let's release the lock
        cache->config.pgc_save_dirty_cb(cache, array, added);
        flushes_so_far++;

        pgc_ll_lock(cache, &cache->clean);

        for (size_t i = 0; i < added; i++) {
            PGC_PAGE *tpg = pages[i];

            internal_fatal(page_get_status_flags(tpg) != 0,
                           "DBENGINE CACHE: page should not be in any list while it is being saved");

            page_set_clean(cache, tpg, true, true);

            page_flag_clear(tpg, PGC_PAGE_IS_BEING_SAVED);
            page_transition_unlock(cache, tpg);
            page_release(cache, tpg, false);
            // tpg ptr may be invalid now
        }

        pgc_ll_unlock(cache, &cache->clean);
        __atomic_add_fetch(&cache->stats.flushes_completed, added, __ATOMIC_RELAXED);
        __atomic_add_fetch(&cache->stats.flushes_completed_size, added_size, __ATOMIC_RELAXED);

        if(!all_of_them && !wait) {
            if(pgc_ll_trylock(cache, &cache->dirty))
                have_dirty_lock = true;

            else {
                stopped_before_finishing = true;
                have_dirty_lock = false;
            }
        }
        else {
            pgc_ll_lock(cache, &cache->dirty);
            have_dirty_lock = true;
        }
    }

    if(!stopped_before_finishing)
        cache->dirty.last_version_checked = dirty_version_at_entry;

    if(have_dirty_lock)
        pgc_ll_unlock(cache, &cache->dirty);

    return stopped_before_finishing;
}

void free_all_unreferenced_clean_pages(PGC *cache) {
    evict_pages(cache, 0, 0, true, true);
}

// ----------------------------------------------------------------------------
// public API

PGC *pgc_create(size_t clean_size_bytes, free_clean_page_callback pgc_free_cb,
                size_t max_dirty_pages_per_call, save_dirty_page_callback pgc_save_dirty_cb,
                size_t max_pages_per_inline_eviction, size_t max_skip_pages_per_inline_eviction,
                size_t max_flushes_inline,
                PGC_OPTIONS options, size_t partitions) {

    PGC *cache = callocz(1, sizeof(PGC));
    cache->config.options = options;
    cache->config.clean_size = (clean_size_bytes < 8 * 1024 * 1024) ? 8 * 1024 * 1024 : clean_size_bytes;
    cache->config.pgc_free_clean_cb = pgc_free_cb;
    cache->config.max_dirty_pages_per_call = max_dirty_pages_per_call,
    cache->config.pgc_save_dirty_cb = pgc_save_dirty_cb;
    cache->config.max_pages_per_inline_eviction = (max_pages_per_inline_eviction < 1) ? 1 : max_pages_per_inline_eviction;
    cache->config.max_skip_pages_per_inline_eviction = (max_skip_pages_per_inline_eviction < 1) ? 1 : max_skip_pages_per_inline_eviction;
    cache->config.max_flushes_inline = (max_flushes_inline < 1) ? 1 : max_flushes_inline;
    cache->config.partitions = partitions < 1 ? (size_t)get_system_cpus() : partitions;

    cache->index = callocz(cache->config.partitions, sizeof(struct pgc_index));

    for(size_t part = 0; part < cache->config.partitions ; part++)
        netdata_rwlock_init(&cache->index[part].rwlock);

    netdata_spinlock_init(&cache->hot.spinlock);
    netdata_spinlock_init(&cache->dirty.spinlock);
    netdata_spinlock_init(&cache->clean.spinlock);

    cache->dirty.linked_list_in_sections_judy = true;

    cache->hot.flags = PGC_PAGE_HOT;
    cache->dirty.flags = PGC_PAGE_DIRTY;
    cache->clean.flags = PGC_PAGE_CLEAN;

    cache->hot.stats = &cache->stats.queues.hot;
    cache->dirty.stats = &cache->stats.queues.dirty;
    cache->clean.stats = &cache->stats.queues.clean;

#ifdef PGC_WITH_ARAL
    cache->aral = arrayalloc_create(sizeof(PGC_PAGE), max_clean_size / sizeof(PGC_PAGE) / 3,
                                    NULL, NULL, false, false);
#endif

    pointer_index_init(cache);

    return cache;
}

void pgc_destroy(PGC *cache) {
    // convert all hot pages to dirty
    all_hot_pages_to_dirty(cache);

    // save all dirty pages to make them clean
    flush_pages(cache, 0, true, true);

    // free all unreferenced clean pages
    free_all_unreferenced_clean_pages(cache);

    if(PGC_REFERENCED_PAGES(cache))
        error("DBENGINE CACHE: there are %zu referenced cache pages - leaving the cache allocated", PGC_REFERENCED_PAGES(cache));
    else {
        pointer_destroy_index(cache);
#ifdef PGC_WITH_ARAL
        arrayalloc_destroy(cache->aral);
#endif
        freez(cache);
    }
}

PGC_PAGE *pgc_page_add_and_acquire(PGC *cache, PGC_ENTRY entry, bool *added) {
    return page_add(cache, &entry, added);
}

void pgc_page_release(PGC *cache, PGC_PAGE *page) {
    page_release(cache, page, is_page_clean(page));
}

void pgc_page_hot_to_dirty_and_release(PGC *cache, PGC_PAGE *page) {
    if(!is_page_hot(page))
        fatal("DBENGINE CACHE: called %s() but page is not hot", __FUNCTION__ );

    // make page dirty
    page_set_dirty(cache, page, false);

    // release the page
    page_release(cache, page, true);
    // page ptr may be invalid now

    // flush, if we have to
    if((cache->config.options & PGC_OPTIONS_FLUSH_PAGES_INLINE) || flushing_critical(cache)) {
        flush_pages(cache, cache->config.max_flushes_inline,
                    false, false);
    }
}

void pgc_page_hot_to_clean_empty_and_release(PGC *cache, PGC_PAGE *page) {
    if(!is_page_hot(page))
        fatal("DBENGINE CACHE: set empty on non-hot page");

    // prevent accesses from increasing the accesses counter
    page_flag_set(page, PGC_PAGE_HAS_NO_DATA_IGNORE_ACCESSES);

    // zero the accesses counter
    __atomic_store_n(&page->accesses, 0, __ATOMIC_RELEASE);

    // if there are no other references to it, evict it immediately
    if(make_acquired_page_clean_and_evict_or_page_release(cache, page))
        __atomic_add_fetch(&cache->stats.hot_empty_pages_evicted_immediately, 1, __ATOMIC_RELAXED);
    else
        __atomic_add_fetch(&cache->stats.hot_empty_pages_evicted_later, 1, __ATOMIC_RELAXED);

    // flush, if we have to
    if(cache->config.options & PGC_OPTIONS_FLUSH_PAGES_INLINE)
        flush_pages(cache, cache->config.max_flushes_inline, false, false);
}

Word_t pgc_page_section(PGC_PAGE *page) {
    return page->section;
}

Word_t pgc_page_metric(PGC_PAGE *page) {
    return page->metric_id;
}

time_t pgc_page_start_time_t(PGC_PAGE *page) {
    return page->start_time_t;
}

time_t pgc_page_end_time_t(PGC_PAGE *page) {
    return page->end_time_t;
}

time_t pgc_page_update_every(PGC_PAGE *page) {
    return page->update_every;
}

void *pgc_page_data(PGC_PAGE *page) {
    return page->data;
}

size_t pgc_page_data_size(PGC_PAGE *page) {
    return page_size_from_assumed_size(page->assumed_size);
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

bool pgc_evict_pages(PGC *cache, size_t max_skip, size_t max_evict) {
    bool under_pressure = cache_under_severe_pressure(cache);
    return evict_pages(cache,
                       under_pressure ? 0 : max_skip,
                       under_pressure ? 0 : max_evict,
                       true, false);
}

bool pgc_flush_pages(PGC *cache, size_t max_flushes) {
    bool under_pressure = flushing_critical(cache);
    return flush_pages(cache, under_pressure ? 0 : max_flushes, true, false);
}

void pgc_page_hot_set_end_time_t(PGC *cache, PGC_PAGE *page, time_t end_time_t) {
    if(!is_page_hot(page))
        fatal("DBENGINE CACHE: end_time_t update on non-hot page");

//    if(end_time_t <= __atomic_load_n(&page->end_time_t, __ATOMIC_RELAXED))
//        fatal("DBENGINE CACHE: end_time_t is not bigger than existing");

    __atomic_store_n(&page->end_time_t, end_time_t, __ATOMIC_RELAXED);
    __atomic_add_fetch(&cache->stats.points_collected, 1, __ATOMIC_RELAXED);
}

PGC_PAGE *pgc_page_get_and_acquire(PGC *cache, Word_t section, Word_t metric_id, time_t start_time_t, bool exact) {
    return page_find_and_acquire(cache, section, metric_id, start_time_t, exact);
}

struct pgc_statistics pgc_get_statistics(PGC *cache) {
    // FIXME - get the statistics atomically
    struct pgc_statistics stats = cache->stats;

    return stats;
}


// ----------------------------------------------------------------------------
// unittest

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
    heartbeat_init(&hb);

    while(!__atomic_load_n(&pgc_uts.stop, __ATOMIC_RELAXED)) {
        // info("COLLECTOR %zu: collecting metrics %zu to %zu, from %ld to %lu", id, metric_start, metric_end, start_time_t, start_time_t + pgc_uts.points_per_page);

        netdata_thread_disable_cancelability();

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
            heartbeat_next(&hb, pgc_uts.time_per_collection_ut);

            for (size_t i = metric_start; i < metric_end; i++) {
                if(pgc_uts.metrics[i])
                    pgc_page_hot_set_end_time_t(pgc_uts.cache, pgc_uts.metrics[i], start_time_t);
            }

            __atomic_store_n(&pgc_uts.last_time_t, start_time_t, __ATOMIC_RELAXED);
        }

        for (size_t i = metric_start; i < metric_end; i++) {
            if (pgc_uts.metrics[i]) {
                if(i % 10 == 0)
                    pgc_page_hot_to_clean_empty_and_release(pgc_uts.cache, pgc_uts.metrics[i]);
                else
                    pgc_page_hot_to_dirty_and_release(pgc_uts.cache, pgc_uts.metrics[i]);
            }
        }

        netdata_thread_enable_cancelability();
    }

    return ptr;
}

void *unittest_stress_test_queries(void *ptr) {
    size_t id = *((size_t *)ptr);
    struct random_data *random_data = &pgc_uts.random_data[id];

    size_t start = 0;
    size_t end = pgc_uts.clean_metrics + pgc_uts.hot_metrics;

    while(!__atomic_load_n(&pgc_uts.stop, __ATOMIC_RELAXED)) {
        netdata_thread_disable_cancelability();

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
                                                page_start_time, i < pages - 1);
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

        netdata_thread_enable_cancelability();
    }

    return ptr;
}

void *unittest_stress_test_service(void *ptr) {
    heartbeat_t hb;
    heartbeat_init(&hb);
    while(!__atomic_load_n(&pgc_uts.stop, __ATOMIC_RELAXED)) {
        heartbeat_next(&hb, 1 * USEC_PER_SEC);

        pgc_flush_pages(pgc_uts.cache, 1000);
        pgc_evict_pages(pgc_uts.cache, 0, 0);
    }
    return ptr;
}

static void unittest_free_clean_page_callback(PGC *cache __maybe_unused, PGC_ENTRY entry __maybe_unused) {
    // info("FREE clean page section %lu, metric %lu, start_time %lu, end_time %lu", entry.section, entry.metric_id, entry.start_time_t, entry.end_time_t);
    ;
}

static void unittest_save_dirty_page_callback(PGC *cache __maybe_unused, PGC_ENTRY *array __maybe_unused, size_t entries __maybe_unused) {
    // info("SAVE %zu pages", entries);
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
                               64, unittest_save_dirty_page_callback,
                               1000, 10000, 1,
                               pgc_uts.options, pgc_uts.partitions);

    pgc_uts.metrics = callocz(pgc_uts.clean_metrics + pgc_uts.hot_metrics, sizeof(PGC_PAGE *));

    pthread_t service_thread;
    netdata_thread_create(&service_thread, "SERVICE",
                          NETDATA_THREAD_OPTION_JOINABLE | NETDATA_THREAD_OPTION_DONT_LOG,
                          unittest_stress_test_service, NULL);

    pthread_t collect_threads[pgc_uts.collect_threads];
    size_t collect_thread_ids[pgc_uts.collect_threads];
    for(size_t i = 0; i < pgc_uts.collect_threads ;i++) {
        collect_thread_ids[i] = i;
        char buffer[100 + 1];
        snprintfz(buffer, 100, "COLLECT_%zu", i);
        netdata_thread_create(&collect_threads[i], buffer,
                              NETDATA_THREAD_OPTION_JOINABLE | NETDATA_THREAD_OPTION_DONT_LOG,
                              unittest_stress_test_collector, &collect_thread_ids[i]);
    }

    pthread_t queries_threads[pgc_uts.query_threads];
    size_t query_thread_ids[pgc_uts.query_threads];
    pgc_uts.random_data = callocz(pgc_uts.query_threads, sizeof(struct random_data));
    for(size_t i = 0; i < pgc_uts.query_threads ;i++) {
        query_thread_ids[i] = i;
        char buffer[100 + 1];
        snprintfz(buffer, 100, "QUERY_%zu", i);
        initstate_r(1, pgc_uts.rand_statebufs, 1024, &pgc_uts.random_data[i]);
        netdata_thread_create(&queries_threads[i], buffer,
                              NETDATA_THREAD_OPTION_JOINABLE | NETDATA_THREAD_OPTION_DONT_LOG,
                              unittest_stress_test_queries, &query_thread_ids[i]);
    }

    heartbeat_t hb;
    heartbeat_init(&hb);

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
        heartbeat_next(&hb, 1 * USEC_PER_SEC);

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
        stats.events_cache_needs_space_90 = __atomic_load_n(&pgc_uts.cache->stats.events_cache_needs_space_90, __ATOMIC_RELAXED);
        stats.events_flush_critical = __atomic_load_n(&pgc_uts.cache->stats.events_flush_critical, __ATOMIC_RELAXED);

        size_t searches_exact = stats.searches_exact - old_stats.searches_exact;
        size_t searches_closest = stats.searches_closest - old_stats.searches_closest;

        size_t hit_exact = stats.searches_exact_hits - old_stats.searches_exact_hits;
        size_t hit_closest = stats.searches_closest_hits - old_stats.searches_closest_hits;

        double hit_exact_pc = (searches_exact > 0) ? (double)hit_exact * 100.0 / (double)searches_exact : 0.0;
        double hit_closest_pc = (searches_closest > 0) ? (double)hit_closest * 100.0 / (double)searches_closest : 0.0;

        stats.collections = __atomic_load_n(&pgc_uts.cache->stats.points_collected, __ATOMIC_RELAXED);

        char *cache_status = "N";
        if(stats.events_cache_under_severe_pressure > old_stats.events_cache_under_severe_pressure)
            cache_status = "F";
        else if(stats.events_cache_needs_space_90 > old_stats.events_cache_needs_space_90)
            cache_status = "f";

        char *flushing_status = "N";
        if(stats.events_flush_critical > old_stats.events_flush_critical)
            flushing_status = "F";

        info("PGS %5zuk +%4zuk/-%4zuk "
             "| RF %5zuk "
             "| HOT %5zuk +%4zuk -%4zuk "
             "| DRT %s %5zuk +%4zuk -%4zuk "
             "| CLN %s %5zuk +%4zuk -%4zuk "
             "| SRCH %4zuk %4zuk, HIT %4.1f%% %4.1f%% "
             "| CLCT %8.4f Mps"
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
             , (double)(stats.collections - old_stats.collections) / 1000.0 / 1000.0
             );
    }
    info("Waiting for threads to stop...");
    __atomic_store_n(&pgc_uts.stop, true, __ATOMIC_RELAXED);

    netdata_thread_join(service_thread, NULL);

    for(size_t i = 0; i < pgc_uts.collect_threads ;i++)
        netdata_thread_join(collect_threads[i],NULL);

    for(size_t i = 0; i < pgc_uts.query_threads ;i++)
        netdata_thread_join(queries_threads[i],NULL);

    pgc_destroy(pgc_uts.cache);

    freez(pgc_uts.metrics);
    freez(pgc_uts.random_data);
}

int pgc_unittest(void) {
    mallopt(M_PERTURB, 0x5A);
    // mallopt(M_MXFAST, 0);

    PGC *cache = pgc_create(32 * 1024 * 1024, unittest_free_clean_page_callback,
                            64, unittest_save_dirty_page_callback,
                            10, 1000, 10,
                            PGC_OPTIONS_DEFAULT, 1);

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
        .start_time_t = 100,
        .end_time_t = 1000,
        .size = 4096,
        .data = NULL,
        .hot = false,
    }, NULL);

    pgc_page_release(cache, page1);

    PGC_PAGE *page2 = pgc_page_add_and_acquire(cache, (PGC_ENTRY){
            .section = 2,
            .metric_id = 10,
            .start_time_t = 1001,
            .end_time_t = 2000,
            .size = 4096,
            .data = NULL,
            .hot = true,
    }, NULL);

    pgc_page_hot_set_end_time_t(cache, page2, 2001);
    pgc_page_hot_to_dirty_and_release(cache, page2);

    PGC_PAGE *page3 = pgc_page_add_and_acquire(cache, (PGC_ENTRY){
            .section = 3,
            .metric_id = 10,
            .start_time_t = 1001,
            .end_time_t = 2000,
            .size = 4096,
            .data = NULL,
            .hot = true,
    }, NULL);

    pgc_page_hot_set_end_time_t(cache, page3, 2001);
    pgc_page_hot_to_dirty_and_release(cache, page3);

    pgc_destroy(cache);

    unittest_stress_test();
    return 0;
}
