// SPDX-License-Identifier: GPL-3.0-or-later

#include "mrg-internals.h"

DEFINE_JUDYL_TYPED(METRIC, METRIC *);
METRIC_JudyLSet acquired_metrics = { 0 };
size_t acquired_metrics_counter = 0;
size_t acquired_metrics_deleted = 0;

ALWAYS_INLINE
static void mrg_metric_prepopulate(MRG *mrg, Word_t section, nd_uuid_t *uuid) {
    MRG_ENTRY entry = {
        .uuid = uuid,
        .section = section,
        .first_time_s = 0,
        .last_time_s = 0,
        .latest_update_every_s = 0,
    };
    bool added = false;
    METRIC *metric = metric_add_and_acquire(mrg, &entry, &added);
    if(likely(added)) {
        METRIC_SET(&acquired_metrics, acquired_metrics_counter++, metric);
        return;
    }
    mrg_metric_release(mrg, metric);
}

static void mrg_release_cb(Word_t idx __maybe_unused, METRIC *m, void *data) {
    MRG *mrg = data;
    if(mrg_metric_release(mrg, m))
        acquired_metrics_deleted++;
}

void mrg_metric_prepopulate_cleanup(MRG *mrg) {
    acquired_metrics_deleted = 0;
    METRIC_FREE(&acquired_metrics, mrg_release_cb, mrg);

    if(acquired_metrics_counter || acquired_metrics_deleted)
        nd_log(NDLS_DAEMON, NDLP_INFO, "MRG DUMP: Prepopulated %zu metrics, released %zu, deleted %zu",
               acquired_metrics_counter, acquired_metrics_counter - acquired_metrics_deleted, acquired_metrics_deleted);

    acquired_metrics_counter = 0;
}

// Main function to load metrics from the database
bool mrg_load(MRG *mrg) {
    size_t processed_metrics = populate_metrics_from_database(mrg, (void (*)(void *, Word_t, nd_uuid_t *))mrg_metric_prepopulate);
    return processed_metrics > 0;
}
