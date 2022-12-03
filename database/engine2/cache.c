#include "cache.h"

typedef uint16_t REFCOUNT16;

typedef enum __attribute__ ((__packed__)) {
    DBENGINE_PGC_PAGE_CLEAN     = (1 << 0), // none of the following
    DBENGINE_PGC_PAGE_DIRTY     = (1 << 1), // contains unsaved data
    DBENGINE_PGC_PAGE_HOT       = (1 << 2), // currently being collected
} DBENGINE_PGC_PAGE_FLAGS;

typedef struct dbengine_page_cache_page {
    void *data;
    size_t assumed_size;
    REFCOUNT16 refcount;
    DBENGINE_PGC_PAGE_FLAGS flags;

    struct {
        struct dbengine_page_cache_page *next;
        struct dbengine_page_cache_page *prev;
    } link;
} DBENGINE_PGC_PAGE;

struct dbengine_page_linked_list {
    SPINLOCK spinlock;
    DBENGINE_PGC_PAGE *base;
    size_t entries;
    size_t size;
};

typedef struct dbengine_page_cache {
    struct {
        netdata_rwlock_t rwlock;
        Pvoid_t JudyL_sections_index;
    } index;

    struct dbengine_page_linked_list clean;
    struct dbengine_page_linked_list dirty;
    struct dbengine_page_linked_list hot;

    struct {
        size_t entries;                 // all the entries (includes clean, dirty, host)
        size_t size;                    // all the entries (includes clean, dirty, host)

        size_t referenced_entries;      // all the entries currently referenced
        size_t referenced_size;         // all the entries currently referenced
    } stats;
} DBENGINE_PAGE_CACHE;

typedef struct dbengine_page_cache_entry {
    bool hot;
    Word_t section;
    Word_t metric_id;
    Word_t start_time_t;
    size_t size;
    void *data;
} DBENGINE_PAGE_CACHE_ENTRY;

// ----------------------------------------------------------------------------
// locking

static void dbengine_cache_index_read_lock(DBENGINE_PAGE_CACHE *cache) {
    netdata_rwlock_rdlock(&cache->index.rwlock);
}
static void dbengine_cache_index_read_unlock(DBENGINE_PAGE_CACHE *cache) {
    netdata_rwlock_unlock(&cache->index.rwlock);
}
static void dbengine_cache_index_write_lock(DBENGINE_PAGE_CACHE *cache) {
    netdata_rwlock_wrlock(&cache->index.rwlock);
}
static void dbengine_cache_index_write_unlock(DBENGINE_PAGE_CACHE *cache) {
    netdata_rwlock_unlock(&cache->index.rwlock);
}


// ----------------------------------------------------------------------------
// helpers

static size_t page_assumed_size(size_t size) {
    return size + sizeof(DBENGINE_PGC_PAGE) + sizeof(Word_t) * 3;
}


// ----------------------------------------------------------------------------
// LRU

static void dbengine_page_cache_ll_add(struct dbengine_page_linked_list *ll, DBENGINE_PGC_PAGE *page) {
    netdata_spinlock_lock(&ll->spinlock);
    DOUBLE_LINKED_LIST_APPEND_UNSAFE(ll->base, page, link.prev, link.next);
    ll->entries++;
    ll->size += page->assumed_size;
    netdata_spinlock_unlock(&ll->spinlock);
}

static void dbengine_page_cache_ll_del(struct dbengine_page_linked_list *ll, DBENGINE_PGC_PAGE *page) {
    netdata_spinlock_lock(&ll->spinlock);
    DOUBLE_LINKED_LIST_REMOVE_UNSAFE(ll->base, page, link.prev, link.next);
    ll->entries--;
    ll->size -= page->assumed_size;
    netdata_spinlock_unlock(&ll->spinlock);
}

static void dbengine_page_cache_ll_move_last(struct dbengine_page_linked_list *ll, DBENGINE_PGC_PAGE *page) {
    netdata_spinlock_lock(&ll->spinlock);
    DOUBLE_LINKED_LIST_REMOVE_UNSAFE(ll->base, page, link.prev, link.next);
    DOUBLE_LINKED_LIST_APPEND_UNSAFE(ll->base, page, link.prev, link.next);
    netdata_spinlock_unlock(&ll->spinlock);
}

static void dbengine_page_cache_lru_evictions(DBENGINE_PAGE_CACHE *cache) {

}

// ----------------------------------------------------------------------------
// state transitions

#define page_flag_check(page, flag) (__atomic_load_n(&((page)->flags), __ATOMIC_SEQ_CST) & (flag))
#define page_flag_set(page, flag)   __atomic_or_fetch(&((page)->flags), flag, __ATOMIC_SEQ_CST)
#define page_flag_clear(page, flag) __atomic_and_fetch(&((page)->flags), ~(flag), __ATOMIC_SEQ_CST)

#define is_page_hot(page) page_flag_check(page, DBENGINE_PGC_PAGE_HOT)
#define is_page_dirty(page) page_flag_check(page, DBENGINE_PGC_PAGE_DIRTY)
#define is_page_clean(page) page_flag_check(page, DBENGINE_PGC_PAGE_CLEAN)

static void page_clear_dirty(DBENGINE_PAGE_CACHE *cache, DBENGINE_PGC_PAGE *page);
static inline void page_clear_clean(DBENGINE_PAGE_CACHE *cache, DBENGINE_PGC_PAGE *page);

static inline void page_set_hot(DBENGINE_PAGE_CACHE *cache, DBENGINE_PGC_PAGE *page) {
    if(is_page_hot(page)) return;

    page_clear_dirty(cache, page);
    page_clear_clean(cache, page);

    dbengine_page_cache_ll_add(&cache->hot, page);
    page_flag_set(page, DBENGINE_PGC_PAGE_HOT);
}

static inline void page_clear_hot(DBENGINE_PAGE_CACHE *cache, DBENGINE_PGC_PAGE *page) {
    if(!is_page_hot(page)) return;
    dbengine_page_cache_ll_del(&cache->hot, page);
    page_flag_clear(page, DBENGINE_PGC_PAGE_HOT);
}

static void page_set_dirty(DBENGINE_PAGE_CACHE *cache, DBENGINE_PGC_PAGE *page) {
    if(is_page_dirty(page)) return;

    page_clear_hot(cache, page);
    page_clear_clean(cache, page);

    dbengine_page_cache_ll_add(&cache->dirty, page);
    page_flag_set(page, DBENGINE_PGC_PAGE_DIRTY);
}

static void page_clear_dirty(DBENGINE_PAGE_CACHE *cache, DBENGINE_PGC_PAGE *page) {
    if(!is_page_dirty(page)) return;
    dbengine_page_cache_ll_del(&cache->dirty, page);
    page_flag_clear(page, DBENGINE_PGC_PAGE_DIRTY);
}

static inline void page_set_clean(DBENGINE_PAGE_CACHE *cache, DBENGINE_PGC_PAGE *page) {
    if(is_page_clean(page)) return;

    page_clear_hot(cache, page);
    page_clear_dirty(cache, page);

    dbengine_page_cache_ll_add(&cache->clean, page);
    page_flag_set(page, DBENGINE_PGC_PAGE_CLEAN);
}

static inline void page_clear_clean(DBENGINE_PAGE_CACHE *cache, DBENGINE_PGC_PAGE *page) {
    if(!is_page_clean(page)) return;
    dbengine_page_cache_ll_del(&cache->clean, page);
    page_flag_clear(page, DBENGINE_PGC_PAGE_CLEAN);
}

// ----------------------------------------------------------------------------
// Indexing

static bool dbengine_page_cache_page_add(DBENGINE_PAGE_CACHE *cache, DBENGINE_PAGE_CACHE_ENTRY *entry) {
    dbengine_cache_index_write_lock(cache);

    Pvoid_t *metric_id_judy_ptr = JudyLIns(&cache->index.JudyL_sections_index, entry->section, PJE0);
    Pvoid_t *time_judy_ptr = JudyLIns(metric_id_judy_ptr, entry->metric_id, PJE0);
    Pvoid_t *page_ptr = JudyLIns(time_judy_ptr, entry->start_time_t, PJE0);

    bool added;
    if(likely(!*page_ptr)) {
        DBENGINE_PGC_PAGE *page = callocz(1, sizeof(DBENGINE_PGC_PAGE));
        page->data = entry->data;
        page->assumed_size = page_assumed_size(entry->size);

        *page_ptr = page;

        // update statistics
        __atomic_add_fetch(&cache->stats.entries, 1, __ATOMIC_SEQ_CST);
        __atomic_add_fetch(&cache->stats.size, page->assumed_size, __ATOMIC_SEQ_CST);

        if(entry->hot)
            page_set_hot(cache, page);
        else
            page_set_clean(cache, page);

        added = true;
    }
    else
        added = false;

    dbengine_cache_index_write_unlock(cache);

    dbengine_page_cache_lru_evictions(cache);

    return added;
}

// ----------------------------------------------------------------------------
// public API

void cache_entry_add(DBENGINE_PAGE_CACHE_ENTRY ce) {

}

void *cache_entry_get_exact(Word_t section, Word_t metric_id, Word_t start_time_t) {

}

DBENGINE_PAGE_CACHE_ENTRY cache_entry_search_closest(Word_t section, Word_t metric_id, Word_t start_time_t) {

}

