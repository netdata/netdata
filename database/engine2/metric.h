#ifndef DBENGINE_METRIC_H
#define DBENGINE_METRIC_H

#include "../rrd.h"

typedef struct metric METRIC;
typedef struct mrg MRG;

typedef struct mrg_entry {
    uuid_t uuid;
    Word_t section;
    time_t first_time_t;
    time_t latest_time_t;
    uint32_t latest_update_every;
} MRG_ENTRY;

MRG *mrg_create(void);
void mrg_destroy(MRG *mrg);

METRIC *mrg_metric_dup(MRG *mrg, METRIC *metric);
void mrg_metric_release(MRG *mrg, METRIC *metric);

METRIC *mrg_metric_add_and_acquire(MRG *mrg, MRG_ENTRY entry, bool *ret);
METRIC *mrg_metric_get_and_acquire(MRG *mrg, uuid_t *uuid, Word_t section);
bool mrg_metric_release_and_delete(MRG *mrg, METRIC *metric);

Word_t mrg_metric_id(MRG *mrg, METRIC *metric);
uuid_t *mrg_metric_uuid(MRG *mrg, METRIC *metric);
Word_t mrg_metric_section(MRG *mrg, METRIC *metric);

bool mrg_metric_set_first_time_t(MRG *mrg, METRIC *metric, time_t first_time_t);
time_t mrg_metric_get_first_time_t(MRG *mrg, METRIC *metric);

bool mrg_metric_set_latest_time_t(MRG *mrg, METRIC *metric, time_t latest_time_t);
bool mrg_metric_set_hot_latest_time_t(MRG *mrg, METRIC *metric, time_t latest_time_t);
time_t mrg_metric_get_latest_time_t(MRG *mrg, METRIC *metric);

bool mrg_metric_set_update_every(MRG *mrg, METRIC *metric, time_t update_every);
time_t mrg_metric_get_update_every(MRG *mrg, METRIC *metric);

bool mrg_metric_set_update_every_if_zero(MRG *mrg, METRIC *metric, time_t update_every);

#endif // DBENGINE_METRIC_H
