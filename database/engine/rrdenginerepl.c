// SPDX-License-Identifier: GPL-3.0-or-later
#include "rrdengine.h"

static void rrdeng_store_past_metrics_page_init(RRDDIM_PAST_DATA *dpd) {
    RRDDIM *rd = dpd->rd;

    struct rrddim_tier *t0 = rd->tiers[0];
    struct rrdeng_metric_handle *mh = (struct rrdeng_metric_handle *) t0->db_metric_handle;
    struct pg_cache_page_index *page_index = mh->page_index;

    dpd->ctx = mh->ctx;

    uv_rwlock_wrlock(&page_index->lock);
    ++page_index->writers;
    uv_rwlock_wrunlock(&page_index->lock);

    void *page = rrdeng_create_page(mh->ctx, &page_index->id, &dpd->descr);
    fatal_assert(page);
}

static void rrdeng_store_past_metrics_page(RRDDIM_PAST_DATA *dpd) {
    struct page_cache *pg_cache;
    struct rrdengine_instance *ctx;
    struct rrdeng_page_descr *descr;

    descr = dpd->descr;
    ctx = dpd->ctx;
    pg_cache = &ctx->pg_cache;

    // copy the dim past dataq in this page
    // TODO: Ask stelios/costa about this
    // Page alignment can be handled with zero values.
    // Every new past data page can reach the aligment with zeros if the values are not enough.
    // for (page_length - rd->rrdset->rrddim_page_alignment) fill with zeros OR
    // simply increase the length since zeros are already there
    rrdeng_page_descr_mutex_lock(ctx, descr);
    memcpy(descr->pg_cache_descr->page, dpd->page, (size_t) dpd->page_length);
    descr->page_length  = dpd->page_length;
    descr->end_time = dpd->end_time;
    descr->start_time = dpd->start_time;
    rrdeng_page_descr_mutex_unlock(ctx, descr);

    // prepare the pg descr to insert and commit the dbengine page
    dpd->page_correlation_id = rrd_atomic_fetch_add(&pg_cache->committed_page_index.latest_corr_id, 1);
    pg_cache_atomic_set_pg_info(descr, descr->end_time, descr->page_length);
}

static void rrdeng_flush_past_metrics_page(RRDDIM_PAST_DATA *dpd) {
    struct rrdengine_instance *ctx = dpd->ctx;

    unsigned long new_metric_API_producers;
    unsigned long old_metric_API_max_producers;
    unsigned long ret_metric_API_max_producers;

    new_metric_API_producers = rrd_atomic_add_fetch(&ctx->stats.metric_API_producers, 1);

    while (unlikely(new_metric_API_producers > (old_metric_API_max_producers = ctx->metric_API_max_producers))) {
        ret_metric_API_max_producers =
            ulong_compare_and_swap(&ctx->metric_API_max_producers, old_metric_API_max_producers, new_metric_API_producers);

        if (old_metric_API_max_producers == ret_metric_API_max_producers)
            break;
    }

    struct rrddim_tier *t0 = dpd->rd->tiers[0];
    struct rrdeng_metric_handle *mh = (struct rrdeng_metric_handle *) t0->db_metric_handle;
    struct pg_cache_page_index *page_index = mh->page_index;
    struct rrdeng_page_descr *descr = dpd->descr;

    // page flags check. Need to enable the pg_cache_descr_state flags
    pg_cache_insert(ctx, page_index, descr);
    // Try to update the time start and end of the metric.
    pg_cache_add_new_metric_time(page_index, descr);

    if (likely(descr->page_length)) {
        rrd_stat_atomic_add(&ctx->stats.metric_API_producers, -1);

        if (rrdeng_page_has_only_empty_metrics(descr)) {
            pg_cache_put(ctx, descr);
            pg_cache_punch_hole(ctx, descr, 1, 0, NULL);
        } else {
            rrdeng_commit_page(ctx, descr, dpd->page_correlation_id);
        }

        return;
    }

    freez(descr->pg_cache_descr->page);
    rrdeng_destroy_pg_cache_descr(ctx, descr->pg_cache_descr);
    freez(descr);
}

static void rrdeng_store_past_metrics_page_finalize(RRDDIM_PAST_DATA *dpd){
    struct rrddim_tier *t0 = dpd->rd->tiers[0];
    struct rrdeng_metric_handle *mh = (struct rrdeng_metric_handle *) t0->db_metric_handle;
    struct pg_cache_page_index* page_index = mh->page_index;

    uv_rwlock_wrlock(&page_index->lock);
    --page_index->writers;
    uv_rwlock_wrunlock(&page_index->lock);
}

// TODO: verify update every of replication/db pages match
bool rrdeng_store_past_metrics_realtime(RRDDIM *rd, RRDDIM_PAST_DATA *dpd)
{
    struct rrddim_tier *t0 = rd->tiers[0];
    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *) t0->db_collection_handle;
    struct rrdeng_page_descr *descr = handle->descr;

    if(!descr || !descr->pg_cache_descr) {
        error("dbenginerepl: No active descr or page for dimension %s.%s", rrdset_id(rd->rrdset), rrddim_id(rd));
        return false;
    }

    storage_number *dbengine_page = (storage_number *) descr->pg_cache_descr->page;
    if (!dbengine_page) {
        error("dbenginerepl: Found null dbengine page");
        return false;
    }

    uint64_t dbengine_page_start_time = descr->start_time / USEC_PER_SEC;
    uint64_t dbengine_page_end_time = descr->end_time / USEC_PER_SEC;

    uint64_t replication_page_start_time = dpd->start_time / USEC_PER_SEC;
    uint64_t replication_page_end_time = dpd->end_time / USEC_PER_SEC;

    // TODO: ask stelios/costa what we should do in this case.
    if (replication_page_end_time > dbengine_page_end_time) {
        error("dbenginerepl: Replication page contains data in the future. %lu seconds will be dropped",
              replication_page_end_time - dbengine_page_end_time);
    }

    bool no_overlap = (replication_page_end_time < dbengine_page_start_time) ||
                      (replication_page_start_time > dbengine_page_end_time);
    if (no_overlap) {
        rrdeng_store_past_metrics_page_init(dpd);
        rrdeng_store_past_metrics_page(dpd);
        rrdeng_flush_past_metrics_page(dpd);
        rrdeng_store_past_metrics_page_finalize(dpd);
        return true;
    }

    // handle non-overlapping LHS of replication data, ie:
    //      [replication_page_start_time, dbengine_page_start_time)
    if (replication_page_start_time < dbengine_page_start_time) {
        size_t rhs_storage_numbers_to_drop =
            (replication_page_end_time - dbengine_page_start_time) / rd->update_every + 1;
        size_t rhs_bytes_to_drop = rhs_storage_numbers_to_drop * sizeof(storage_number);

        RRDDIM_PAST_DATA lhs_dpd = *dpd;
        lhs_dpd.end_time -= (rhs_storage_numbers_to_drop * rd->update_every) * USEC_PER_SEC;
        lhs_dpd.page_length -= rhs_bytes_to_drop;

        rrdeng_store_past_metrics_page_init(&lhs_dpd);
        rrdeng_store_past_metrics_page(&lhs_dpd);
        rrdeng_flush_past_metrics_page(&lhs_dpd);
        rrdeng_store_past_metrics_page_finalize(&lhs_dpd);

        // bookeeping for the next interval
        dpd->start_time = lhs_dpd.end_time + (rd->update_every * USEC_PER_SEC);
        dpd->page += lhs_dpd.page_length;
        dpd->page_length -= lhs_dpd.page_length;
        replication_page_start_time = dpd->start_time / USEC_PER_SEC;
    }

    // handle overlapping part, ie:
    //      [dbengine_page_start_time, min(dbengine_page_end_time, replication_page_end_time)]
    if (replication_page_start_time >= dbengine_page_start_time &&
        replication_page_start_time < dbengine_page_end_time)
    {
        RRDDIM_PAST_DATA overlap_dpd = *dpd;
        overlap_dpd.end_time = MIN(replication_page_end_time, dbengine_page_end_time) * USEC_PER_SEC;
        overlap_dpd.page_length = MIN(descr->page_length, dpd->page_length);

        size_t dbengine_page_offset = (replication_page_start_time - dbengine_page_start_time) / rd->update_every;
        memcpy(dbengine_page + dbengine_page_offset, overlap_dpd.page, overlap_dpd.page_length);
    }

    return true;
}
