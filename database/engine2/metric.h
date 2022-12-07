#ifndef DBENGINE_METRIC_H
#define DBENGINE_METRIC_H

#include "../rrd.h"

typedef struct metric METRIC;
typedef struct mrg MRG;

typedef struct mrg_entry {
    uuid_t uuid;
    Word_t section;
    size_t pages;
    time_t first_time_t;
    time_t last_time_t;
    time_t latest_update_every;
} MRG_ENTRY;



#endif // DBENGINE_METRIC_H
