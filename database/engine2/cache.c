#include "cache.h"

typedef int32_t REFCOUNT;
#define REFCOUNT_DELETING (-100)

typedef enum __attribute__ ((__packed__)) {
    // mutually exclusive flags
    DBENGINE_PGC_PAGE_CLEAN     = (1 << 0), // none of the following
    DBENGINE_PGC_PAGE_DIRTY     = (1 << 1), // contains unsaved data
    DBENGINE_PGC_PAGE_HOT       = (1 << 2), // currently being collected
} PGC_PAGE_FLAGS;

#define page_flag_check(page, flag) (__atomic_load_n(&((page)->flags), __ATOMIC_SEQ_CST) & (flag))
#define page_flag_set(page, flag)   __atomic_or_fetch(&((page)->flags), flag, __ATOMIC_SEQ_CST)
#define page_flag_clear(page, flag) __atomic_and_fetch(&((page)->flags), ~(flag), __ATOMIC_SEQ_CST)

#define is_page_hot(page) page_flag_check(page, DBENGINE_PGC_PAGE_HOT)
#define is_page_dirty(page) page_flag_check(page, DBENGINE_PGC_PAGE_DIRTY)
#define is_page_clean(page) page_flag_check(page, DBENGINE_PGC_PAGE_CLEAN)

struct pgc_page {
    // indexing data
    Word_t section;
    Word_t metric_id;
    Word_t start_time_t;

    void *data;
    size_t assumed_size;
    REFCOUNT refcount;
    PGC_PAGE_FLAGS flags;

    struct {
        struct pgc_page *next;
        struct pgc_page *prev;
    } link;
};

struct pgc_linked_list {
    SPINLOCK spinlock;
    PGC_PAGE *base;
    size_t entries;
    size_t size;
};

struct pgc {
    struct {
        size_t max_size;
    } config;

    struct {
        netdata_rwlock_t rwlock;
        Pvoid_t sections_judy;
    } index;

    struct pgc_linked_list clean;
    struct pgc_linked_list dirty;
    struct pgc_linked_list hot;

    struct {
        size_t entries;                 // all the entries (includes clean, dirty, host)
        size_t size;                    // all the entries (includes clean, dirty, host)

        size_t referenced_entries;      // all the entries currently referenced
        size_t referenced_size;         // all the entries currently referenced
    } stats;
};

// ----------------------------------------------------------------------------
// locking

static void dbengine_cache_index_read_lock(PGC *cache) {
    netdata_rwlock_rdlock(&cache->index.rwlock);
}
static void dbengine_cache_index_read_unlock(PGC *cache) {
    netdata_rwlock_unlock(&cache->index.rwlock);
}
static void dbengine_cache_index_write_lock(PGC *cache) {
    netdata_rwlock_wrlock(&cache->index.rwlock);
}
static void dbengine_cache_index_write_unlock(PGC *cache) {
    netdata_rwlock_unlock(&cache->index.rwlock);
}


// ----------------------------------------------------------------------------
// helpers

static size_t page_assumed_size(size_t size) {
    return size + sizeof(PGC_PAGE) + sizeof(Word_t) * 3;
}


// ----------------------------------------------------------------------------
// LRU

static void dbengine_page_cache_ll_add(struct pgc_linked_list *ll, PGC_PAGE *page) {
    netdata_spinlock_lock(&ll->spinlock);
    DOUBLE_LINKED_LIST_APPEND_UNSAFE(ll->base, page, link.prev, link.next);
    __atomic_add_fetch(&ll->entries, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ll->size, page->assumed_size, __ATOMIC_RELAXED);
    netdata_spinlock_unlock(&ll->spinlock);
}

static void dbengine_page_cache_ll_del(struct pgc_linked_list *ll, PGC_PAGE *page) {
    netdata_spinlock_lock(&ll->spinlock);
    DOUBLE_LINKED_LIST_REMOVE_UNSAFE(ll->base, page, link.prev, link.next);
    __atomic_sub_fetch(&ll->entries, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&ll->size, page->assumed_size, __ATOMIC_RELAXED);
    netdata_spinlock_unlock(&ll->spinlock);
}

static void page_has_been_used(PGC *cache, PGC_PAGE *page) {
    if(is_page_clean(page)) {
        netdata_spinlock_lock(&cache->clean.spinlock);
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(cache->clean.base, page, link.prev, link.next);
        DOUBLE_LINKED_LIST_APPEND_UNSAFE(cache->clean.base, page, link.prev, link.next);
        netdata_spinlock_unlock(&cache->clean.spinlock);
    }
}


// ----------------------------------------------------------------------------
// state transitions

static inline void page_clear_clean(PGC *cache, PGC_PAGE *page) {
    if(!is_page_clean(page)) return;

    // first clear the flag, then remove from the list (required for move_page_last())
    page_flag_clear(page, DBENGINE_PGC_PAGE_CLEAN);
    dbengine_page_cache_ll_del(&cache->clean, page);
}

static void page_clear_dirty(PGC *cache, PGC_PAGE *page) {
    if(!is_page_dirty(page)) return;

    // first clear the flag, then remove from the list (required for move_page_last())
    page_flag_clear(page, DBENGINE_PGC_PAGE_DIRTY);
    dbengine_page_cache_ll_del(&cache->dirty, page);
}

static inline void page_clear_hot(PGC *cache, PGC_PAGE *page) {
    if(!is_page_hot(page)) return;

    // first clear the flag, then remove from the list (required for move_page_last())
    page_flag_clear(page, DBENGINE_PGC_PAGE_HOT);
    dbengine_page_cache_ll_del(&cache->hot, page);
}

static inline void page_set_clean(PGC *cache, PGC_PAGE *page) {
    if(is_page_clean(page)) return;

    page_clear_hot(cache, page);
    page_clear_dirty(cache, page);

    // first add to linked list, the set the flag (required for move_page_last())
    dbengine_page_cache_ll_add(&cache->clean, page);
    page_flag_set(page, DBENGINE_PGC_PAGE_CLEAN);
}

static void page_set_dirty(PGC *cache, PGC_PAGE *page) {
    if(is_page_dirty(page)) return;

    page_clear_hot(cache, page);
    page_clear_clean(cache, page);

    // first add to linked list, the set the flag (required for move_page_last())
    dbengine_page_cache_ll_add(&cache->dirty, page);
    page_flag_set(page, DBENGINE_PGC_PAGE_DIRTY);
}

static inline void page_set_hot(PGC *cache, PGC_PAGE *page) {
    if(is_page_hot(page)) return;

    page_clear_dirty(cache, page);
    page_clear_clean(cache, page);

    // first add to linked list, the set the flag (required for move_page_last())
    dbengine_page_cache_ll_add(&cache->hot, page);
    page_flag_set(page, DBENGINE_PGC_PAGE_HOT);
}


// ----------------------------------------------------------------------------
// Referencing

static inline void PGC_REFERENCED_ITEMS_PLUS1(PGC *cache, PGC_PAGE *page) {
    __atomic_add_fetch(&cache->stats.referenced_entries, 1, __ATOMIC_SEQ_CST);
    __atomic_add_fetch(&cache->stats.referenced_size, page->assumed_size, __ATOMIC_SEQ_CST);
}

static inline void PGC_REFERENCED_ITEMS_MINUS1(PGC *cache, PGC_PAGE *page) {
    __atomic_sub_fetch(&cache->stats.referenced_entries, 1, __ATOMIC_SEQ_CST);
    __atomic_sub_fetch(&cache->stats.referenced_size, page->assumed_size, __ATOMIC_SEQ_CST);
}

static inline bool page_acquire(PGC *cache, PGC_PAGE *page) {
    REFCOUNT expected, desired;

    expected = __atomic_load_n(&page->refcount, __ATOMIC_SEQ_CST);

    do {
        if(expected < 0)
            return false;

        desired = expected + 1;

    } while(!__atomic_compare_exchange_n(&page->refcount, &expected, desired, true, __ATOMIC_SEQ_CST, __ATOMIC_RELAXED));

    if(desired == 1)
        PGC_REFERENCED_ITEMS_PLUS1(cache, page);

    page_has_been_used(cache, page);

    return true;
}

static inline void page_release(PGC *cache, PGC_PAGE *page) {
    REFCOUNT expected, desired;

    expected = __atomic_load_n(&page->refcount, __ATOMIC_SEQ_CST);

    do {
        if(expected <= 0)
            fatal("DBENGINE2: trying to release a page with reference counter %d", expected);

        desired = expected - 1;

    } while(!__atomic_compare_exchange_n(&page->refcount, &expected, desired, true, __ATOMIC_SEQ_CST, __ATOMIC_RELAXED));

    if(desired == 0)
        PGC_REFERENCED_ITEMS_MINUS1(cache, page);
}

static inline bool page_get_for_deletion(PGC *cache __maybe_unused, PGC_PAGE *page) {
    REFCOUNT expected, desired;

    expected = __atomic_load_n(&page->refcount, __ATOMIC_SEQ_CST);

    if(expected != 0)
        return false;

    desired = REFCOUNT_DELETING;

    return __atomic_compare_exchange_n(&page->refcount, &expected, desired, true, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST);
}


// ----------------------------------------------------------------------------
// Indexing

static inline size_t CACHE_CURRENT_CLEAN_SIZE(PGC *cache) {
    return __atomic_load_n(&cache->clean.size, __ATOMIC_SEQ_CST);
}

static void dbengine_page_cache_lru_evictions(PGC *cache) {
    while(CACHE_CURRENT_CLEAN_SIZE(cache) > cache->config.max_size) {
        netdata_spinlock_lock(&cache->clean.spinlock);

        // find one to delete
        PGC_PAGE *page = cache->clean.base;
        while(page && !page_get_for_deletion(cache, page))
            page = page->link.next;

        netdata_spinlock_unlock(&cache->clean.spinlock);

        if(page) {
            // remove it from the linked list
            dbengine_page_cache_ll_del(&cache->clean, page);

            // remove it from the Judy arrays
            dbengine_cache_index_write_lock(cache);

            Pvoid_t *metrics_judy_pptr = JudyLGet(cache->index.sections_judy, page->section, PJE0);
            if(unlikely(!metrics_judy_pptr))
                fatal("DBENGINE CACHE: section '%lu' should exist, but it does not.", page->section);

            Pvoid_t *pages_judy_pptr = JudyLGet(*metrics_judy_pptr, page->metric_id, PJE0);
            if(unlikely(!pages_judy_pptr))
                fatal("DBENGINE CACHE: metric '%lu' in section '%lu' should exist, but it does not.",
                      page->metric_id, page->section);

            Pvoid_t *page_ptr = JudyLGet(*pages_judy_pptr, page->start_time_t, PJE0);
            if(unlikely(!page_ptr))
                fatal("DBENGINE CACHE: page with start time '%lu' of metric '%lu' in section '%lu' should exist, but it does not.",
                      page->start_time_t, page->metric_id, page->section);

            if(unlikely(*page_ptr != page))
                fatal("DBENGINE CACHE: page with start time '%lu' of metric '%lu' in section '%lu' should exist, but the index returned a different address.",
                      page->start_time_t, page->metric_id, page->section);

            if(JudyLDel(pages_judy_pptr, page->start_time_t, PJE0))
                fatal("DBENGINE CACHE: page with start time '%lu' of metric '%lu' in section '%lu' exists, but cannot be deleted.",
                      page->start_time_t, page->metric_id, page->section);

            if(!*pages_judy_pptr && JudyLDel(metrics_judy_pptr, page->metric_id, PJE0))
                fatal("DBENGINE CACHE: metric '%lu' in section '%lu' exists and is empty, but cannot be deleted.",
                      page->metric_id, page->section);

            if(!*metrics_judy_pptr && JudyLDel(&cache->index.sections_judy, page->section, PJE0))
                fatal("DBENGINE CACHE: section '%lu' exists and is empty, but cannot be deleted.", page->section);

            dbengine_cache_index_write_unlock(cache);

            // FIXME - call the callback to free this memory

            // update statistics
            __atomic_sub_fetch(&cache->stats.entries, 1, __ATOMIC_SEQ_CST);
            __atomic_sub_fetch(&cache->stats.size, page->assumed_size, __ATOMIC_SEQ_CST);

            // free our memory
            freez(page);
        }
        else {
            error_limit_static_global_var(erl, 1, 0);
            error_limit(&erl,
                        "DBENGINE CACHE: cache size %zu, exceeds max size %zu, but all the data in it are currently referenced and cannot be evicted",
                        CACHE_CURRENT_CLEAN_SIZE(cache), cache->config.max_size);

            break;
        }
    }
}

static PGC_PAGE *dbengine_page_cache_page_add(PGC *cache, PGC_ENTRY *entry) {
    PGC_PAGE *page;

    do {
        dbengine_cache_index_write_lock(cache);

        Pvoid_t *metric_id_judy_ptr = JudyLIns(&cache->index.sections_judy, entry->section, PJE0);
        Pvoid_t *time_judy_ptr = JudyLIns(metric_id_judy_ptr, entry->metric_id, PJE0);
        Pvoid_t *page_ptr = JudyLIns(time_judy_ptr, entry->start_time_t, PJE0);

        page = *page_ptr;

        if (likely(!page)) {
            page = callocz(1, sizeof(PGC_PAGE));
            page->section = entry->section;
            page->metric_id = entry->metric_id;
            page->start_time_t = entry->start_time_t;
            page->refcount = 1;
            page->data = entry->data;
            page->assumed_size = page_assumed_size(entry->size);

            // put it in the index
            *page_ptr = page;

            dbengine_cache_index_write_unlock(cache);

            if (entry->hot)
                page_set_hot(cache, page);
            else
                page_set_clean(cache, page);

            // update statistics
            __atomic_add_fetch(&cache->stats.entries, 1, __ATOMIC_SEQ_CST);
            __atomic_add_fetch(&cache->stats.size, page->assumed_size, __ATOMIC_SEQ_CST);
        }
        else {
            dbengine_cache_index_write_unlock(cache);

            if (!page_acquire(cache, page))
                page = NULL;
        }

    } while(!page);

    dbengine_page_cache_lru_evictions(cache);

    return page;
}


// ----------------------------------------------------------------------------
// public API

PGC *pgc_create(size_t max_size) {
    PGC *cache = callocz(1, sizeof(PGC));
    cache->config.max_size = max_size;
    netdata_rwlock_init(&cache->index.rwlock);
    netdata_spinlock_init(&cache->hot.spinlock);
    netdata_spinlock_init(&cache->dirty.spinlock);
    netdata_spinlock_init(&cache->clean.spinlock);

    return cache;
}

PGC_PAGE *pgc_page_add_and_acquire(PGC *cache, PGC_ENTRY entry) {
    return dbengine_page_cache_page_add(cache, &entry);
}

void *cache_entry_get_exact(Word_t section, Word_t metric_id, Word_t start_time_t) {

}

PGC_ENTRY cache_entry_search_closest(Word_t section, Word_t metric_id, Word_t start_time_t) {

}

