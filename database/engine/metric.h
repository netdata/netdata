#ifndef DBENGINE_METRIC_H
#define DBENGINE_METRIC_H

#include "../rrd.h"

#define MRG_PARTITIONS 10

typedef struct metric METRIC;
typedef struct mrg MRG;

typedef struct mrg_entry {
    uuid_t uuid;
    Word_t section;
    time_t first_time_s;
    time_t last_time_s;
    uint32_t latest_update_every_s;
} MRG_ENTRY;

struct mrg_statistics {
    size_t entries;
    size_t entries_referenced;
    size_t entries_with_retention;

    size_t size;                // total memory used, with indexing

    size_t current_references;

    size_t additions;
    size_t additions_duplicate;

    size_t deletions;
    size_t delete_having_retention_or_referenced;
    size_t delete_misses;

    size_t search_hits;
    size_t search_misses;

    size_t writers;
    size_t writers_conflicts;
};

MRG *mrg_create(void);
void mrg_destroy(MRG *mrg);

METRIC *mrg_metric_dup(MRG *mrg, METRIC *metric);
bool mrg_metric_release(MRG *mrg, METRIC *metric);

METRIC *mrg_metric_add_and_acquire(MRG *mrg, MRG_ENTRY entry, bool *ret);
METRIC *mrg_metric_get_and_acquire(MRG *mrg, uuid_t *uuid, Word_t section);
bool mrg_metric_release_and_delete(MRG *mrg, METRIC *metric);

Word_t mrg_metric_id(MRG *mrg, METRIC *metric);
uuid_t *mrg_metric_uuid(MRG *mrg, METRIC *metric);
Word_t mrg_metric_section(MRG *mrg, METRIC *metric);

bool mrg_metric_set_first_time_s(MRG *mrg, METRIC *metric, time_t first_time_s);
bool mrg_metric_set_first_time_s_if_bigger(MRG *mrg, METRIC *metric, time_t first_time_s);
time_t mrg_metric_get_first_time_s(MRG *mrg, METRIC *metric);

bool mrg_metric_set_clean_latest_time_s(MRG *mrg, METRIC *metric, time_t latest_time_s);
bool mrg_metric_set_hot_latest_time_s(MRG *mrg, METRIC *metric, time_t latest_time_s);
time_t mrg_metric_get_latest_time_s(MRG *mrg, METRIC *metric);

bool mrg_metric_set_update_every(MRG *mrg, METRIC *metric, time_t update_every_s);
bool mrg_metric_set_update_every_s_if_zero(MRG *mrg, METRIC *metric, time_t update_every_s);
time_t mrg_metric_get_update_every_s(MRG *mrg, METRIC *metric);

void mrg_metric_expand_retention(MRG *mrg, METRIC *metric, time_t first_time_s, time_t last_time_s, time_t update_every_s);
void mrg_metric_get_retention(MRG *mrg, METRIC *metric, time_t *first_time_s, time_t *last_time_s, time_t *update_every_s);
bool mrg_metric_zero_disk_retention(MRG *mrg __maybe_unused, METRIC *metric);

bool mrg_metric_set_writer(MRG *mrg, METRIC *metric);
bool mrg_metric_clear_writer(MRG *mrg, METRIC *metric);

struct mrg_statistics mrg_get_statistics(MRG *mrg);
size_t mrg_aral_structures(void);
size_t mrg_aral_overhead(void);

#endif // DBENGINE_METRIC_H
