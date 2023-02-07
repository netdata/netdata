#ifndef DBENGINE_CACHE_H
#define DBENGINE_CACHE_H

#include "../rrd.h"

// CACHE COMPILE TIME CONFIGURATION
// #define PGC_COUNT_POINTS_COLLECTED 1

typedef struct pgc PGC;
typedef struct pgc_page PGC_PAGE;
#define PGC_NAME_MAX 23

typedef enum __attribute__ ((__packed__)) {
    PGC_OPTIONS_NONE       = 0,
    PGC_OPTIONS_EVICT_PAGES_INLINE = (1 << 0),
    PGC_OPTIONS_FLUSH_PAGES_INLINE = (1 << 1),
    PGC_OPTIONS_AUTOSCALE          = (1 << 2),
} PGC_OPTIONS;

#define PGC_OPTIONS_DEFAULT (PGC_OPTIONS_EVICT_PAGES_INLINE | PGC_OPTIONS_FLUSH_PAGES_INLINE | PGC_OPTIONS_AUTOSCALE)

typedef struct pgc_entry {
    Word_t section;             // the section this belongs to
    Word_t metric_id;           // the metric this belongs to
    time_t start_time_s;        // the start time of the page
    time_t end_time_s;          // the end time of the page
    size_t size;                // the size in bytes of the allocation, outside the cache
    void *data;                 // a pointer to data outside the cache
    uint32_t update_every_s;      // the update every of the page
    bool hot;                   // true if this entry is currently being collected
    uint8_t *custom_data;
} PGC_ENTRY;

#define PGC_CACHE_LINE_PADDING(x) uint8_t padding##x[128]

struct pgc_queue_statistics {
    size_t entries;
    size_t size;

    PGC_CACHE_LINE_PADDING(1);

    size_t max_entries;
    size_t max_size;

    PGC_CACHE_LINE_PADDING(2);

    size_t added_entries;
    size_t added_size;

    PGC_CACHE_LINE_PADDING(3);

    size_t removed_entries;
    size_t removed_size;

    PGC_CACHE_LINE_PADDING(4);
};

struct pgc_statistics {
    size_t wanted_cache_size;
    size_t current_cache_size;

    PGC_CACHE_LINE_PADDING(1);

    size_t added_entries;
    size_t added_size;

    PGC_CACHE_LINE_PADDING(2);

    size_t removed_entries;
    size_t removed_size;

    PGC_CACHE_LINE_PADDING(3);

    size_t entries;                 // all the entries (includes clean, dirty, host)
    size_t size;                    // all the entries (includes clean, dirty, host)

    size_t evicting_entries;
    size_t evicting_size;

    size_t flushing_entries;
    size_t flushing_size;

    size_t hot2dirty_entries;
    size_t hot2dirty_size;

    PGC_CACHE_LINE_PADDING(4);

    size_t acquires;
    PGC_CACHE_LINE_PADDING(4a);
    size_t releases;
    PGC_CACHE_LINE_PADDING(4b);
    size_t acquires_for_deletion;
    PGC_CACHE_LINE_PADDING(4c);

    size_t referenced_entries;      // all the entries currently referenced
    size_t referenced_size;         // all the entries currently referenced

    PGC_CACHE_LINE_PADDING(5);

    size_t searches_exact;
    size_t searches_exact_hits;
    size_t searches_exact_misses;

    PGC_CACHE_LINE_PADDING(6);

    size_t searches_closest;
    size_t searches_closest_hits;
    size_t searches_closest_misses;

    PGC_CACHE_LINE_PADDING(7);

    size_t flushes_completed;
    size_t flushes_completed_size;
    size_t flushes_cancelled;
    size_t flushes_cancelled_size;

#ifdef PGC_COUNT_POINTS_COLLECTED
    PGC_CACHE_LINE_PADDING(8);
    size_t points_collected;
#endif

    PGC_CACHE_LINE_PADDING(9);

    size_t insert_spins;
    size_t evict_spins;
    size_t release_spins;
    size_t acquire_spins;
    size_t delete_spins;
    size_t flush_spins;

    PGC_CACHE_LINE_PADDING(10);

    size_t workers_search;
    size_t workers_add;
    size_t workers_evict;
    size_t workers_flush;
    size_t workers_jv2_flush;
    size_t workers_hot2dirty;

    size_t evict_skipped;
    size_t hot_empty_pages_evicted_immediately;
    size_t hot_empty_pages_evicted_later;

    PGC_CACHE_LINE_PADDING(11);

    // events
    size_t events_cache_under_severe_pressure;
    size_t events_cache_needs_space_aggressively;
    size_t events_flush_critical;

    PGC_CACHE_LINE_PADDING(12);

    struct {
        PGC_CACHE_LINE_PADDING(0);
        struct pgc_queue_statistics hot;
        PGC_CACHE_LINE_PADDING(1);
        struct pgc_queue_statistics dirty;
        PGC_CACHE_LINE_PADDING(2);
        struct pgc_queue_statistics clean;
        PGC_CACHE_LINE_PADDING(3);
    } queues;
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
void pgc_page_hot_to_dirty_and_release(PGC *cache, PGC_PAGE *page);

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
time_t pgc_page_update_every_s(PGC_PAGE *page);
time_t pgc_page_fix_update_every(PGC_PAGE *page, time_t update_every_s);
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
void pgc_page_hot_set_end_time_s(PGC *cache, PGC_PAGE *page, time_t end_time_s);
bool pgc_page_to_clean_evict_or_release(PGC *cache, PGC_PAGE *page);

typedef void (*migrate_to_v2_callback)(Word_t section, unsigned datafile_fileno, uint8_t type, Pvoid_t JudyL_metrics, Pvoid_t JudyL_extents_pos, size_t count_of_unique_extents, size_t count_of_unique_metrics, size_t count_of_unique_pages, void *data);
void pgc_open_cache_to_journal_v2(PGC *cache, Word_t section, unsigned datafile_fileno, uint8_t type, migrate_to_v2_callback cb, void *data);
void pgc_open_evict_clean_pages_of_datafile(PGC *cache, struct rrdengine_datafile *datafile);
size_t pgc_count_clean_pages_having_data_ptr(PGC *cache, Word_t section, void *ptr);
size_t pgc_count_hot_pages_having_data_ptr(PGC *cache, Word_t section, void *ptr);

typedef size_t (*dynamic_target_cache_size_callback)(void);
void pgc_set_dynamic_target_cache_size_callback(PGC *cache, dynamic_target_cache_size_callback callback);

// return true when there is more work to do
bool pgc_evict_pages(PGC *cache, size_t max_skip, size_t max_evict);
bool pgc_flush_pages(PGC *cache, size_t max_flushes);

struct pgc_statistics pgc_get_statistics(PGC *cache);
size_t pgc_hot_and_dirty_entries(PGC *cache);

struct aral_statistics *pgc_aral_statistics(void);
size_t pgc_aral_structures(void);
size_t pgc_aral_overhead(void);

#endif // DBENGINE_CACHE_H
