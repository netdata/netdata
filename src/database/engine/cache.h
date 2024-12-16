// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef DBENGINE_CACHE_H
#define DBENGINE_CACHE_H

#include "datafile.h"
#include "../rrd.h"

// CACHE COMPILE TIME CONFIGURATION
// #define PGC_COUNT_POINTS_COLLECTED 1

typedef struct pgc PGC;
typedef struct pgc_page PGC_PAGE;
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

struct pgc_size_histogram_entry {
    size_t upto;
    size_t count;
};

#define PGC_SIZE_HISTOGRAM_ENTRIES 15
#define PGC_QUEUE_HOT   0
#define PGC_QUEUE_DIRTY 1
#define PGC_QUEUE_CLEAN 2

struct pgc_size_histogram {
    struct pgc_size_histogram_entry array[PGC_SIZE_HISTOGRAM_ENTRIES];
};

struct pgc_queue_statistics {
    CACHE_LINE_PADDING();
    struct pgc_size_histogram size_histogram;

    CACHE_LINE_PADDING();
    size_t entries;
    CACHE_LINE_PADDING();
    size_t size;

    CACHE_LINE_PADDING();
    size_t max_entries;
    CACHE_LINE_PADDING();
    size_t max_size;

    CACHE_LINE_PADDING();
    size_t added_entries;
    CACHE_LINE_PADDING();
    size_t added_size;

    CACHE_LINE_PADDING();
    size_t removed_entries;
    CACHE_LINE_PADDING();
    size_t removed_size;

    CACHE_LINE_PADDING();
};

struct pgc_statistics {
    CACHE_LINE_PADDING();
    size_t wanted_cache_size;
    CACHE_LINE_PADDING();
    size_t current_cache_size;
    CACHE_LINE_PADDING();

    // ----------------------------------------------------------------------------------------------------------------
    // volume

    CACHE_LINE_PADDING();
    size_t entries;                 // all the entries (includes clean, dirty, hot)
    CACHE_LINE_PADDING();
    size_t size;                    // all the entries (includes clean, dirty, hot)

    CACHE_LINE_PADDING();
    size_t referenced_entries;      // all the entries currently referenced
    CACHE_LINE_PADDING();
    size_t referenced_size;         // all the entries currently referenced

    CACHE_LINE_PADDING();
    size_t added_entries;
    CACHE_LINE_PADDING();
    size_t added_size;

    CACHE_LINE_PADDING();
    size_t removed_entries;
    CACHE_LINE_PADDING();
    size_t removed_size;

#ifdef PGC_COUNT_POINTS_COLLECTED
    CACHE_LINE_PADDING();
    size_t points_collected;
#endif

    // ----------------------------------------------------------------------------------------------------------------
    // migrations

    CACHE_LINE_PADDING();
    size_t evicting_entries;
    CACHE_LINE_PADDING();
    size_t evicting_size;

    CACHE_LINE_PADDING();
    size_t flushing_entries;
    CACHE_LINE_PADDING();
    size_t flushing_size;

    CACHE_LINE_PADDING();
    size_t hot2dirty_entries;
    CACHE_LINE_PADDING();
    size_t hot2dirty_size;

    CACHE_LINE_PADDING();
    size_t hot_empty_pages_evicted_immediately;
    CACHE_LINE_PADDING();
    size_t hot_empty_pages_evicted_later;

    // ----------------------------------------------------------------------------------------------------------------
    // workload

    CACHE_LINE_PADDING();
    size_t acquires;
    CACHE_LINE_PADDING();
    size_t releases;

    CACHE_LINE_PADDING();
    size_t acquires_for_deletion;

    CACHE_LINE_PADDING();
    size_t searches_exact;
    CACHE_LINE_PADDING();
    size_t searches_exact_hits;
    CACHE_LINE_PADDING();
    size_t searches_exact_misses;

    CACHE_LINE_PADDING();
    size_t searches_closest;
    CACHE_LINE_PADDING();
    size_t searches_closest_hits;
    CACHE_LINE_PADDING();
    size_t searches_closest_misses;

    CACHE_LINE_PADDING();
    size_t flushes_completed;
    CACHE_LINE_PADDING();
    size_t flushes_completed_size;
    CACHE_LINE_PADDING();
    size_t flushes_cancelled_size;

    // ----------------------------------------------------------------------------------------------------------------
    // critical events

    CACHE_LINE_PADDING();
    size_t events_cache_under_severe_pressure;
    CACHE_LINE_PADDING();
    size_t events_cache_needs_space_aggressively;
    CACHE_LINE_PADDING();
    size_t events_flush_critical;

    // ----------------------------------------------------------------------------------------------------------------
    // worker threads

    CACHE_LINE_PADDING();
    size_t workers_search;
    CACHE_LINE_PADDING();
    size_t workers_add;
    CACHE_LINE_PADDING();
    size_t workers_evict;
    CACHE_LINE_PADDING();
    size_t workers_flush;
    CACHE_LINE_PADDING();
    size_t workers_jv2_flush;
    CACHE_LINE_PADDING();
    size_t workers_hot2dirty;

    // ----------------------------------------------------------------------------------------------------------------
    // waste events

    // waste events - spins
    CACHE_LINE_PADDING();
    size_t waste_insert_spins;
    CACHE_LINE_PADDING();
    size_t waste_evict_useless_spins;
    CACHE_LINE_PADDING();
    size_t waste_release_spins;
    CACHE_LINE_PADDING();
    size_t waste_acquire_spins;
    CACHE_LINE_PADDING();
    size_t waste_delete_spins;

    // waste events - eviction
    CACHE_LINE_PADDING();
    size_t waste_evict_relocated;
    CACHE_LINE_PADDING();
    size_t waste_evict_thread_signals;
    CACHE_LINE_PADDING();
    size_t waste_evictions_inline_on_add;
    CACHE_LINE_PADDING();
    size_t waste_evictions_inline_on_release;

    // waste events - flushing
    CACHE_LINE_PADDING();
    size_t waste_flush_on_add;
    CACHE_LINE_PADDING();
    size_t waste_flush_on_release;
    CACHE_LINE_PADDING();
    size_t waste_flushes_cancelled;

    // ----------------------------------------------------------------------------------------------------------------
    // per queue statistics

    struct pgc_queue_statistics queues[3];
};

typedef void (*free_clean_page_callback)(PGC *cache, PGC_ENTRY entry);
typedef void (*save_dirty_page_callback)(PGC *cache, PGC_ENTRY *entries_array, PGC_PAGE **pages_array, size_t entries);
typedef void (*save_dirty_init_callback)(PGC *cache, Word_t section);
// create a cache
PGC *pgc_create(const char *name,
                size_t clean_size_bytes, free_clean_page_callback pgc_free_clean_cb,
                size_t max_dirty_pages_per_flush, save_dirty_init_callback pgc_save_init_cb, save_dirty_page_callback pgc_save_dirty_cb,
                size_t max_pages_per_inline_eviction, size_t max_inline_evictors,
                size_t max_skip_pages_per_inline_eviction,
                size_t max_flushes_inline,
                PGC_OPTIONS options, size_t partitions, size_t additional_bytes_per_page);

// destroy the cache
void pgc_destroy(PGC *cache);

#define PGC_SECTION_ALL ((Word_t)0)
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
size_t pgc_get_current_cache_size(PGC *cache);
size_t pgc_get_wanted_cache_size(PGC *cache);

// resetting the end time of a hot page
void pgc_page_hot_set_end_time_s(PGC *cache, PGC_PAGE *page, time_t end_time_s, size_t additional_bytes);
bool pgc_page_to_clean_evict_or_release(PGC *cache, PGC_PAGE *page);

typedef void (*migrate_to_v2_callback)(Word_t section, unsigned datafile_fileno, uint8_t type, Pvoid_t JudyL_metrics, Pvoid_t JudyL_extents_pos, size_t count_of_unique_extents, size_t count_of_unique_metrics, size_t count_of_unique_pages, void *data);
void pgc_open_cache_to_journal_v2(PGC *cache, Word_t section, unsigned datafile_fileno, uint8_t type, migrate_to_v2_callback cb, void *data);
void pgc_open_evict_clean_pages_of_datafile(PGC *cache, struct rrdengine_datafile *datafile);
size_t pgc_count_clean_pages_having_data_ptr(PGC *cache, Word_t section, void *ptr);
size_t pgc_count_hot_pages_having_data_ptr(PGC *cache, Word_t section, void *ptr);

typedef size_t (*dynamic_target_cache_size_callback)(void);
void pgc_set_dynamic_target_cache_size_callback(PGC *cache, dynamic_target_cache_size_callback callback);

typedef size_t (*nominal_page_size_callback)(void *);
void pgc_set_nominal_page_size_callback(PGC *cache, nominal_page_size_callback callback);

// return true when there is more work to do
bool pgc_evict_pages(PGC *cache, size_t max_skip, size_t max_evict);
bool pgc_flush_pages(PGC *cache);

struct pgc_statistics pgc_get_statistics(PGC *cache);
size_t pgc_hot_and_dirty_entries(PGC *cache);

struct aral_statistics *pgc_aral_stats(void);

static inline size_t indexing_partition(Word_t ptr, Word_t modulo) __attribute__((const));
static inline size_t indexing_partition(Word_t ptr, Word_t modulo) {
    XXH64_hash_t hash = XXH3_64bits(&ptr, sizeof(ptr));
    return hash % modulo;
}

static inline size_t pgc_max_evictors(void) {
    return 1 + netdata_conf_cpus() / 2;
}

static inline size_t pgc_max_flushers(void) {
    return netdata_conf_cpus();
}

#endif // DBENGINE_CACHE_H
