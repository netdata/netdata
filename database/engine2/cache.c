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
} PGC_PAGE_FLAGS;

#define page_flag_check(page, flag) (__atomic_load_n(&((page)->flags), __ATOMIC_RELAXED) & (flag))
#define page_flag_set(page, flag)   __atomic_or_fetch(&((page)->flags), flag, __ATOMIC_RELAXED)
#define page_flag_clear(page, flag) __atomic_and_fetch(&((page)->flags), ~(flag), __ATOMIC_RELAXED)

#define is_page_hot(page) (page_flag_check(page, PGC_PAGE_HOT | PGC_PAGE_DIRTY | PGC_PAGE_CLEAN) == PGC_PAGE_HOT)
#define is_page_dirty(page) (page_flag_check(page, PGC_PAGE_HOT | PGC_PAGE_DIRTY | PGC_PAGE_CLEAN) == PGC_PAGE_DIRTY)
#define is_page_clean(page) (page_flag_check(page, PGC_PAGE_HOT | PGC_PAGE_DIRTY | PGC_PAGE_CLEAN) == PGC_PAGE_CLEAN)

struct pgc_page {
    // indexing data
    Word_t section;
    Word_t metric_id;
    time_t start_time_t;
    time_t end_time_t;
    time_t update_every;

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
    size_t entries;
    size_t size;
    size_t added_entries;
    size_t added_size;
    size_t removed_entries;
    size_t removed_size;
    union {
        PGC_PAGE *base;
        Pvoid_t sections_judy;
    };
    PGC_PAGE_FLAGS flags;
    size_t version;
    size_t last_version_checked;
    bool linked_list_in_sections_judy; // when true, we use 'sections_judy', otherwise we use 'base'
};

struct pgc {
    struct {
        size_t partitions;
        size_t max_clean_size;
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

    SPINLOCK evictions_spinlock;
    SPINLOCK flushing_spinlock;

    struct {
        // size_t last_unique_id;          // each page gets a unique id, every time it is added to the cache

        size_t added_entries;
        size_t added_size;

        size_t removed_entries;
        size_t removed_size;

        size_t entries;                 // all the entries (includes clean, dirty, host)
        size_t size;                    // all the entries (includes clean, dirty, host)

        size_t referenced_entries;      // all the entries currently referenced
        size_t referenced_size;         // all the entries currently referenced

        size_t searches_exact;
        size_t searches_exact_hits;
        size_t searches_exact_misses;

        size_t searches_closest;
        size_t searches_closest_hits;
        size_t searches_closest_misses;

        size_t flushes_completed;
        size_t flushes_completed_size;
        size_t flushes_cancelled;
        size_t flushes_cancelled_size;

        size_t points_collected;

        size_t search_spins;
        size_t insert_spins;
        size_t evict_spins;
        size_t release_spins;
        size_t acquire_spins;
        size_t delete_spins;

        size_t evict_skipped;
        size_t *pages_added_per_partition;
    } stats;

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

    uint8_t bytes[sizeof(Word_t)];
    Word_t *p = (Word_t *)bytes;
    *p = metric_id;

    uint8_t *s = bytes;
    uint8_t *e = &bytes[sizeof(Word_t) - 1];

    size_t total = 0;
    while(s <= e)
        total += *s++;

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

static inline bool pgc_evictions_trylock(PGC *cache) {
    return netdata_spinlock_trylock(&cache->evictions_spinlock);
}

static inline void pgc_evictions_lock(PGC *cache) {
    netdata_spinlock_lock(&cache->evictions_spinlock);
}

static inline void pgc_evictions_unlock(PGC *cache) {
    netdata_spinlock_unlock(&cache->evictions_spinlock);
}

static inline bool pgc_flushing_trylock(PGC *cache) {
    return netdata_spinlock_trylock(&cache->flushing_spinlock);
}

static inline void pgc_flushing_lock(PGC *cache) {
    netdata_spinlock_lock(&cache->flushing_spinlock);
}

static inline void pgc_flushing_unlock(PGC *cache) {
    netdata_spinlock_unlock(&cache->flushing_spinlock);
}

// ----------------------------------------------------------------------------
// helpers

static inline size_t CACHE_CURRENT_CLEAN_SIZE(PGC *cache) {
    return __atomic_load_n(&cache->clean.size, __ATOMIC_RELAXED);
}

#define is_pgc_full(cache) (CACHE_CURRENT_CLEAN_SIZE(cache) > (cache)->config.max_clean_size)
#define is_pgc_90full(cache) (CACHE_CURRENT_CLEAN_SIZE(cache) > ((cache)->config.max_clean_size * 9 / 10))

static bool evict_this_page(PGC *cache, PGC_PAGE *page);
static bool evict_pages(PGC *cache, size_t max_skip, size_t max_evict, bool wait, bool all_of_them);

static size_t page_assumed_size(size_t size) {
    return size + sizeof(PGC_PAGE) + sizeof(Word_t) * 3;
}

static size_t page_size_from_assumed_size(size_t assumed_size) {
    return assumed_size - sizeof(PGC_PAGE) - sizeof(Word_t) * 3;
}

// ----------------------------------------------------------------------------
// Linked list management

struct section_dirty_pages {
    size_t entries;
    size_t size;
    PGC_PAGE *base;
};

static void pgc_ll_add(PGC *cache __maybe_unused, struct pgc_linked_list *ll, PGC_PAGE *page, bool having_lock) {
    if(!having_lock)
        pgc_ll_lock(cache, ll);

    internal_fatal(page_flag_check(page, PGC_PAGE_HOT | PGC_PAGE_DIRTY | PGC_PAGE_CLEAN) != 0,
                   "DBENGINE CACHE: invalid page flags, the page has %d, but it is should be %d",
                   page_flag_check(page, PGC_PAGE_HOT | PGC_PAGE_DIRTY | PGC_PAGE_CLEAN),
                   0);

    if(ll->linked_list_in_sections_judy) {
        Pvoid_t *dirty_pages_pptr = JudyLIns(&ll->sections_judy, page->section, PJE0);
        struct section_dirty_pages *sdp = *dirty_pages_pptr;

        if(!sdp) {
            sdp = callocz(1, sizeof(struct section_dirty_pages));
            *dirty_pages_pptr = sdp;
        }

        sdp->entries++;
        sdp->size += page->assumed_size;
        DOUBLE_LINKED_LIST_APPEND_UNSAFE(sdp->base, page, link.prev, link.next);
    }
    else {
        DOUBLE_LINKED_LIST_APPEND_UNSAFE(ll->base, page, link.prev, link.next);
    }

    ll->version++;

    page_flag_set(page, ll->flags);

    if(!having_lock)
        pgc_ll_unlock(cache, ll);

    __atomic_add_fetch(&ll->entries, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ll->size, page->assumed_size, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ll->added_entries, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ll->added_size, page->assumed_size, __ATOMIC_RELAXED);
}

static void pgc_ll_del(PGC *cache __maybe_unused, struct pgc_linked_list *ll, PGC_PAGE *page, bool having_lock) {
    __atomic_sub_fetch(&ll->entries, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&ll->size, page->assumed_size, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ll->removed_entries, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ll->removed_size, page->assumed_size, __ATOMIC_RELAXED);

    if(!having_lock)
        pgc_ll_lock(cache, ll);

    internal_fatal(page_flag_check(page, PGC_PAGE_HOT | PGC_PAGE_DIRTY | PGC_PAGE_CLEAN) != ll->flags,
                   "DBENGINE CACHE: invalid page flags, the page has %d, but it is should be %d",
                   page_flag_check(page, PGC_PAGE_HOT | PGC_PAGE_DIRTY | PGC_PAGE_CLEAN),
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
            if(!JudyLDel(&ll->sections_judy, page->section, PJE0))
                fatal("DBENGINE CACHE: cannot delete section from Judy LL");

            freez(sdp);
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
    if(is_page_clean(page)) {
        if(!pgc_ll_trylock(cache, &cache->clean))
            // it is locked, don't bother...
            return;

        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(cache->clean.base, page, link.prev, link.next);
        DOUBLE_LINKED_LIST_APPEND_UNSAFE(cache->clean.base, page, link.prev, link.next);
        pgc_ll_unlock(cache, &cache->clean);
    }
}


// ----------------------------------------------------------------------------
// state transitions

static inline void page_clear_clean(PGC *cache, PGC_PAGE *page) {
    if(!is_page_clean(page)) return;
    pgc_ll_del(cache, &cache->clean, page, false);
}

static void page_clear_dirty(PGC *cache, PGC_PAGE *page) {
    if(!is_page_dirty(page)) return;
    pgc_ll_del(cache, &cache->dirty, page, false);
}

static inline void page_clear_hot(PGC *cache, PGC_PAGE *page, bool having_hot_lock) {
    if(!is_page_hot(page)) return;
    pgc_ll_del(cache, &cache->hot, page, having_hot_lock);
}

static inline void page_set_clean(PGC *cache, PGC_PAGE *page, bool having_transition_lock, bool having_clean_lock) {
    if(!having_transition_lock)
        page_transition_lock(cache, page);

    if(is_page_clean(page)) {
        if(!having_transition_lock)
            page_transition_unlock(cache, page);
        return;
    }

    page_clear_hot(cache, page, false);
    page_clear_dirty(cache, page);

    // first add to linked list, the set the flag (required for move_page_last())
    pgc_ll_add(cache, &cache->clean, page, having_clean_lock);

    if(!having_transition_lock)
        page_transition_unlock(cache, page);
}

static void page_set_dirty(PGC *cache, PGC_PAGE *page, bool having_hot_lock) {
    page_transition_lock(cache, page);

    if(is_page_dirty(page)) {
        page_transition_unlock(cache, page);
        return;
    }

    page_clear_hot(cache, page, having_hot_lock);
    page_clear_clean(cache, page);

    // first add to linked list, the set the flag (required for move_page_last())
    pgc_ll_add(cache, &cache->dirty, page, false);

    page_transition_unlock(cache, page);
}

static inline void page_set_hot(PGC *cache, PGC_PAGE *page) {
    page_transition_lock(cache, page);

    if(is_page_hot(page)) {
        page_transition_unlock(cache, page);
        return;
    }

    page_clear_dirty(cache, page);
    page_clear_clean(cache, page);

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

static inline void PGC_REFERENCED_PAGES_MINUS1(PGC *cache, PGC_PAGE *page) {
    __atomic_sub_fetch(&cache->stats.referenced_entries, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&cache->stats.referenced_size, page->assumed_size, __ATOMIC_RELAXED);
}

static inline bool page_acquire(PGC *cache, PGC_PAGE *page) {
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

    page_has_been_accessed(cache, page);

    return true;
}

static inline void page_release(PGC *cache, PGC_PAGE *page, bool evict_if_necessary) {
    REFCOUNT expected, desired;

    expected = __atomic_load_n(&page->refcount, __ATOMIC_RELAXED);

    size_t spins = 0;
    do {
        spins++;

        if(unlikely(expected <= 0))
            fatal("DBENGINE CACHE: trying to release a page with reference counter %d", expected);

        desired = expected - 1;

    } while(!__atomic_compare_exchange_n(&page->refcount, &expected, desired, false, __ATOMIC_RELEASE, __ATOMIC_RELAXED));

    if(unlikely(spins > 1))
        __atomic_add_fetch(&cache->stats.release_spins, spins - 1, __ATOMIC_RELAXED);

    if(desired == 0) {
        PGC_REFERENCED_PAGES_MINUS1(cache, page);

        if(unlikely(evict_if_necessary && is_pgc_full(cache)))
            evict_this_page(cache, page);
    }
}

static inline bool page_get_for_deletion(PGC *cache __maybe_unused, PGC_PAGE *page) {
    REFCOUNT expected, desired = REFCOUNT_DELETING;

    expected = __atomic_load_n(&page->refcount, __ATOMIC_RELAXED);
    size_t spins = 0;

    do {
        spins++;

        if (expected != 0)
            return false;

    } while(!__atomic_compare_exchange_n(&page->refcount, &expected, desired, false, __ATOMIC_ACQUIRE, __ATOMIC_RELAXED));

    // we can delete this page

    if(!is_page_clean(page))
        fatal("DBENGINE CACHE: page to be deleted is not in the clean list");

    if (page_flag_check(page, PGC_PAGE_IS_BEING_DELETED))
        fatal("DBENGINE CACHE: page is already being deleted");

    page_flag_set(page, PGC_PAGE_IS_BEING_DELETED);

    if(unlikely(spins > 1))
        __atomic_add_fetch(&cache->stats.delete_spins, spins - 1, __ATOMIC_RELAXED);

    return true;
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

    if(unlikely(!JudyLDel(pages_judy_pptr, page->start_time_t, PJE0)))
        fatal("DBENGINE CACHE: page with start time '%ld' of metric '%lu' in section '%lu' exists, but cannot be deleted.",
              page->start_time_t, page->metric_id, page->section);

    if(!*pages_judy_pptr && !JudyLDel(metrics_judy_pptr, page->metric_id, PJE0))
        fatal("DBENGINE CACHE: metric '%lu' in section '%lu' exists and is empty, but cannot be deleted.",
              page->metric_id, page->section);

    if(!*metrics_judy_pptr && !JudyLDel(&cache->index[partition].sections_judy, page->section, PJE0))
        fatal("DBENGINE CACHE: section '%lu' exists and is empty, but cannot be deleted.", page->section);

    pointer_del(cache, page);
}

static bool evict_this_page(PGC *cache, PGC_PAGE *page) {
    pointer_check(cache, page);

    if(!is_page_clean(page))
        return false;

    if(!page_get_for_deletion(cache, page))
        return false;

    // remove it from the linked list
    pgc_ll_del(cache, &cache->clean, page, false);

    size_t partition = indexing_partition(cache, page->metric_id);
    pgc_index_write_lock(cache, partition);
    remove_this_page_from_index_unsafe(cache, page, partition);
    pgc_index_write_unlock(cache, partition);
    free_this_page(cache, page);

    return true;
}

// returns true, when there is more work to do
static bool evict_pages(PGC *cache, size_t max_skip, size_t max_evict, bool wait, bool all_of_them) {
    if(!all_of_them && !is_pgc_90full(cache))
        // don't bother - not enough to do anything
        return false;

    internal_fatal(cache->clean.linked_list_in_sections_judy,
                   "wrong clean pages configuration - clean pages need to have a linked list, not a judy array");

    if(unlikely(!max_skip))
        max_skip = SIZE_MAX;

    if(unlikely(!max_evict))
        max_evict = SIZE_MAX;

    if(all_of_them || wait)
        // we really need the lock
        pgc_evictions_lock(cache);

    else if(!pgc_evictions_trylock(cache))
        // another thread is evicting pages currently
        return true;

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

        PGC_PAGE *first_skipped = NULL;

        pgc_ll_lock(cache, &cache->clean);
        for(PGC_PAGE *page = cache->clean.base; page ; ) {
            PGC_PAGE *next = page->link.next;

            if(page_get_for_deletion(cache, page)) {
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

                if(unlikely(page == first_skipped))
                    // we looped through all the ones to be skipped
                    break;

                if(unlikely(!first_skipped))
                    // remember this page, to stop iterating forever
                    first_skipped = page;

                // put it at the end of the clean list
                DOUBLE_LINKED_LIST_REMOVE_UNSAFE(cache->clean.base, page, link.prev, link.next);
                DOUBLE_LINKED_LIST_APPEND_UNSAFE(cache->clean.base, page, link.prev, link.next);

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
            size_t waiting = cache->config.partitions;
            while(waiting) {
                waiting = 0;

                for (size_t partition = 0; partition < cache->config.partitions; partition++) {
                    if (!partition_waiting[partition]) continue;

                    if(pgc_index_write_trylock(cache, partition)) {
                        for (PGC_PAGE *page = to_evict[partition]; page; page = page->link.next)
                            remove_this_page_from_index_unsafe(cache, page, partition);

                        pgc_index_write_unlock(cache, partition);
                        partition_waiting[partition] = false;
                    }
                    else
                        waiting++;
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

    } while(pages_to_evict && (all_of_them || (is_pgc_90full(cache) && total_pages_evicted < max_evict && total_pages_skipped < max_skip)));

    if(all_of_them && PGC_REFERENCED_PAGES(cache)) {
        error_limit_static_global_var(erl, 1, 0);
        error_limit(&erl, "DBENGINE CACHE: cannot free all clean pages, some are still referenced");
    }
    else if(!total_pages_evicted && is_pgc_full(cache)) {
        error_limit_static_global_var(erl, 1, 0);
        error_limit(&erl,
                    "DBENGINE CACHE: cache size %zu, exceeds max size %zu, but all the data in it are currently referenced and cannot be evicted",
                    CACHE_CURRENT_CLEAN_SIZE(cache), cache->config.max_clean_size);
    }

    if(unlikely(total_pages_skipped))
        __atomic_add_fetch(&cache->stats.evict_skipped, total_pages_skipped, __ATOMIC_RELAXED);

    if(unlikely(spins > 1))
        __atomic_add_fetch(&cache->stats.evict_spins, spins - 1, __ATOMIC_RELAXED);

    pgc_evictions_unlock(cache);

    return stopped_before_finishing;
}

static PGC_PAGE *page_add(PGC *cache, PGC_ENTRY *entry) {
    PGC_PAGE *page;
    size_t spins = 0;

    do {
        spins++;

        size_t partition = indexing_partition(cache, entry->metric_id);
        pgc_index_write_lock(cache, partition);

        Pvoid_t *metrics_judy_pptr = JudyLIns(&cache->index[partition].sections_judy, entry->section, PJE0);
        if(unlikely(metrics_judy_pptr == PJERR))
            fatal("DBENGINE CACHE: corrupted sections judy array");

        Pvoid_t *pages_judy_pptr = JudyLIns(metrics_judy_pptr, entry->metric_id, PJE0);
        if(unlikely(pages_judy_pptr == PJERR))
            fatal("DBENGINE CACHE: corrupted pages judy array");

        Pvoid_t *page_ptr = JudyLIns(pages_judy_pptr, entry->start_time_t, PJE0);
        if(unlikely(page_ptr == PJERR))
            fatal("DBENGINE CACHE: corrupted page in judy array");

        page = *page_ptr;

        if (likely(!page)) {
#ifdef PGC_WITH_ARAL
            page = arrayalloc_mallocz(cache->aral);
#else
            page = callocz(1, sizeof(PGC_PAGE));
#endif
            page->refcount = 1;
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

            __atomic_add_fetch(&cache->stats.pages_added_per_partition[partition], 1, __ATOMIC_RELAXED);

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
        }
        else {
            pgc_index_write_unlock(cache, partition);

            if (!page_acquire(cache, page))
                page = NULL;
        }

    } while(!page);

    if(unlikely(spins > 1))
        __atomic_add_fetch(&cache->stats.insert_spins, spins - 1, __ATOMIC_RELAXED);

    if(!entry->hot && (cache->config.options & PGC_OPTIONS_EVICT_PAGES_INLINE))
        evict_pages(cache,
                    cache->config.max_skip_pages_per_inline_eviction,
                    cache->config.max_pages_per_inline_eviction,
                    false, false);

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
    size_t spins = 0;
    size_t partition = indexing_partition(cache, metric_id);
    bool try_again = false;

    do {
        spins++;

        pgc_index_read_lock(cache, partition);

        Pvoid_t *metrics_judy_pptr = JudyLGet(cache->index[partition].sections_judy, section, PJE0);
        if(unlikely(metrics_judy_pptr == PJERR))
            fatal("DBENGINE CACHE: corrupted sections judy array");

        if(unlikely(!metrics_judy_pptr)) {
            // section does not exist
            pgc_index_read_unlock(cache, partition);
            break;
        }

        Pvoid_t *pages_judy_pptr = JudyLGet(*metrics_judy_pptr, metric_id, PJE0);
        if(unlikely(pages_judy_pptr == PJERR))
            fatal("DBENGINE CACHE: corrupted pages judy array");

        if(unlikely(!pages_judy_pptr)) {
            // metric does not exist
            pgc_index_read_unlock(cache, partition);
            break;
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

        try_again = false;

        if(page) {
            pointer_check(cache, page);

            if(!page_acquire(cache, page)) {
                // this page is not good to use
                page = NULL;
                try_again = true;
            }
        }

        pgc_index_read_unlock(cache, partition);

    } while(!page && try_again);

    if(page)
        __atomic_add_fetch(stats_hit_ptr, 1, __ATOMIC_RELAXED);
    else
        __atomic_add_fetch(stats_miss_ptr, 1, __ATOMIC_RELAXED);

    if(unlikely(spins > 1))
        __atomic_add_fetch(&cache->stats.search_spins, spins - 1, __ATOMIC_RELAXED);

    return page;
}

static void all_hot_pages_to_dirty(PGC *cache) {
    pgc_ll_lock(cache, &cache->hot);

    PGC_PAGE *page = cache->hot.base;
    while(page) {
        PGC_PAGE *next = page->link.next;

        if(page_acquire(cache, page)) {
            page_set_dirty(cache, page, true);
            page_release(cache, page, false);
        }

        page = next;
    }

    pgc_ll_unlock(cache, &cache->hot);
}

static inline PPvoid_t JudyLFirstThenNext(Pcvoid_t PArray, Word_t * PIndex, bool *first) {
    if(unlikely(*first)) {
        *first = false;
        return JudyLFirst(PArray, PIndex, PJE0);
    }

    return JudyLNext(PArray, PIndex, PJE0);
}

// returns true when there is more work to do
static bool flush_pages(PGC *cache, size_t max_flushes, bool wait, bool all_of_them) {
    if(all_of_them || wait)
        pgc_flushing_lock(cache);

    else if(!pgc_flushing_trylock(cache))
        // another thread is flushing pages currently
        return true;

    internal_fatal(!cache->dirty.linked_list_in_sections_judy,
                   "wrong dirty pages configuration - dirty pages need to have a judy array, not a linked list");

    if(!all_of_them && !wait) {
        // we have been called from a data collection thread
        // let's not waste its time...

        if(!pgc_ll_trylock(cache, &cache->dirty)) {
            // we would block, so give up...
            pgc_flushing_unlock(cache);
            return true;
        }

        // we got the lock at this point
    }
    else
        pgc_ll_lock(cache, &cache->dirty);

    if(!all_of_them && (cache->dirty.entries < cache->config.max_dirty_pages_per_call || cache->dirty.last_version_checked == cache->dirty.version)) {
        pgc_ll_unlock(cache, &cache->dirty);
        pgc_flushing_unlock(cache);
        return false;
    }

    if(all_of_them || !max_flushes)
        max_flushes = SIZE_MAX;

    Word_t last_section = 0;
    size_t flushes_so_far = 0;
    Pvoid_t *dirty_pages_pptr;
    bool stopped_before_finishing = false;
    bool first = true;
    while ((dirty_pages_pptr = JudyLFirstThenNext(cache->dirty.sections_judy, &last_section, &first))) {

        if(!all_of_them && flushes_so_far > max_flushes) {
            stopped_before_finishing = true;
            break;
        }

        struct section_dirty_pages *sdp = *dirty_pages_pptr;
        PGC_PAGE *page = sdp->base;
        while (page && (all_of_them || sdp->entries >= cache->config.max_dirty_pages_per_call)) {
            PGC_ENTRY array[cache->config.max_dirty_pages_per_call];
            PGC_PAGE *pages[cache->config.max_dirty_pages_per_call];
            size_t used = 0, total_count = 0, total_size = 0;

            while (page && used < cache->config.max_dirty_pages_per_call) {
                PGC_PAGE *next = page->link.next;

                if (page_acquire(cache, page)) {
                    internal_fatal(page->section != last_section,
                                   "DBENGINE CACHE: dirty page is not in the right section");

                    if(!page_transition_trylock(cache, page)) {
                        page_release(cache, page, false);
                        page = next;
                        continue;
                    }

                    page_flag_set(page, PGC_PAGE_IS_BEING_SAVED);

                    pages[used] = page;
                    array[used] = (PGC_ENTRY) {
                            .section = page->section,
                            .metric_id = page->metric_id,
                            .start_time_t = page->start_time_t,
                            .end_time_t = __atomic_load_n(&page->end_time_t, __ATOMIC_RELAXED),
                            .update_every = page->update_every,
                            .size = page_size_from_assumed_size(page->assumed_size),
                            .data = page->data,
                            .hot = (is_page_hot(page)) ? true : false,
                    };

                    total_count++;
                    total_size += array[used].size;

                    used++;
                }

                page = next;
            }
            pgc_ll_unlock(cache, &cache->dirty);

            if (used) {
                bool clean_them = false;

                if (all_of_them || used == cache->config.max_dirty_pages_per_call) {
                    // call the callback to save them
                    // it may take some time, so let's release the lock
                    cache->config.pgc_save_dirty_cb(cache, array, used);
                    clean_them = true;
                    flushes_so_far++;
                }

                if(clean_them)
                    pgc_ll_lock(cache, &cache->clean);

                for (size_t i = 0; i < used; i++) {
                    PGC_PAGE *tpg = pages[i];

                    if (clean_them)
                        page_set_clean(cache, tpg, true, true);

                    page_transition_unlock(cache, tpg);
                    page_release(cache, tpg, false);
                    page_flag_clear(tpg, PGC_PAGE_IS_BEING_SAVED);
                }

                if(clean_them) {
                    pgc_ll_unlock(cache, &cache->clean);
                    __atomic_add_fetch(&cache->stats.flushes_completed, total_count, __ATOMIC_RELAXED);
                    __atomic_add_fetch(&cache->stats.flushes_completed_size, total_size, __ATOMIC_RELAXED);
                }
                else {
                    __atomic_add_fetch(&cache->stats.flushes_cancelled, total_count, __ATOMIC_RELAXED);
                    __atomic_add_fetch(&cache->stats.flushes_cancelled_size, total_size, __ATOMIC_RELAXED);
                }
            }

            pgc_ll_lock(cache, &cache->dirty);
        }
    }

    if(!stopped_before_finishing)
        cache->dirty.last_version_checked = cache->dirty.version;

    pgc_ll_unlock(cache, &cache->dirty);
    pgc_flushing_unlock(cache);

    return stopped_before_finishing;
}

void free_all_unreferenced_clean_pages(PGC *cache) {
    evict_pages(cache, 0, 0, true, true);
}

// ----------------------------------------------------------------------------
// public API

PGC *pgc_create(size_t max_clean_size, free_clean_page_callback pgc_free_cb,
                size_t max_dirty_pages_per_call, save_dirty_page_callback pgc_save_dirty_cb,
                size_t max_pages_per_inline_eviction, size_t max_skip_pages_per_inline_eviction,
                size_t max_flushes_inline,
                PGC_OPTIONS options, size_t partitions) {

    PGC *cache = callocz(1, sizeof(PGC));
    cache->config.options = options;
    cache->config.max_clean_size = max_clean_size;
    cache->config.pgc_free_clean_cb = pgc_free_cb;
    cache->config.max_dirty_pages_per_call = max_dirty_pages_per_call,
    cache->config.pgc_save_dirty_cb = pgc_save_dirty_cb;
    cache->config.max_pages_per_inline_eviction = max_pages_per_inline_eviction;
    cache->config.max_skip_pages_per_inline_eviction = max_skip_pages_per_inline_eviction;
    cache->config.max_flushes_inline = max_flushes_inline;
    cache->config.partitions = partitions;

    cache->index = callocz(cache->config.partitions, sizeof(struct pgc_index));
    cache->stats.pages_added_per_partition = callocz(cache->config.partitions, sizeof(size_t));

    for(size_t part = 0; part < cache->config.partitions ; part++)
        netdata_rwlock_init(&cache->index[part].rwlock);

    netdata_spinlock_init(&cache->hot.spinlock);
    netdata_spinlock_init(&cache->dirty.spinlock);
    netdata_spinlock_init(&cache->clean.spinlock);

    netdata_spinlock_init(&cache->evictions_spinlock);
    netdata_spinlock_init(&cache->flushing_spinlock);

    cache->dirty.linked_list_in_sections_judy = true;

    cache->hot.flags = PGC_PAGE_HOT;
    cache->dirty.flags = PGC_PAGE_DIRTY;
    cache->clean.flags = PGC_PAGE_CLEAN;

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

PGC_PAGE *pgc_page_add_and_acquire(PGC *cache, PGC_ENTRY entry) {
    return page_add(cache, &entry);
}

void pgc_page_release(PGC *cache, PGC_PAGE *page) {
    page_release(cache, page, is_page_clean(page));
}

void pgc_page_hot_to_dirty_and_release(PGC *cache, PGC_PAGE *page) {
    if(!is_page_hot(page))
        fatal("DBENGINE CACHE: called %s() but page is not hot", __FUNCTION__ );

    page_set_dirty(cache, page, false);
    page_release(cache, page, true);

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
    return evict_pages(cache, max_skip, max_evict, true, false);
}

bool pgc_flush_pages(PGC *cache, size_t max_flushes) {
    return flush_pages(cache, max_flushes, true, false);
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
    PGC_OPTIONS options;
    char rand_statebufs[1024];
    struct random_data *random_data;
} pgc_uts = {
        .stop            = false,
        .metrics         = NULL,
        .clean_metrics   =   1000000,
        .hot_metrics     =   5000000,
        .first_time_t    = 100000000,
        .last_time_t     = 0,
        .cache_size      = 4096,
        .collect_threads = 1000,
        .query_threads   = 5,
        .partitions      = 100,
        .options         = PGC_OPTIONS_NONE,/* PGC_OPTIONS_FLUSH_PAGES_INLINE | PGC_OPTIONS_EVICT_PAGES_INLINE,*/
        .points_per_page = 10,
        .time_per_collection_ut = 1000000,
        .time_per_query_ut = 500,
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
            pgc_uts.metrics[i] = pgc_page_add_and_acquire(pgc_uts.cache, (PGC_ENTRY) {
                    .section = 1,
                    .metric_id = i,
                    .start_time_t = start_time_t,
                    .end_time_t = start_time_t,
                    .update_every = 1,
                    .size = 4096,
                    .data = NULL,
                    .hot = true,
            });

            if(!pgc_is_page_hot(pgc_uts.metrics[i])) {
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
            if (pgc_uts.metrics[i])
                pgc_page_hot_to_dirty_and_release(pgc_uts.cache, pgc_uts.metrics[i]);
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
            });
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

        pgc_flush_pages(pgc_uts.cache, 1);
        pgc_evict_pages(pgc_uts.cache, 100000000, 100000000);
    }
    return ptr;
}

static void unittest_free_clean_page_callback(PGC *cache __maybe_unused, PGC_ENTRY entry __maybe_unused) {
    // info("FREE clean page section %lu, metric %lu, start_time %lu, end_time %lu", entry.section, entry.metric_id, entry.start_time_t, entry.end_time_t);
    ;
}

static void unittest_save_dirty_page_callback(PGC *cache __maybe_unused, PGC_ENTRY *array __maybe_unused, size_t entries __maybe_unused) {
    // info("SAVE %zu pages", entries);
//    if(!pgc_uts.stop) {
//        static const struct timespec work_duration = {.tv_sec = 0, .tv_nsec = 10000};
//        nanosleep(&work_duration, NULL);
//    }
    ;
}

void unittest_stress_test(void) {
    pgc_uts.cache = pgc_create(pgc_uts.cache_size * 1024 * 1024,
                               unittest_free_clean_page_callback,
                               64, unittest_save_dirty_page_callback,
                               10, 100, 2,
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
    } stats = {}, old_stats = {};

    for(int i = 0; i < 86400 ;i++) {
        heartbeat_next(&hb, 1 * USEC_PER_SEC);

        old_stats = stats;
        stats.entries       = __atomic_load_n(&pgc_uts.cache->stats.entries, __ATOMIC_RELAXED);
        stats.added         = __atomic_load_n(&pgc_uts.cache->stats.added_entries, __ATOMIC_RELAXED);
        stats.deleted       = __atomic_load_n(&pgc_uts.cache->stats.removed_entries, __ATOMIC_RELAXED);
        stats.referenced    = __atomic_load_n(&pgc_uts.cache->stats.referenced_entries, __ATOMIC_RELAXED);

        stats.hot_entries   = __atomic_load_n(&pgc_uts.cache->hot.entries, __ATOMIC_RELAXED);
        stats.hot_added     = __atomic_load_n(&pgc_uts.cache->hot.added_entries, __ATOMIC_RELAXED);
        stats.hot_deleted   = __atomic_load_n(&pgc_uts.cache->hot.removed_entries, __ATOMIC_RELAXED);

        stats.dirty_entries = __atomic_load_n(&pgc_uts.cache->dirty.entries, __ATOMIC_RELAXED);
        stats.dirty_added   = __atomic_load_n(&pgc_uts.cache->dirty.added_entries, __ATOMIC_RELAXED);
        stats.dirty_deleted = __atomic_load_n(&pgc_uts.cache->dirty.removed_entries, __ATOMIC_RELAXED);

        stats.clean_entries = __atomic_load_n(&pgc_uts.cache->clean.entries, __ATOMIC_RELAXED);
        stats.clean_added   = __atomic_load_n(&pgc_uts.cache->clean.added_entries, __ATOMIC_RELAXED);
        stats.clean_deleted = __atomic_load_n(&pgc_uts.cache->clean.removed_entries, __ATOMIC_RELAXED);

        stats.searches_exact = __atomic_load_n(&pgc_uts.cache->stats.searches_exact, __ATOMIC_RELAXED);
        stats.searches_exact_hits = __atomic_load_n(&pgc_uts.cache->stats.searches_exact_hits, __ATOMIC_RELAXED);

        stats.searches_closest = __atomic_load_n(&pgc_uts.cache->stats.searches_closest, __ATOMIC_RELAXED);
        stats.searches_closest_hits = __atomic_load_n(&pgc_uts.cache->stats.searches_closest_hits, __ATOMIC_RELAXED);

        size_t searches_exact = stats.searches_exact - old_stats.searches_exact;
        size_t searches_closest = stats.searches_closest - old_stats.searches_closest;

        size_t hit_exact = stats.searches_exact_hits - old_stats.searches_exact_hits;
        size_t hit_closest = stats.searches_closest_hits - old_stats.searches_closest_hits;

        double hit_exact_pc = (searches_exact > 0) ? (double)hit_exact * 100.0 / (double)searches_exact : 0.0;
        double hit_closest_pc = (searches_closest > 0) ? (double)hit_closest * 100.0 / (double)searches_closest : 0.0;

        stats.collections = __atomic_load_n(&pgc_uts.cache->stats.points_collected, __ATOMIC_RELAXED);

        info("PAGES %6zu k %s [+%5zu k/-%5zu k] "
             "| REF %6zu k "
             "| HOT %6zu k [+%5zu k/-%5zu k] "
             "| DIRTY %6zu k [+%5zu k/-%5zu k] "
             "| CLEAN %6zu k [+%5zu k/-%5zu k] "
             "| SEARCH %6zu k / %6zu k, HIT %5.1f %% %5.1f %%, COLL %5.1f M points"
             , stats.entries / 1000, is_pgc_full(pgc_uts.cache) ? "F" : is_pgc_90full(pgc_uts.cache) ? "f" : "N", (stats.added - old_stats.added) / 1000, (stats.deleted - old_stats.deleted) / 1000
             , stats.referenced / 1000
             , stats.hot_entries / 1000, (stats.hot_added - old_stats.hot_added) / 1000, (stats.hot_deleted - old_stats.hot_deleted) / 1000
             , stats.dirty_entries / 1000, (stats.dirty_added - old_stats.dirty_added) / 1000, (stats.dirty_deleted - old_stats.dirty_deleted) / 1000
             , stats.clean_entries / 1000, (stats.clean_added - old_stats.clean_added) / 1000, (stats.clean_deleted - old_stats.clean_deleted) / 1000
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
    });

    pgc_page_release(cache, page1);

    PGC_PAGE *page2 = pgc_page_add_and_acquire(cache, (PGC_ENTRY){
            .section = 2,
            .metric_id = 10,
            .start_time_t = 1001,
            .end_time_t = 2000,
            .size = 4096,
            .data = NULL,
            .hot = true,
    });

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
    });

    pgc_page_hot_set_end_time_t(cache, page3, 2001);
    pgc_page_hot_to_dirty_and_release(cache, page3);

    pgc_destroy(cache);

    unittest_stress_test();
    return 0;
}
