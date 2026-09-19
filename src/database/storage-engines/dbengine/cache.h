// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef DBENGINE_CACHE_H
#define DBENGINE_CACHE_H

#include "datafile.h"
#include "database/storage-engines/dbengine/include/dbengine/dbengine-stats.h"

typedef struct pgc PGC;
typedef struct pgc_page PGC_PAGE;
struct dbengine_engine;
#define PGC_NAME_MAX 23

typedef enum __attribute__ ((__packed__)) {
    PGC_OPTIONS_NONE       = 0,
    PGC_OPTIONS_EVICT_PAGES_NO_INLINE   = (1 << 0),
    PGC_OPTIONS_FLUSH_PAGES_NO_INLINE   = (1 << 1),
    PGC_OPTIONS_AUTOSCALE               = (1 << 2),
} PGC_OPTIONS;

#define PGC_OPTIONS_DEFAULT (PGC_OPTIONS_EVICT_PAGES_NO_INLINE | PGC_OPTIONS_AUTOSCALE)

typedef struct pgc_entry {
    Word_t section;             // the section this belongs to
    Word_t metric_id;           // the metric this belongs to
    time_t start_time_s;        // the start time of the page
    time_t end_time_s;          // the end time of the page
    size_t size;                // the size in bytes of the allocation, outside the cache
    void *data;                 // a pointer to data outside the cache
    uint32_t update_every_s;    // the update every of the page
    bool hot;                   // true if this entry is currently being collected
    uint8_t *custom_data;
} PGC_ENTRY;

typedef void (*free_clean_page_callback)(PGC *cache, PGC_ENTRY entry);
typedef void (*save_dirty_page_callback)(PGC *cache, PGC_ENTRY *entries_array, PGC_PAGE **pages_array, size_t entries);
typedef void (*save_dirty_init_callback)(PGC *cache, Word_t section);
typedef int64_t (*dynamic_target_cache_size_callback)(PGC *cache);
typedef size_t (*nominal_page_size_callback)(void *);

// everything a cache is made of. The cache copies it before its evictor thread starts and changes nothing of it
// afterwards, so the thread, and every other one that touches the cache, sees the cache as it was made.
struct pgc_config {
    const char *name;                                   // up to PGC_NAME_MAX characters
    size_t clean_size_bytes;                            // at least 1 MiB
    free_clean_page_callback free_clean_cb;
    size_t max_dirty_pages_per_flush;
    save_dirty_init_callback save_init_cb;
    save_dirty_page_callback save_dirty_cb;
    size_t max_pages_per_inline_eviction;
    size_t max_inline_evictors;
    size_t max_skip_pages_per_inline_eviction;
    size_t max_flushes_inline;
    PGC_OPTIONS options;
    size_t partitions;                                  // 0: twice the cpus; clamped to [4, 256]
    size_t additional_bytes_per_page;

    // the settings the cache follows, passed by the creator so that the cache reads no engine configuration;
    // cpus (>= 1) sizes the default partitions and the evictors and flushers the cache supports
    bool statistics;
    bool use_all_ram;
    uint64_t out_of_memory_protection_bytes;
    size_t cpus;

    // what the cache belongs to and asks
    struct dbengine_engine *engine;                     // NULL for a cache that belongs to no engine (the tests')
    dynamic_target_cache_size_callback dynamic_target_size_cb;  // asked, on every usage check, for the size to aim
                                                        // at (it wins when larger than the cache's own); NULL: none
    nominal_page_size_callback nominal_page_size_cb;    // the memory a page's data takes, when it is not the entry's
                                                        // size; NULL: the entry's size
};

// create a cache from its configuration, which the caller may discard afterwards
PGC *pgc_create(const struct pgc_config *cfg);

// destroy the cache; its dirty pages are flushed, and saved through the save callbacks only when flush is set
// false when the cache stays allocated: pages are still referenced (or there is no cache)
bool pgc_destroy(PGC *cache, bool flush);

#define PGC_SECTION_ALL ((Word_t)0)
void pgc_flush_dirty_pages(PGC *cache, Word_t section);
void pgc_flush_all_hot_and_dirty_pages(PGC *cache, Word_t section);

// add a page to the cache and return a pointer to it
PGC_PAGE *pgc_page_add_and_acquire(PGC *cache, PGC_ENTRY entry, bool *added);

// get another reference counter on an already referenced page
PGC_PAGE *pgc_page_dup(PGC *cache, PGC_PAGE *page);

// release a page (all pointers to it are now invalid)
void pgc_page_release(PGC *cache, PGC_PAGE *page);

// mark a hot page dirty, and release it
void pgc_page_hot_to_dirty_and_release(PGC *cache, PGC_PAGE *page, bool never_flush);

// find a page from the cache
typedef enum {
    PGC_SEARCH_EXACT,
    PGC_SEARCH_CLOSEST,
    PGC_SEARCH_FIRST,
    PGC_SEARCH_NEXT,
    PGC_SEARCH_LAST,
    PGC_SEARCH_PREV,
} PGC_SEARCH;

PGC_PAGE *pgc_page_get_and_acquire(PGC *cache, Word_t section, Word_t metric_id, time_t start_time_s, PGC_SEARCH method);

// get information from an acquired page
Word_t pgc_page_section(PGC_PAGE *page);
Word_t pgc_page_metric(PGC_PAGE *page);
time_t pgc_page_start_time_s(PGC_PAGE *page);
time_t pgc_page_end_time_s(PGC_PAGE *page);
uint32_t pgc_page_update_every_s(PGC_PAGE *page);
uint32_t pgc_page_fix_update_every(PGC_PAGE *page, uint32_t update_every_s);
time_t pgc_page_fix_end_time_s(PGC_PAGE *page, time_t end_time_s);
void *pgc_page_data(PGC_PAGE *page);
void *pgc_page_custom_data(PGC *cache, PGC_PAGE *page);
size_t pgc_page_data_size(PGC *cache, PGC_PAGE *page);
bool pgc_is_page_hot(PGC_PAGE *page);
bool pgc_is_page_dirty(PGC_PAGE *page);
bool pgc_is_page_clean(PGC_PAGE *page);
void pgc_reset_hot_max(PGC *cache);
int64_t pgc_get_current_cache_size(PGC *cache);
int64_t pgc_get_wanted_cache_size(PGC *cache);

// resetting the end time of a hot page
void pgc_page_hot_set_end_time_s(PGC *cache, PGC_PAGE *page, time_t end_time_s, size_t additional_bytes);
bool pgc_page_to_clean_evict_or_release(PGC *cache, PGC_PAGE *page);

typedef bool (*migrate_to_v2_callback)(Word_t section, unsigned datafile_fileno, uint8_t type, Pvoid_t JudyL_metrics, Pvoid_t JudyL_extents_pos, size_t count_of_unique_extents, size_t count_of_unique_metrics, size_t count_of_unique_pages, void *data);
void pgc_open_cache_to_journal_v2(
    PGC *cache,
    Word_t section,
    unsigned datafile_fileno,
    uint8_t type,
    migrate_to_v2_callback cb,
    void *data,
    bool startup);
void pgc_open_evict_clean_pages_of_datafile(PGC *cache, struct dbengine_datafile *datafile);
size_t pgc_count_clean_pages_having_data_ptr(PGC *cache, Word_t section, void *ptr);
size_t pgc_count_hot_pages_having_data_ptr(PGC *cache, Word_t section, void *ptr);

// return true when there is more work to do
bool pgc_evict_pages(PGC *cache, size_t max_skip, size_t max_evict);
bool pgc_flush_pages(PGC *cache);

struct dbengine_cache_stats pgc_get_statistics(PGC *cache);
size_t pgc_hot_and_dirty_entries(PGC *cache);

struct aral_statistics *pgc_aral_stats(void);

static inline size_t indexing_partition(Word_t ptr, Word_t modulo) __attribute__((const));
static inline size_t indexing_partition(Word_t ptr, Word_t modulo) {
    XXH64_hash_t hash = XXH3_64bits(&ptr, sizeof(ptr));
    return hash % modulo;
}

// the evictors and flushers a cache runs are functions of the cpus it was created for
static inline size_t pgc_evictors_for_cpus(size_t cpus) {
    return 1 + cpus / 2;
}

size_t pgc_max_evictors(PGC *cache);
size_t pgc_max_flushers(PGC *cache);

// the engine the cache was made for; NULL for a cache that belongs to none (the tests')
struct dbengine_engine *pgc_engine(PGC *cache);

#endif // DBENGINE_CACHE_H
