#include "cache.h"

typedef int32_t REFCOUNT;
#define REFCOUNT_DELETING (-100)

typedef enum __attribute__ ((__packed__)) {
    // mutually exclusive flags
    DBENGINE_PGC_PAGE_CLEAN     = (1 << 0), // none of the following
    DBENGINE_PGC_PAGE_DIRTY     = (1 << 1), // contains unsaved data
    DBENGINE_PGC_PAGE_HOT       = (1 << 2), // currently being collected

    // flags related to various actions on each page
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
    //Word_t unique_id;

    // indexing data
    Word_t section;
    Word_t metric_id;
    Word_t start_time_t;
    Word_t end_time_t;

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
    union {
        PGC_PAGE *base;
        Pvoid_t sections_judy;
    };
    uint32_t version;
    bool linked_list_in_sections_judy; // when true, we use 'sections_judy', otherwise we use 'base'
};

struct pgc {
    struct {
        size_t max_clean_size;
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
        // size_t last_unique_id;          // each page gets a unique id, every time it is added to the cache

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
// Linked list management

struct section_dirty_pages {
    size_t entries;
    size_t size;
    PGC_PAGE *base;
};

static void dbengine_page_cache_ll_add(PGC *cache __maybe_unused, struct pgc_linked_list *ll, PGC_PAGE *page) {
    netdata_spinlock_lock(&ll->spinlock);
    __atomic_add_fetch(&ll->entries, 1, __ATOMIC_RELAXED);
    __atomic_add_fetch(&ll->size, page->assumed_size, __ATOMIC_RELAXED);

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

    netdata_spinlock_unlock(&ll->spinlock);
}

static void dbengine_page_cache_ll_del(PGC *cache __maybe_unused, struct pgc_linked_list *ll, PGC_PAGE *page, bool having_lock) {
    if(!having_lock)
        netdata_spinlock_lock(&ll->spinlock);

    __atomic_sub_fetch(&ll->entries, 1, __ATOMIC_RELAXED);
    __atomic_sub_fetch(&ll->size, page->assumed_size, __ATOMIC_RELAXED);

    if(ll->linked_list_in_sections_judy) {
        Pvoid_t *dirty_pages_pptr = JudyLGet(ll->sections_judy, page->section, PJE0);
        struct section_dirty_pages *sdp = *dirty_pages_pptr;
        sdp->entries--;
        sdp->size -= page->assumed_size;
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(sdp->base, page, link.prev, link.next);

        if(!sdp->base) {
            JudyLDel(&ll->sections_judy, page->section, PJE0);
            freez(sdp);
        }
    }
    else {
        DOUBLE_LINKED_LIST_REMOVE_UNSAFE(ll->base, page, link.prev, link.next);
    }

    ll->version++;

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
    dbengine_page_cache_ll_del(cache, &cache->clean, page, false);
}

static void page_clear_dirty(PGC *cache, PGC_PAGE *page, bool having_dirty_lock) {
    if(!is_page_dirty(page)) return;

    // first clear the flag, then remove from the list (required for move_page_last())
    page_flag_clear(page, DBENGINE_PGC_PAGE_DIRTY);
    dbengine_page_cache_ll_del(cache, &cache->dirty, page, having_dirty_lock);
}

static inline void page_clear_hot(PGC *cache, PGC_PAGE *page, bool having_hot_lock) {
    if(!is_page_hot(page)) return;

    // first clear the flag, then remove from the list (required for move_page_last())
    page_flag_clear(page, DBENGINE_PGC_PAGE_HOT);
    dbengine_page_cache_ll_del(cache, &cache->hot, page, having_hot_lock);
}

static inline void page_set_clean(PGC *cache, PGC_PAGE *page, bool having_dirty_lock) {
    netdata_spinlock_lock(&page->transition_spinlock);

    if(is_page_clean(page)) {
        netdata_spinlock_unlock(&page->transition_spinlock);
        return;
    }

    page_clear_hot(cache, page, false);
    page_clear_dirty(cache, page, having_dirty_lock);

    // first add to linked list, the set the flag (required for move_page_last())
    dbengine_page_cache_ll_add(cache, &cache->clean, page);
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
    dbengine_page_cache_ll_add(cache, &cache->dirty, page);
    page_flag_set(page, DBENGINE_PGC_PAGE_DIRTY);

    netdata_spinlock_unlock(&page->transition_spinlock);
}

static inline void page_set_hot(PGC *cache, PGC_PAGE *page) {
    netdata_spinlock_lock(&page->transition_spinlock);

    if(is_page_hot(page)) {
        netdata_spinlock_unlock(&page->transition_spinlock);
        return;
    }

    page_clear_dirty(cache, page, false);
    page_clear_clean(cache, page);

    // first add to linked list, the set the flag (required for move_page_last())
    dbengine_page_cache_ll_add(cache, &cache->hot, page);
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

static void evict_pages(PGC *cache, bool all) {
#ifdef NETDATA_INTERNAL_CHECKS
    internal_fatal(cache->clean.linked_list_in_sections_judy,
                   "wrong clean pages configuration - clean pages need to have a linked list, not a judy array");
#endif

    while(all || CACHE_CURRENT_CLEAN_SIZE(cache) > cache->config.max_clean_size) {
        netdata_spinlock_lock(&cache->clean.spinlock);

        // find one to delete
        PGC_PAGE *page = cache->clean.base;
        if(!page) break;

        while(page && !page_get_for_deletion(cache, page))
            page = page->link.next;

        if(page) {
            page_flag_set(page, DBENGINE_PGC_PAGE_IS_BEING_DELETED);

            // remove it from the linked list
            dbengine_page_cache_ll_del(cache, &cache->clean, page, true);

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

            if(unlikely(!JudyLDel(pages_judy_pptr, page->start_time_t, PJE0)))
                fatal("DBENGINE CACHE: page with start time '%lu' of metric '%lu' in section '%lu' exists, but cannot be deleted.",
                      page->start_time_t, page->metric_id, page->section);

            if(!*pages_judy_pptr && !JudyLDel(metrics_judy_pptr, page->metric_id, PJE0))
                fatal("DBENGINE CACHE: metric '%lu' in section '%lu' exists and is empty, but cannot be deleted.",
                      page->metric_id, page->section);

            if(!*metrics_judy_pptr && !JudyLDel(&cache->index.sections_judy, page->section, PJE0))
                fatal("DBENGINE CACHE: section '%lu' exists and is empty, but cannot be deleted.", page->section);

            dbengine_cache_index_write_unlock(cache);

            // call the callback to free the user supplied memory
            cache->config.pgc_free_clean_cb(cache, (PGC_ENTRY){
                .section = page->section,
                .metric_id = page->metric_id,
                .start_time_t = page->start_time_t,
                .end_time_t = __atomic_load_n(&page->end_time_t, __ATOMIC_SEQ_CST),
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
                            CACHE_CURRENT_CLEAN_SIZE(cache), cache->config.max_clean_size);
            }

            break;
        }
    }
}

static PGC_PAGE *page_add(PGC *cache, PGC_ENTRY *entry) {
    PGC_PAGE *page;

    do {
        dbengine_cache_index_write_lock(cache);

        Pvoid_t *metrics_judy_pptr = JudyLIns(&cache->index.sections_judy, entry->section, PJE0);
        Pvoid_t *pages_judy_pptr = JudyLIns(metrics_judy_pptr, entry->metric_id, PJE0);
        Pvoid_t *page_ptr = JudyLIns(pages_judy_pptr, entry->start_time_t, PJE0);

        page = *page_ptr;

        if (likely(!page)) {
            page = callocz(1, sizeof(PGC_PAGE));
            // page->unique_id = __atomic_add_fetch(&cache->stats.last_unique_id, 1, __ATOMIC_RELAXED);
            page->refcount = 1;
            page->flags = DBENGINE_PGC_PAGE_IS_BEING_CREATED;
            page->section = entry->section;
            page->metric_id = entry->metric_id;
            page->start_time_t = entry->start_time_t;
            page->end_time_t = entry->end_time_t,
            page->data = entry->data;
            page->assumed_size = page_assumed_size(entry->size);
            netdata_spinlock_init(&page->transition_spinlock);

            // put it in the index
            *page_ptr = page;

            dbengine_cache_index_write_unlock(cache);

            if (entry->hot)
                page_set_hot(cache, page);
            else
                page_set_clean(cache, page, false);

            page_flag_clear(page, DBENGINE_PGC_PAGE_IS_BEING_CREATED);

            PGC_REFERENCED_PAGES_PLUS1(cache, page);

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

    evict_pages(cache, false);

    return page;
}

PGC_PAGE *page_find_and_acquire(PGC *cache, Word_t section, Word_t metric_id, Word_t start_time_t) {
    PGC_PAGE *page = NULL;

    do {
        dbengine_cache_index_read_lock(cache);

        Pvoid_t *metrics_judy_pptr = JudyLGet(cache->index.sections_judy, section, PJE0);
        if(unlikely(!metrics_judy_pptr)) {
            // section does not exist
            dbengine_cache_index_read_unlock(cache);
            break;
        }

        Pvoid_t *pages_judy_pptr = JudyLGet(*metrics_judy_pptr, metric_id, PJE0);
        if(unlikely(!pages_judy_pptr)) {
            // metric does not exist
            dbengine_cache_index_read_unlock(cache);
            break;
        }

        Pvoid_t *page_ptr;

        page_ptr = JudyLGet(*pages_judy_pptr, start_time_t, PJE0);
        if(page_ptr) {
            // exact match on the timestamp
            page = *page_ptr;
        }
        else {
            Word_t time = start_time_t;

            // find the previous page
            page_ptr = JudyLLast(*pages_judy_pptr, &time, PJE0);
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

        dbengine_cache_index_read_unlock(cache);

        if(!page)
            // no page found for this query
            break;

        if(!page_acquire(cache, page))
            // retry, this page is not good for us...
            page = NULL;

    } while(!page);

    return page;
}

static void all_hot_pages_to_dirty(PGC *cache) {
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

static inline PPvoid_t JudyLFirstThenNext(Pcvoid_t PArray, Word_t * PIndex, bool *first) {
    if(unlikely(*first)) {
        *first = false;
        return JudyLFirst(PArray, PIndex, PJE0);
    }

    return JudyLNext(PArray, PIndex, PJE0);
}

static void flush_dirty_pages(PGC *cache, bool all_of_them) {
    netdata_spinlock_lock(&cache->dirty.spinlock);

#ifdef NETDATA_INTERNAL_CHECKS
    internal_fatal(!cache->dirty.linked_list_in_sections_judy,
                   "wrong dirty pages configuration - dirty pages need to have a judy array, not a linked list");
#endif

    Word_t last_section = 0;
    Pvoid_t *dirty_pages_pptr;
    bool first = true;
    while ((dirty_pages_pptr = JudyLFirstThenNext(cache->dirty.sections_judy, &last_section, &first))) {

        struct section_dirty_pages *sdp = *dirty_pages_pptr;
        PGC_PAGE *page = sdp->base;
        while (page && (all_of_them || sdp->entries >= cache->config.max_dirty_pages_to_save_at_once)) {
            PGC_ENTRY array[cache->config.max_dirty_pages_to_save_at_once];
            PGC_PAGE *pages[cache->config.max_dirty_pages_to_save_at_once];
            size_t used = 0;

            while (page && used < cache->config.max_dirty_pages_to_save_at_once) {
                PGC_PAGE *next = page->link.next;

                if (page_acquire(cache, page)) {
                    netdata_spinlock_lock(&page->transition_spinlock);
                    page_flag_set(page, DBENGINE_PGC_PAGE_IS_BEING_SAVED);

                    pages[used] = page;
                    array[used] = (PGC_ENTRY) {
                            .section = page->section,
                            .metric_id = page->metric_id,
                            .start_time_t = page->start_time_t,
                            .end_time_t = __atomic_load_n(&page->end_time_t, __ATOMIC_SEQ_CST),
                            .size = page_size_from_assumed_size(page->assumed_size),
                            .data = page->data,
                            .hot = (is_page_hot(page)) ? true : false,
                    };
                    used++;
                }

                page = next;
            }

            if (used) {
                bool clean_them = false;

                if (all_of_them || used == cache->config.max_dirty_pages_to_save_at_once) {
                    // call the callback to save them
                    // it may take some time, so let's release the lock
                    netdata_spinlock_unlock(&cache->dirty.spinlock);
                    cache->config.pgc_save_dirty_cb(cache, array, used);
                    netdata_spinlock_lock(&cache->dirty.spinlock);

                    clean_them = true;
                }

                for (size_t i = 0; i < used; i++) {
                    PGC_PAGE *tpg = pages[i];
                    page_flag_clear(tpg, DBENGINE_PGC_PAGE_IS_BEING_SAVED);
                    netdata_spinlock_unlock(&tpg->transition_spinlock);

                    if (clean_them)
                        page_set_clean(cache, tpg, true);

                    page_release(cache, tpg);
                }
            }

        }
    }

    netdata_spinlock_unlock(&cache->dirty.spinlock);
}

void free_all_unreferenced_clean_pages(PGC *cache) {
    evict_pages(cache, true);
}

// ----------------------------------------------------------------------------
// public API

PGC *pgc_create(size_t max_clean_size, free_clean_page_callback pgc_free_cb,
                size_t max_dirty_pages_to_save_at_once, save_dirty_page_callback pgc_save_dirty_cb) {
    PGC *cache = callocz(1, sizeof(PGC));
    cache->config.max_clean_size = max_clean_size;
    cache->config.pgc_free_clean_cb = pgc_free_cb;
    cache->config.max_dirty_pages_to_save_at_once = max_dirty_pages_to_save_at_once,
    cache->config.pgc_save_dirty_cb = pgc_save_dirty_cb;
    netdata_rwlock_init(&cache->index.rwlock);
    netdata_spinlock_init(&cache->hot.spinlock);
    netdata_spinlock_init(&cache->dirty.spinlock);
    netdata_spinlock_init(&cache->clean.spinlock);

    cache->dirty.linked_list_in_sections_judy = true;

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
    return page_add(cache, &entry);
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

Word_t pgc_page_section(PGC_PAGE *page) {
    return page->section;
}

Word_t pgc_page_metric(PGC_PAGE *page) {
    return page->metric_id;
}

Word_t pgc_page_start_time_t(PGC_PAGE *page) {
    return page->start_time_t;
}

void *pgc_page_data(PGC_PAGE *page) {
    return page->data;
}

void pgc_page_hot_set_end_time_t(PGC_PAGE *page, Word_t end_time_t) {
    if(is_page_hot(page))
        __atomic_store_n(&page->end_time_t, end_time_t, __ATOMIC_SEQ_CST);
}

PGC_PAGE *pgc_page_get_and_acquire(PGC *cache, Word_t section, Word_t metric_id, Word_t start_time_t) {
    return page_find_and_acquire(cache, section, metric_id, start_time_t);
}

// ----------------------------------------------------------------------------
// unittest

struct {
    PGC_PAGE **metrics;
    Word_t clean_metrics;
    Word_t hot_metrics;
    time_t first_time_t;
    time_t last_time_t;
} pgc_uts = {
        .metrics        = NULL,
        .clean_metrics  =    9000000,
        .hot_metrics    =    1000000,
        .first_time_t   =  100000000,
        .last_time_t    = 1000000000,
};

void unittest_stress_test(void) {
    pgc_uts.metrics = callocz(pgc_uts.clean_metrics + pgc_uts.hot_metrics, sizeof(PGC_PAGE *));

}

static void unittest_free_clean_page_callback(PGC *cache __maybe_unused, PGC_ENTRY entry __maybe_unused) {
    // info("FREE clean page section %lu, metric %lu, start_time %lu, end_time %lu", entry.section, entry.metric_id, entry.start_time_t, entry.end_time_t);
    ;
}

static void unittest_save_dirty_page_callback(PGC *cache __maybe_unused, PGC_ENTRY *array __maybe_unused, size_t entries __maybe_unused) {
    info("SAVE %zu pages", entries);
    ;
}

int pgc_unittest(void) {
    PGC *cache = pgc_create(32 * 1024 * 1024, unittest_free_clean_page_callback, 64, unittest_save_dirty_page_callback);

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

    pgc_page_hot_set_end_time_t(page2, 2001);
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

    pgc_page_hot_set_end_time_t(page3, 2001);
    pgc_page_hot_to_dirty_and_release(cache, page3);

    usec_t start_ut = now_monotonic_usec();
    for(int i = 0; i < 1000000; i++) {
        PGC_PAGE *p = pgc_page_add_and_acquire(cache, (PGC_ENTRY){
                .section = 1,
                .metric_id = 10,
                .start_time_t = 100 * (i + 1),
                .end_time_t = 1000 + 100 * (i + 1),
                .size = 4096,
                .data = NULL,
                .hot = false,
        });

        pgc_page_release(cache, p);
    }
    usec_t end_ut = now_monotonic_usec();

    info("Created 1M pages in %llu", end_ut - start_ut);

    pgc_destroy(cache);
    return 0;
}
