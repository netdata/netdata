#ifndef DBENGINE_CACHE_H
#define DBENGINE_CACHE_H

#include "../rrd.h"

typedef struct pgc PGC;
typedef struct pgc_page PGC_PAGE;

typedef struct pgc_entry {
    bool hot;                   // true if this entry is currently being collected
    Word_t section;             // the section this belongs to
    Word_t metric_id;           // the metric this belongs to
    Word_t start_time_t;        // the start time of the page
    size_t size;                // the size in bytes of the allocation, outside the cache
    void *data;                 // a pointer to data outside the cache
} PGC_ENTRY;

// create a cache
PGC *pgc_create(size_t max_size);

// add a page to the cache
PGC_PAGE *pgc_page_add_and_acquire(PGC *cache, PGC_ENTRY entry);


#endif // DBENGINE_CACHE_H
