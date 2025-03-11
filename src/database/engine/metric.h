// SPDX-License-Identifier: GPL-3.0-or-later
#ifndef DBENGINE_METRIC_H
#define DBENGINE_METRIC_H

#include "../rrd.h"

typedef struct metric METRIC;
typedef struct mrg MRG;

typedef struct mrg_entry {
    nd_uuid_t *uuid;
    Word_t section;
    time_t first_time_s;
    time_t last_time_s;
    uint32_t latest_update_every_s;
} MRG_ENTRY;

struct mrg_statistics {
    // --- non-atomic --- under a write lock

    size_t entries;
    int64_t size;    // total memory used, with indexing

    size_t additions;
    size_t additions_duplicate;

    size_t deletions;
    size_t delete_having_retention_or_referenced;
    size_t delete_misses;

    // --- atomic --- multiple readers / writers

    PAD64(ssize_t) entries_acquired;
    PAD64(ssize_t) current_references;

    PAD64(size_t) search_hits;
    PAD64(size_t) search_misses;

    PAD64(size_t) writers;
    PAD64(size_t) writers_conflicts;
};

MRG *mrg_create(void);

// returns the number of metrics that were freed, but were still referenced
size_t mrg_destroy(MRG *mrg);

METRIC *mrg_metric_dup(MRG *mrg, METRIC *metric);
void mrg_metric_release(MRG *mrg, METRIC *metric);

METRIC *mrg_metric_add_and_acquire(MRG *mrg, MRG_ENTRY entry, bool *ret);
METRIC *mrg_metric_get_and_acquire_by_id(MRG *mrg, UUIDMAP_ID id, Word_t section);
METRIC *mrg_metric_get_and_acquire_by_uuid(MRG *mrg, nd_uuid_t *uuid, Word_t section);
bool mrg_metric_release_and_delete(MRG *mrg, METRIC *metric);

Word_t mrg_metric_id(MRG *mrg, METRIC *metric);
nd_uuid_t *mrg_metric_uuid(MRG *mrg, METRIC *metric);
UUIDMAP_ID mrg_metric_uuidmap_id_dup(MRG *mrg, METRIC *metric);
Word_t mrg_metric_section(MRG *mrg, METRIC *metric);

bool mrg_metric_set_first_time_s(MRG *mrg, METRIC *metric, time_t first_time_s);
bool mrg_metric_set_first_time_s_if_bigger(MRG *mrg, METRIC *metric, time_t first_time_s);
time_t mrg_metric_get_first_time_s(MRG *mrg, METRIC *metric);

bool mrg_metric_set_clean_latest_time_s(MRG *mrg, METRIC *metric, time_t latest_time_s);
bool mrg_metric_set_hot_latest_time_s(MRG *mrg, METRIC *metric, time_t latest_time_s);
time_t mrg_metric_get_latest_time_s(MRG *mrg, METRIC *metric);
time_t mrg_metric_get_latest_clean_time_s(MRG *mrg, METRIC *metric);

bool mrg_metric_set_update_every(MRG *mrg, METRIC *metric, uint32_t update_every_s);
bool mrg_metric_set_update_every_s_if_zero(MRG *mrg, METRIC *metric, uint32_t update_every_s);
uint32_t mrg_metric_get_update_every_s(MRG *mrg, METRIC *metric);

void mrg_metric_expand_retention(MRG *mrg, METRIC *metric, time_t first_time_s, time_t last_time_s, uint32_t update_every_s);
void mrg_metric_get_retention(MRG *mrg, METRIC *metric, time_t *first_time_s, time_t *last_time_s, uint32_t *update_every_s);
bool mrg_metric_has_zero_disk_retention(MRG *mrg, METRIC *metric);
void mrg_metric_clear_retention(MRG *mrg, METRIC *metric);

#ifdef NETDATA_INTERNAL_CHECKS
bool mrg_metric_set_writer(MRG *mrg, METRIC *metric);
bool mrg_metric_clear_writer(MRG *mrg, METRIC *metric);
#endif

void mrg_get_statistics(MRG *mrg, struct mrg_statistics *s);
struct aral_statistics *mrg_aral_stats(void);

void mrg_update_metric_retention_and_granularity_by_uuid(
        MRG *mrg, Word_t section, nd_uuid_t *uuid,
        time_t first_time_s, time_t last_time_s,
        uint32_t update_every_s, time_t now_s);

#endif // DBENGINE_METRIC_H
