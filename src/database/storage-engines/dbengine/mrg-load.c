// SPDX-License-Identifier: GPL-3.0-or-later

#include "mrg-internals.h"

ALWAYS_INLINE
static void mrg_metric_prepopulate(void *mrg_ptr, struct dbengine_tier *ctx, nd_uuid_t *uuid) {
    MRG *mrg = mrg_ptr;
    struct dbengine_engine *engine = mrg->engine;
    MRG_ENTRY entry = {
        .uuid = uuid,
        .section = (Word_t)ctx,    // the registry sections are the tiers
        .first_time_s = 0,
        .last_time_s = 0,
        .latest_update_every_s = 0,
    };
    bool added = false;
    METRIC *metric = metric_add_and_acquire(mrg, &entry, &added);
    if(likely(added)) {
        METRIC_SET(&engine->preload.acquired, engine->preload.counter++, metric);
        return;
    }
    mrg_metric_release(mrg, metric);
}

static void mrg_release_cb(Word_t idx __maybe_unused, METRIC *m, void *data) {
    MRG *mrg = data;
    struct dbengine_engine *engine = mrg->engine;
    if(mrg_metric_release(mrg, m))
        engine->preload.deleted++;
}

void mrg_metric_prepopulate_cleanup(MRG *mrg) {
    struct dbengine_engine *engine = mrg->engine;

    engine->preload.deleted = 0;
    METRIC_FREE(&engine->preload.acquired, mrg_release_cb, mrg);

    if(engine->preload.counter || engine->preload.deleted)
        nd_log(NDLS_DAEMON, NDLP_INFO, "MRG DUMP: Prepopulated %zu metrics, released %zu, deleted %zu",
               engine->preload.counter, engine->preload.counter - engine->preload.deleted, engine->preload.deleted);

    engine->preload.counter = 0;
}

// Pre-populate the registry from the embedder's list of known metrics, if it provides one
bool mrg_load(MRG *mrg) {
    const struct dbengine_config *cfg = &mrg->engine->cfg;
    if(!cfg->preload_metrics)
        return false;

    size_t processed_metrics = cfg->preload_metrics(mrg, mrg_metric_prepopulate);
    return processed_metrics > 0;
}
