#include "cache.h"

typedef int32_t REFCOUNT;
#define REFCOUNT_DELETING (-100)

typedef enum __attribute__ ((__packed__)) {
    // mutually exclusive flags
    DBENGINE_PGC_PAGE_CLEAN     = (1 << 0), // none of the following
    DBENGINE_PGC_PAGE_DIRTY     = (1 << 1), // contains unsaved data
    DBENGINE_PGC_PAGE_HOT       = (1 << 2), // currently being collected

    DBENGINE_PGC_PAGE_IS_BEING_CREATED = (1 << 3),
    DBENGINE_PGC_PAGE_IS_BEING_DELETED = (1 << 4),
    DBENGINE_PGC_PAGE_IS_BEING_SAVED   = (1 << 5),
} PGC_PAGE_FLAGS;

#define page_flag_check(page, flag) (__atomic_load_n(&((page)->flags), __ATOMIC_RELAXED) & (flag))
#define page_flag_set(page, flag)   __atomic_or_fetch(&((page)->flags), flag, __ATOMIC_RELAXED)
#define page_flag_clear(page, flag) __atomic_and_fetch(&((page)->flags), ~(flag), __ATOMIC_RELAXED)

#define is_page_hot(page) page_flag_check(page, DBENGINE_PGC_PAGE_HOT)
#define is_page_dirty(page) page_flag_check(page, DBENGINE_PGC_PAGE_DIRTY)
#define is_page_clean(page) page_flag_check(page, DBENGINE_PGC_PAGE_CLEAN)

struct pgc_page {
    Word_t unique_id;

    // indexing data
    Word_t section;
    Word_t metric_id;
    Word_t start_time_t;

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
    PGC_PAGE *base;
    size_t entries;
    size_t size;
};

struct pgc {
    struct {
        size_t max_size;
        size_t max_dirty_pages_to_save_at_once;
        free_clean_page_callback pgc_free_clean_cb;
        save_dirty_page_callback pgc_save_dirty_cb;
    } config;

    struct {
        netdata_rwlock_t rwlock;
        Pvoid_t sections_judy;
    } index;

    struct pgc_linked_list clean;       // LRU is applied here to free memory from the cache
    struct pgc_linked_list dirty;       // in the dirty list, pages are ordered the way they were marked dirty
    struct pgc_linked_list hot;         // in the hot list, pages are order the way they were marked hot

    struct {
        size_t last_unique_id;          // each page gets an unique id, every time it is added to the cache

        size_t added_entries;
        size_t added_size;

        size_t deleted_entries;
        size_t deleted_size;

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

static size_t page_size_from_assumed_size(size_t assumed_size) {
    return assumed_size - sizeof(PGC_PAGE) - sizeof(Word_t) * 3;
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

static void dbengine_page_cache_ll_del(struct pgc_linked_list *ll, PGC_PAGE *page, bool having_lock) {
    if(!having_lock)
        netdata_spinlock_lock(&ll->spinlock);

    DOUBLE_LINKED_LIST_REMOVE_UNSAFE(ll->base, page, link.prev, link.next);
    __atomic_sub_fetch(&ll->entries, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&ll->size, page->assumed_size, __ATOMIC_RELAXED);

    if(!having_lock)
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
    dbengine_page_cache_ll_del(&cache->clean, page, false);
}

static void page_clear_dirty(PGC *cache, PGC_PAGE *page) {
    if(!is_page_dirty(page)) return;

    // first clear the flag, then remove from the list (required for move_page_last())
    page_flag_clear(page, DBENGINE_PGC_PAGE_DIRTY);
    dbengine_page_cache_ll_del(&cache->dirty, page, false);
}

static inline void page_clear_hot(PGC *cache, PGC_PAGE *page, bool having_hot_lock) {
    if(!is_page_hot(page)) return;

    // first clear the flag, then remove from the list (required for move_page_last())
    page_flag_clear(page, DBENGINE_PGC_PAGE_HOT);
    dbengine_page_cache_ll_del(&cache->hot, page, having_hot_lock);
}

static inline void page_set_clean(PGC *cache, PGC_PAGE *page) {
    netdata_spinlock_lock(&page->transition_spinlock);

    if(is_page_clean(page)) {
        netdata_spinlock_unlock(&page->transition_spinlock);
        return;
    }

    page_clear_hot(cache, page, false);
    page_clear_dirty(cache, page);

    // first add to linked list, the set the flag (required for move_page_last())
    dbengine_page_cache_ll_add(&cache->clean, page);
    page_flag_set(page, DBENGINE_PGC_PAGE_CLEAN);

    netdata_spinlock_unlock(&page->transition_spinlock);
}

static void page_set_dirty(PGC *cache, PGC_PAGE *page, bool having_hot_lock) {
    netdata_spinlock_lock(&page->transition_spinlock);

    if(is_page_dirty(page)) {
        netdata_spinlock_unlock(&page->transition_spinlock);
        return;
    }

    page_clear_hot(cache, page, having_hot_lock);
    page_clear_clean(cache, page);

    // first add to linked list, the set the flag (required for move_page_last())
    dbengine_page_cache_ll_add(&cache->dirty, page);
    page_flag_set(page, DBENGINE_PGC_PAGE_DIRTY);

    netdata_spinlock_unlock(&page->transition_spinlock);
}

static inline void page_set_hot(PGC *cache, PGC_PAGE *page) {
    netdata_spinlock_lock(&page->transition_spinlock);

    if(is_page_hot(page)) {
        netdata_spinlock_unlock(&page->transition_spinlock);
        return;
    }

    page_clear_dirty(cache, page);
    page_clear_clean(cache, page);

    // first add to linked list, the set the flag (required for move_page_last())
    dbengine_page_cache_ll_add(&cache->hot, page);
    page_flag_set(page, DBENGINE_PGC_PAGE_HOT);

    netdata_spinlock_unlock(&page->transition_spinlock);
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

    expected = __atomic_load_n(&page->refcount, __ATOMIC_SEQ_CST);

    do {
        if(expected < 0)
            return false;

        desired = expected + 1;

    } while(!__atomic_compare_exchange_n(&page->refcount, &expected, desired, true, __ATOMIC_SEQ_CST, __ATOMIC_RELAXED));

    if(desired == 1)
        PGC_REFERENCED_PAGES_PLUS1(cache, page);

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
        PGC_REFERENCED_PAGES_MINUS1(cache, page);
}

static inline bool page_get_for_deletion(PGC *cache __maybe_unused, PGC_PAGE *page) {
    REFCOUNT expected, desired;

    expected = __atomic_load_n(&page->refcount, __ATOMIC_SEQ_CST);

    if(expected != 0)
        return false;

    desired = REFCOUNT_DELETING;

    bool ret = __atomic_compare_exchange_n(&page->refcount, &expected, desired, true, __ATOMIC_SEQ_CST, __ATOMIC_SEQ_CST);

    return ret;
}


// ----------------------------------------------------------------------------
// Indexing

static inline size_t CACHE_CURRENT_CLEAN_SIZE(PGC *cache) {
    return __atomic_load_n(&cache->clean.size, __ATOMIC_SEQ_CST);
}

static void dbengine_page_cache_lru_evictions(PGC *cache, bool all) {
    while(all || CACHE_CURRENT_CLEAN_SIZE(cache) > cache->config.max_size) {
        netdata_spinlock_lock(&cache->clean.spinlock);

        // find one to delete
        PGC_PAGE *page = cache->clean.base;
        while(page && !page_get_for_deletion(cache, page))
            page = page->link.next;

        if(page) {
            page_flag_set(page, DBENGINE_PGC_PAGE_IS_BEING_DELETED);

            // remove it from the linked list
            dbengine_page_cache_ll_del(&cache->clean, page, true);

            netdata_spinlock_unlock(&cache->clean.spinlock);

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

            if(unlikely(JudyLDel(pages_judy_pptr, page->start_time_t, PJE0)))
                fatal("DBENGINE CACHE: page with start time '%lu' of metric '%lu' in section '%lu' exists, but cannot be deleted.",
                      page->start_time_t, page->metric_id, page->section);

            if(!*pages_judy_pptr && JudyLDel(metrics_judy_pptr, page->metric_id, PJE0))
                fatal("DBENGINE CACHE: metric '%lu' in section '%lu' exists and is empty, but cannot be deleted.",
                      page->metric_id, page->section);

            if(!*metrics_judy_pptr && JudyLDel(&cache->index.sections_judy, page->section, PJE0))
                fatal("DBENGINE CACHE: section '%lu' exists and is empty, but cannot be deleted.", page->section);

            dbengine_cache_index_write_unlock(cache);

            // call the callback to free the user supplied memory
            cache->config.pgc_free_clean_cb(cache, (PGC_ENTRY){
                .section = page->section,
                .metric_id = page->metric_id,
                .start_time_t = page->start_time_t,
                .size = page_size_from_assumed_size(page->assumed_size),
                .hot = (is_page_hot(page)) ? true : false,

            });

            page_flag_clear(page, DBENGINE_PGC_PAGE_IS_BEING_DELETED);

            // update statistics
            __atomic_add_fetch(&cache->stats.deleted_entries, 1, __ATOMIC_RELAXED);
            __atomic_add_fetch(&cache->stats.deleted_size, page->assumed_size, __ATOMIC_RELAXED);

            __atomic_sub_fetch(&cache->stats.entries, 1, __ATOMIC_RELAXED);
            __atomic_sub_fetch(&cache->stats.size, page->assumed_size, __ATOMIC_RELAXED);

            // free our memory
            freez(page);
        }
        else {
            netdata_spinlock_unlock(&cache->clean.spinlock);

            if(all) {
                error_limit_static_global_var(erl, 1, 0);
                error_limit(&erl, "DBENGINE CACHE: cannot free all clean pages, some are still referenced");
            }
            else if(cache->clean.base) {
                error_limit_static_global_var(erl, 1, 0);
                error_limit(&erl,
                            "DBENGINE CACHE: cache size %zu, exceeds max size %zu, but all the data in it are currently referenced and cannot be evicted",
                            CACHE_CURRENT_CLEAN_SIZE(cache), cache->config.max_size);
            }

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
            page->unique_id = __atomic_add_fetch(&cache->stats.last_unique_id, 1, __ATOMIC_RELAXED);
            page->refcount = 1;
            page->flags = DBENGINE_PGC_PAGE_IS_BEING_CREATED;
            page->section = entry->section;
            page->metric_id = entry->metric_id;
            page->start_time_t = entry->start_time_t;
            page->data = entry->data;
            page->assumed_size = page_assumed_size(entry->size);

            // put it in the index
            *page_ptr = page;

            dbengine_cache_index_write_unlock(cache);

            if (entry->hot)
                page_set_hot(cache, page);
            else
                page_set_clean(cache, page);

            page_flag_clear(page, DBENGINE_PGC_PAGE_IS_BEING_CREATED);

            // update statistics
            __atomic_add_fetch(&cache->stats.added_entries, 1, __ATOMIC_RELAXED);
            __atomic_add_fetch(&cache->stats.added_size, page->assumed_size, __ATOMIC_RELAXED);

            __atomic_add_fetch(&cache->stats.entries, 1, __ATOMIC_RELAXED);
            __atomic_add_fetch(&cache->stats.size, page->assumed_size, __ATOMIC_RELAXED);
        }
        else {
            dbengine_cache_index_write_unlock(cache);

            if (!page_acquire(cache, page))
                page = NULL;
        }

    } while(!page);

    dbengine_page_cache_lru_evictions(cache, false);

    return page;
}

void all_hot_pages_to_dirty(PGC *cache) {
    netdata_spinlock_lock(&cache->hot.spinlock);
    PGC_PAGE *page = cache->hot.base;
    while(page) {
        PGC_PAGE *next = page->link.next;

        if(page_acquire(cache, page)) {
            page_set_dirty(cache, page, true);
            page_release(cache, page);
        }

        page = next;
    }
    netdata_spinlock_unlock(&cache->hot.spinlock);
}

void flush_dirty_pages(PGC *cache, bool all_of_them) {

}

void free_all_unreferenced_clean_pages(PGC *cache) {
    dbengine_page_cache_lru_evictions(cache, true);
}

// ----------------------------------------------------------------------------
// public API

PGC *pgc_create(size_t max_size, free_clean_page_callback pgc_free_cb,
                size_t max_dirty_pages_to_save_at_once, save_dirty_page_callback pgc_save_dirty_cb) {
    PGC *cache = callocz(1, sizeof(PGC));
    cache->config.max_size = max_size;
    cache->config.pgc_free_clean_cb = pgc_free_cb;
    cache->config.max_dirty_pages_to_save_at_once = max_dirty_pages_to_save_at_once,
    cache->config.pgc_save_dirty_cb = pgc_save_dirty_cb;
    netdata_rwlock_init(&cache->index.rwlock);
    netdata_spinlock_init(&cache->hot.spinlock);
    netdata_spinlock_init(&cache->dirty.spinlock);
    netdata_spinlock_init(&cache->clean.spinlock);

    return cache;
}

void pgc_destroy(PGC *cache) {
    // convert all hot pages to dirty
    all_hot_pages_to_dirty(cache);

    // save all dirty pages to make them clean
    flush_dirty_pages(cache, true);

    // free all unreferenced clean pages
    free_all_unreferenced_clean_pages(cache);

    if(PGC_REFERENCED_PAGES(cache))
        error("DBENGINE CACHE: there are %zu referenced cache pages - leaving the cache allocated", PGC_REFERENCED_PAGES(cache));
    else
        freez(cache);
}

PGC_PAGE *pgc_page_add_and_acquire(PGC *cache, PGC_ENTRY entry) {
    return dbengine_page_cache_page_add(cache, &entry);
}

void pgc_page_release(PGC *cache, PGC_PAGE *page) {
    page_release(cache, page);
}

void pgc_page_hot_to_dirty_and_release(PGC *cache, PGC_PAGE *page) {
    if(is_page_hot(page)) {
        page_set_dirty(cache, page, false);
        flush_dirty_pages(cache, false);
    }

    page_release(cache, page);
}

void *cache_entry_get_exact(Word_t section, Word_t metric_id, Word_t start_time_t) {

}

PGC_ENTRY cache_entry_search_closest(Word_t section, Word_t metric_id, Word_t start_time_t) {

}

