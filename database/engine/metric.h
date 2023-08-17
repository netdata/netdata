#ifndef DBENGINE_METRIC_H
#define DBENGINE_METRIC_H

#include "../rrd.h"

#define MRG_CACHE_LINE_PADDING(x) uint8_t padding##x[64]

typedef struct metric METRIC;
typedef struct mrg MRG;

typedef struct mrg_entry {
    uuid_t *uuid;
    Word_t section;
    time_t first_time_s;
    time_t last_time_s;
    uint32_t latest_update_every_s;
} MRG_ENTRY;

struct mrg_statistics {
    // --- non-atomic --- under a write lock

    size_t entries;
    size_t size;    // total memory used, with indexing

    size_t additions;
    size_t additions_duplicate;

    size_t deletions;
    size_t delete_having_retention_or_referenced;
    size_t delete_misses;

    MRG_CACHE_LINE_PADDING(0);

    // --- atomic --- multiple readers / writers

    size_t entries_referenced;

    MRG_CACHE_LINE_PADDING(1);
    size_t entries_with_retention;

    MRG_CACHE_LINE_PADDING(2);
    size_t current_references;

    MRG_CACHE_LINE_PADDING(3);
    size_t search_hits;
    size_t search_misses;

    MRG_CACHE_LINE_PADDING(4);
    size_t writers;
    size_t writers_conflicts;
};

MRG *mrg_create(ssize_t partitions);
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

void mrg_get_statistics(MRG *mrg, struct mrg_statistics *s);
size_t mrg_aral_structures(void);
size_t mrg_aral_overhead(void);


void mrg_update_metric_retention_and_granularity_by_uuid(
        MRG *mrg, Word_t section, uuid_t *uuid,
        time_t first_time_s, time_t last_time_s,
        time_t update_every_s, time_t now_s);

#endif // DBENGINE_METRIC_H
