#ifndef DBENGINE_CACHE_H
#define DBENGINE_CACHE_H

#include "../rrd.h"

typedef struct pgc PGC;
typedef struct pgc_page PGC_PAGE;

typedef enum __attribute__ ((__packed__)) {
    PGC_OPTIONS_NONE       = 0,
    PGC_OPTIONS_EVICT_PAGES_INLINE = (1 << 0),
    PGC_OPTIONS_FLUSH_PAGES_INLINE = (1 << 1),
} PGC_OPTIONS;

#define PGC_OPTIONS_DEFAULT (PGC_OPTIONS_EVICT_PAGES_INLINE | PGC_OPTIONS_FLUSH_PAGES_INLINE)

typedef struct pgc_entry {
    bool hot;                   // true if this entry is currently being collected
    Word_t section;             // the section this belongs to
    Word_t metric_id;           // the metric this belongs to
    time_t start_time_t;        // the start time of the page
    time_t end_time_t;          // the end time of the page
    time_t update_every;        // the update every of the page
    size_t size;                // the size in bytes of the allocation, outside the cache
    void *data;                 // a pointer to data outside the cache
} PGC_ENTRY;

typedef void (*free_clean_page_callback)(PGC *cache, PGC_ENTRY entry);
typedef void (*save_dirty_page_callback)(PGC *cache, PGC_ENTRY *array, size_t entries);

// create a cache
PGC *pgc_create(size_t max_clean_size, free_clean_page_callback pgc_free_clean_cb,
                size_t max_dirty_pages_to_save_at_once, save_dirty_page_callback pgc_save_dirty_cb,
                PGC_OPTIONS options);

// destroy the cache
void pgc_destroy(PGC *cache);

// add a page to the cache and return a pointer to it
PGC_PAGE *pgc_page_add_and_acquire(PGC *cache, PGC_ENTRY entry);

// release a page (all pointers to it are now invalid)
void pgc_page_release(PGC *cache, PGC_PAGE *page);

// mark a hot page dirty, and release it
void pgc_page_hot_to_dirty_and_release(PGC *cache, PGC_PAGE *page);

// find a page from the cache
PGC_PAGE *pgc_page_get_and_acquire(PGC *cache, Word_t section, Word_t metric_id, time_t start_time_t, bool exact);

// get information from an acquired page
Word_t pgc_page_section(PGC_PAGE *page);
Word_t pgc_page_metric(PGC_PAGE *page);
time_t pgc_page_start_time_t(PGC_PAGE *page);
time_t pgc_page_end_time_t(PGC_PAGE *page);
time_t pgc_page_update_every(PGC_PAGE *page);
void *pgc_page_data(PGC_PAGE *page);

// resetting the end time of a hot page
void pgc_page_hot_set_end_time_t(PGC *cache, PGC_PAGE *page, time_t end_time_t);

void pgc_evict_pages(PGC *cache);
void pgc_flush_pages(PGC *cache);

#endif // DBENGINE_CACHE_H
