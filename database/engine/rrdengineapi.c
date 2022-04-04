// SPDX-License-Identifier: GPL-3.0-or-later
#include "rrdengine.h"
#include "../storage_engine.h"

int default_rrdeng_page_cache_mb = 32;
int default_rrdeng_disk_quota_mb = 256;
int default_multidb_disk_quota_mb = 256;
/* Default behaviour is to unblock data collection if the page cache is full of dirty pages by dropping metrics */
uint8_t rrdeng_drop_metrics_under_page_cache_pressure = 1;

static inline struct rrdengine_instance *get_rrdeng_ctx_from_host(RRDHOST *host)
{
    return (struct rrdengine_instance*) host->rrdeng_ctx;
}

/* This UUID is not unique across hosts */
void rrdeng_generate_legacy_uuid(const char *dim_id, char *chart_id, uuid_t *ret_uuid)
{
    EVP_MD_CTX *evpctx;
    unsigned char hash_value[EVP_MAX_MD_SIZE];
    unsigned int hash_len;

    evpctx = EVP_MD_CTX_create();
    EVP_DigestInit_ex(evpctx, EVP_sha256(), NULL);
    EVP_DigestUpdate(evpctx, dim_id, strlen(dim_id));
    EVP_DigestUpdate(evpctx, chart_id, strlen(chart_id));
    EVP_DigestFinal_ex(evpctx, hash_value, &hash_len);
    EVP_MD_CTX_destroy(evpctx);
    fatal_assert(hash_len > sizeof(uuid_t));
    memcpy(ret_uuid, hash_value, sizeof(uuid_t));
}

/* Transform legacy UUID to be unique across hosts deterministically */
void rrdeng_convert_legacy_uuid_to_multihost(char machine_guid[GUID_LEN + 1], uuid_t *legacy_uuid, uuid_t *ret_uuid)
{
    EVP_MD_CTX *evpctx;
    unsigned char hash_value[EVP_MAX_MD_SIZE];
    unsigned int hash_len;

    evpctx = EVP_MD_CTX_create();
    EVP_DigestInit_ex(evpctx, EVP_sha256(), NULL);
    EVP_DigestUpdate(evpctx, machine_guid, GUID_LEN);
    EVP_DigestUpdate(evpctx, *legacy_uuid, sizeof(uuid_t));
    EVP_DigestFinal_ex(evpctx, hash_value, &hash_len);
    EVP_MD_CTX_destroy(evpctx);
    fatal_assert(hash_len > sizeof(uuid_t));
    memcpy(ret_uuid, hash_value, sizeof(uuid_t));
}

void rrdeng_metric_init(RRDDIM *rd)
{
    struct page_cache *pg_cache;
    struct rrdengine_instance *ctx;
    uuid_t legacy_uuid;
    uuid_t multihost_legacy_uuid;
    Pvoid_t *PValue;
    struct pg_cache_page_index *page_index = NULL;
    int is_multihost_child = 0;
    RRDHOST *host = rd->rrdset->rrdhost;

    ctx = get_rrdeng_ctx_from_host(rd->rrdset->rrdhost);
    if (unlikely(!ctx)) {
        error("Failed to fetch multidb context");
        return;
    }
    pg_cache = &ctx->pg_cache;

    rrdeng_generate_legacy_uuid(rd->id, rd->rrdset->id, &legacy_uuid);
    if (host != localhost && host->rrdeng_ctx->engine && host->rrdeng_ctx == host->rrdeng_ctx->engine->multidb_instance)
        is_multihost_child = 1;

    uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
    PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, &legacy_uuid, sizeof(uuid_t));
    if (likely(NULL != PValue)) {
        page_index = *PValue;
    }
    uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);
    if (is_multihost_child || NULL == PValue) {
        /* First time we see the legacy UUID or metric belongs to child host in multi-host DB.
         * Drop legacy support, normal path */

        uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
        PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, &rd->state->metric_uuid, sizeof(uuid_t));
        if (likely(NULL != PValue)) {
            page_index = *PValue;
        }
        uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);
        if (NULL == PValue) {
            uv_rwlock_wrlock(&pg_cache->metrics_index.lock);
            PValue = JudyHSIns(&pg_cache->metrics_index.JudyHS_array, &rd->state->metric_uuid, sizeof(uuid_t), PJE0);
            fatal_assert(NULL == *PValue); /* TODO: figure out concurrency model */
            *PValue = page_index = create_page_index(&rd->state->metric_uuid);
            page_index->prev = pg_cache->metrics_index.last_page_index;
            pg_cache->metrics_index.last_page_index = page_index;
            uv_rwlock_wrunlock(&pg_cache->metrics_index.lock);
        }
    } else {
        /* There are legacy UUIDs in the database, implement backward compatibility */

        rrdeng_convert_legacy_uuid_to_multihost(rd->rrdset->rrdhost->machine_guid, &legacy_uuid,
                                                &multihost_legacy_uuid);

        int need_to_store = uuid_compare(rd->state->metric_uuid, multihost_legacy_uuid);

        uuid_copy(rd->state->metric_uuid, multihost_legacy_uuid);

        if (unlikely(need_to_store))
            (void)sql_store_dimension(&rd->state->metric_uuid, rd->rrdset->chart_uuid, rd->id, rd->name, rd->multiplier, rd->divisor,
                rd->algorithm);

    }
    rd->state->rrdeng_uuid = &page_index->id;
    rd->state->page_index = page_index;
}

/*
 * Gets a handle for storing metrics to the database.
 * The handle must be released with rrdeng_store_metric_final().
 */
void rrdeng_store_metric_init(RRDDIM *rd)
{
    struct rrdeng_collect_handle *handle;
    struct rrdengine_instance *ctx;
    struct pg_cache_page_index *page_index;

    ctx = get_rrdeng_ctx_from_host(rd->rrdset->rrdhost);

    handle = callocz(1, sizeof(struct rrdeng_collect_handle));
    handle->ctx = ctx;
    handle->descr = NULL;
    handle->prev_descr = NULL;
    handle->unaligned_page = 0;
    rd->state->handle = (STORAGE_COLLECT_HANDLE *)handle;

    page_index = rd->state->page_index;
    uv_rwlock_wrlock(&page_index->lock);
    ++page_index->writers;
    uv_rwlock_wrunlock(&page_index->lock);
}

/* The page must be populated and referenced */
static int page_has_only_empty_metrics(struct rrdeng_page_descr *descr)
{
    unsigned i;
    uint8_t has_only_empty_metrics = 1;
    storage_number *page;

    page = descr->pg_cache_descr->page;
    for (i = 0 ; i < descr->page_length / sizeof(storage_number); ++i) {
        if (SN_EMPTY_SLOT != page[i]) {
            has_only_empty_metrics = 0;
            break;
        }
    }
    return has_only_empty_metrics;
}

void rrdeng_store_metric_flush_current_page(RRDDIM *rd)
{
    struct rrdeng_collect_handle *handle;
    struct rrdengine_instance *ctx;
    struct rrdeng_page_descr *descr;

    handle = (struct rrdeng_collect_handle *)rd->state->handle;
    ctx = handle->ctx;
    if (unlikely(!ctx))
        return;
    descr = handle->descr;
    if (unlikely(NULL == descr)) {
        return;
    }
    if (likely(descr->page_length)) {
        int page_is_empty;

        rrd_stat_atomic_add(&ctx->stats.metric_API_producers, -1);

        if (handle->prev_descr) {
            /* unpin old second page */
            pg_cache_put(ctx, handle->prev_descr);
        }
        page_is_empty = page_has_only_empty_metrics(descr);
        if (page_is_empty) {
            debug(D_RRDENGINE, "Page has empty metrics only, deleting:");
            if (unlikely(debug_flags & D_RRDENGINE))
                print_page_cache_descr(descr);
            pg_cache_put(ctx, descr);
            pg_cache_punch_hole(ctx, descr, 1, 0, NULL);
            handle->prev_descr = NULL;
        } else {
            /*
             * Disable pinning for now as it leads to deadlocks. When a collector stops collecting the extra pinned page
             * eventually gets rotated but it cannot be destroyed due to the extra reference.
             */
            /* added 1 extra reference to keep 2 dirty pages pinned per metric, expected refcnt = 2 */
/*          rrdeng_page_descr_mutex_lock(ctx, descr);
            ret = pg_cache_try_get_unsafe(descr, 0);
            rrdeng_page_descr_mutex_unlock(ctx, descr);
            fatal_assert(1 == ret);*/

            rrdeng_commit_page(ctx, descr, handle->page_correlation_id);
            /* handle->prev_descr = descr;*/
        }
    } else {
        freez(descr->pg_cache_descr->page);
        rrdeng_destroy_pg_cache_descr(ctx, descr->pg_cache_descr);
        freez(descr);
    }
    handle->descr = NULL;
}

void rrdeng_store_metric_next(RRDDIM *rd, usec_t point_in_time, storage_number number)
{
    struct rrdeng_collect_handle *handle = (struct rrdeng_collect_handle *)rd->state->handle;
    struct rrdengine_instance *ctx = handle->ctx;
    struct page_cache *pg_cache = &ctx->pg_cache;
    struct rrdeng_page_descr *descr = handle->descr;
    storage_number *page;
    uint8_t must_flush_unaligned_page = 0, perfect_page_alignment = 0;

    if (descr) {
        /* Make alignment decisions */

        if (descr->page_length == rd->rrdset->rrddim_page_alignment) {
            /* this is the leading dimension that defines chart alignment */
            perfect_page_alignment = 1;
        }
        /* is the metric far enough out of alignment with the others? */
        if (unlikely(descr->page_length + sizeof(number) < rd->rrdset->rrddim_page_alignment)) {
            handle->unaligned_page = 1;
            debug(D_RRDENGINE, "Metric page is not aligned with chart:");
            if (unlikely(debug_flags & D_RRDENGINE))
                print_page_cache_descr(descr);
        }
        if (unlikely(handle->unaligned_page &&
                     /* did the other metrics change page? */
                     rd->rrdset->rrddim_page_alignment <= sizeof(number))) {
            debug(D_RRDENGINE, "Flushing unaligned metric page.");
            must_flush_unaligned_page = 1;
            handle->unaligned_page = 0;
        }
    }
    if (unlikely(NULL == descr ||
                 descr->page_length + sizeof(number) > RRDENG_BLOCK_SIZE ||
                 must_flush_unaligned_page)) {
        rrdeng_store_metric_flush_current_page(rd);

        page = rrdeng_create_page(ctx, &rd->state->page_index->id, &descr);
        fatal_assert(page);

        handle->descr = descr;

        handle->page_correlation_id = rrd_atomic_fetch_add(&pg_cache->committed_page_index.latest_corr_id, 1);

        if (0 == rd->rrdset->rrddim_page_alignment) {
            /* this is the leading dimension that defines chart alignment */
            perfect_page_alignment = 1;
        }
    }
    page = descr->pg_cache_descr->page;
    page[descr->page_length / sizeof(number)] = number;
    pg_cache_atomic_set_pg_info(descr, point_in_time, descr->page_length + sizeof(number));

    if (perfect_page_alignment)
        rd->rrdset->rrddim_page_alignment = descr->page_length;
    if (unlikely(INVALID_TIME == descr->start_time)) {
        unsigned long new_metric_API_producers, old_metric_API_max_producers, ret_metric_API_max_producers;
        descr->start_time = point_in_time;

        new_metric_API_producers = rrd_atomic_add_fetch(&ctx->stats.metric_API_producers, 1);
        while (unlikely(new_metric_API_producers > (old_metric_API_max_producers = ctx->metric_API_max_producers))) {
            /* Increase ctx->metric_API_max_producers */
            ret_metric_API_max_producers = ulong_compare_and_swap(&ctx->metric_API_max_producers,
                                                                  old_metric_API_max_producers,
                                                                  new_metric_API_producers);
            if (old_metric_API_max_producers == ret_metric_API_max_producers) {
                /* success */
                break;
            }
        }

        pg_cache_insert(ctx, rd->state->page_index, descr);
    } else {
        pg_cache_add_new_metric_time(rd->state->page_index, descr);
    }
}

/*
 * Releases the database reference from the handle for storing metrics.
 * Returns 1 if it's safe to delete the dimension.
 */
int rrdeng_store_metric_finalize(RRDDIM *rd)
{
    struct rrdeng_collect_handle *handle;
    struct rrdengine_instance *ctx;
    struct pg_cache_page_index *page_index;
    uint8_t can_delete_metric = 0;

    handle = (struct rrdeng_collect_handle *)rd->state->handle;
    ctx = handle->ctx;
    page_index = rd->state->page_index;
    rrdeng_store_metric_flush_current_page(rd);
    if (handle->prev_descr) {
        /* unpin old second page */
        pg_cache_put(ctx, handle->prev_descr);
    }
    uv_rwlock_wrlock(&page_index->lock);
    if (!--page_index->writers && !page_index->page_count) {
        can_delete_metric = 1;
    }
    uv_rwlock_wrunlock(&page_index->lock);
    freez(handle);

   return can_delete_metric;
}

/* Returns 1 if the data collection interval is well defined, 0 otherwise */
static int metrics_with_known_interval(struct rrdeng_page_descr *descr)
{
    unsigned page_entries;

    if (unlikely(INVALID_TIME == descr->start_time || INVALID_TIME == descr->end_time))
        return 0;
    page_entries = descr->page_length / sizeof(storage_number);
    if (likely(page_entries > 1)) {
        return 1;
    }
    return 0;
}

static inline uint32_t *pginfo_to_dt(struct rrdeng_page_info *page_info)
{
    return (uint32_t *)&page_info->scratch[0];
}

static inline uint32_t *pginfo_to_points(struct rrdeng_page_info *page_info)
{
    return (uint32_t *)&page_info->scratch[sizeof(uint32_t)];
}

/**
 * Calculates the regions of different data collection intervals in a netdata chart in the time range
 * [start_time,end_time]. This call takes the netdata chart read lock.
 * @param st the netdata chart whose data collection interval boundaries are calculated.
 * @param start_time inclusive starting time in usec
 * @param end_time inclusive ending time in usec
 * @param region_info_arrayp It allocates (*region_info_arrayp) and populates it with information of regions of a
 *         reference dimension that that have different data collection intervals and overlap with the time range
 *         [start_time,end_time]. The caller must free (*region_info_arrayp) with freez(). If region_info_arrayp is set
 *         to NULL nothing was allocated.
 * @param max_intervalp is dereferenced and set to be the largest data collection interval of all regions.
 * @return number of regions with different data collection intervals.
 */
unsigned rrdeng_variable_step_boundaries(RRDSET *st, time_t start_time, time_t end_time,
                                         struct rrdeng_region_info **region_info_arrayp, unsigned *max_intervalp, struct context_param *context_param_list)
{
    struct pg_cache_page_index *page_index;
    struct rrdengine_instance *ctx;
    unsigned pages_nr;
    RRDDIM *rd_iter, *rd;
    struct rrdeng_page_info *page_info_array, *curr, *prev, *old_prev;
    unsigned i, j, page_entries, region_points, page_points, regions, max_interval;
    time_t now;
    usec_t dt, current_position_time, max_time = 0, min_time, curr_time, first_valid_time_in_page;
    struct rrdeng_region_info *region_info_array;
    uint8_t is_first_region_initialized;

    ctx = get_rrdeng_ctx_from_host(st->rrdhost);
    regions = 1;
    *max_intervalp = max_interval = 0;
    region_info_array = NULL;
    *region_info_arrayp = NULL;
    page_info_array = NULL;

    RRDDIM *temp_rd = context_param_list ? context_param_list->rd : NULL;
    rrdset_rdlock(st);
    for(rd_iter = temp_rd?temp_rd:st->dimensions, rd = NULL, min_time = (usec_t)-1 ; rd_iter ; rd_iter = rd_iter->next) {
        /*
         * Choose oldest dimension as reference. This is not equivalent to the union of all dimensions
         * but it is a best effort approximation with a bias towards older metrics in a chart. It
         * matches netdata behaviour in the sense that dimensions are generally aligned in a chart
         * and older dimensions contain more information about the time range. It does not work well
         * for metrics that have recently stopped being collected.
         */
        curr_time = pg_cache_oldest_time_in_range(ctx, rd_iter->state->rrdeng_uuid,
                                                  start_time * USEC_PER_SEC, end_time * USEC_PER_SEC);
        if (INVALID_TIME != curr_time && curr_time < min_time) {
            rd = rd_iter;
            min_time = curr_time;
        }
    }
    rrdset_unlock(st);
    if (NULL == rd) {
        return 1;
    }
    pages_nr = pg_cache_preload(ctx, rd->state->rrdeng_uuid, start_time * USEC_PER_SEC, end_time * USEC_PER_SEC,
                                &page_info_array, &page_index);
    if (pages_nr) {
        /* conservative allocation, will reduce the size later if necessary */
        region_info_array = mallocz(sizeof(*region_info_array) * pages_nr);
    }
    is_first_region_initialized = 0;
    region_points = 0;

    /* pages loop */
    for (i = 0, curr = NULL, prev = NULL ; i < pages_nr ; ++i) {
        old_prev = prev;
        prev = curr;
        curr = &page_info_array[i];
        *pginfo_to_points(curr) = 0; /* initialize to invalid page */
        *pginfo_to_dt(curr) = 0; /* no known data collection interval yet */
        if (unlikely(INVALID_TIME == curr->start_time || INVALID_TIME == curr->end_time ||
                     curr->end_time < curr->start_time)) {
            info("Ignoring page with invalid timestamps.");
            prev = old_prev;
            continue;
        }
        page_entries = curr->page_length / sizeof(storage_number);
        fatal_assert(0 != page_entries);
        if (likely(1 != page_entries)) {
            dt = (curr->end_time - curr->start_time) / (page_entries - 1);
            *pginfo_to_dt(curr) = ROUND_USEC_TO_SEC(dt);
            if (unlikely(0 == *pginfo_to_dt(curr)))
                *pginfo_to_dt(curr) = 1;
        } else {
            dt = 0;
        }
        for (j = 0, page_points = 0 ; j < page_entries ; ++j) {
            uint8_t is_metric_out_of_order, is_metric_earlier_than_range;

            is_metric_earlier_than_range = 0;
            is_metric_out_of_order = 0;

            current_position_time = curr->start_time + j * dt;
            now = current_position_time / USEC_PER_SEC;
            if (now > end_time) { /* there will be no more pages in the time range */
                break;
            }
            if (now < start_time)
                is_metric_earlier_than_range = 1;
            if (unlikely(current_position_time < max_time)) /* just went back in time */
                is_metric_out_of_order = 1;
            if (is_metric_earlier_than_range || unlikely(is_metric_out_of_order)) {
                if (unlikely(is_metric_out_of_order))
                    info("Ignoring metric with out of order timestamp.");
                continue; /* next entry */
            }
            /* here is a valid metric */
            ++page_points;
            region_info_array[regions - 1].points = ++region_points;
            max_time = current_position_time;
            if (1 == page_points)
                first_valid_time_in_page = current_position_time;
            if (unlikely(!is_first_region_initialized)) {
                fatal_assert(1 == regions);
                /* this is the first region */
                region_info_array[0].start_time = current_position_time;
                is_first_region_initialized = 1;
            }
        }
        *pginfo_to_points(curr) = page_points;
        if (0 == page_points) {
            prev = old_prev;
            continue;
        }

        if (unlikely(0 == *pginfo_to_dt(curr))) { /* unknown data collection interval */
            fatal_assert(1 == page_points);

            if (likely(NULL != prev)) { /* get interval from previous page */
                *pginfo_to_dt(curr) = *pginfo_to_dt(prev);
            } else { /* there is no previous page in the query */
                struct rrdeng_page_info db_page_info;

                /* go to database */
                pg_cache_get_filtered_info_prev(ctx, page_index, curr->start_time,
                                                metrics_with_known_interval, &db_page_info);
                if (unlikely(db_page_info.start_time == INVALID_TIME || db_page_info.end_time == INVALID_TIME ||
                             0 == db_page_info.page_length)) { /* nothing in the database, default to update_every */
                    *pginfo_to_dt(curr) = rd->update_every;
                } else {
                    unsigned db_entries;
                    usec_t db_dt;

                    db_entries = db_page_info.page_length / sizeof(storage_number);
                    db_dt = (db_page_info.end_time - db_page_info.start_time) / (db_entries - 1);
                    *pginfo_to_dt(curr) = ROUND_USEC_TO_SEC(db_dt);
                    if (unlikely(0 == *pginfo_to_dt(curr)))
                        *pginfo_to_dt(curr) = 1;

                }
            }
        }
        if (likely(prev) && unlikely(*pginfo_to_dt(curr) != *pginfo_to_dt(prev))) {
            info("Data collection interval change detected in query: %"PRIu32" -> %"PRIu32,
                 *pginfo_to_dt(prev), *pginfo_to_dt(curr));
            region_info_array[regions++ - 1].points -= page_points;
            region_info_array[regions - 1].points = region_points = page_points;
            region_info_array[regions - 1].start_time = first_valid_time_in_page;
        }
        if (*pginfo_to_dt(curr) > max_interval)
            max_interval = *pginfo_to_dt(curr);
        region_info_array[regions - 1].update_every = *pginfo_to_dt(curr);
    }
    if (page_info_array)
        freez(page_info_array);
    if (region_info_array) {
        if (likely(is_first_region_initialized)) {
            /* free unnecessary memory */
            region_info_array = reallocz(region_info_array, sizeof(*region_info_array) * regions);
            *region_info_arrayp = region_info_array;
            *max_intervalp = max_interval;
        } else {
            /* empty result */
            freez(region_info_array);
        }
    }
    return regions;
}

/*
 * Gets a handle for loading metrics from the database.
 * The handle must be released with rrdeng_load_metric_final().
 */
void rrdeng_load_metric_init(RRDDIM *rd, struct rrddim_query_handle *rrdimm_handle, time_t start_time, time_t end_time)
{
    struct rrdeng_query_handle *handle;
    struct rrdengine_instance *ctx;
    unsigned pages_nr;

    ctx = get_rrdeng_ctx_from_host(rd->rrdset->rrdhost);
    rrdimm_handle->start_time = start_time;
    rrdimm_handle->end_time = end_time;

    handle = calloc(1, sizeof(struct rrdeng_query_handle));
    handle->next_page_time = start_time;
    handle->now = start_time;
    handle->position = 0;
    handle->ctx = ctx;
    handle->descr = NULL;
    rrdimm_handle->handle = (STORAGE_QUERY_HANDLE *)handle;
    pages_nr = pg_cache_preload(ctx, rd->state->rrdeng_uuid, start_time * USEC_PER_SEC, end_time * USEC_PER_SEC,
                                NULL, &handle->page_index);
    if (unlikely(NULL == handle->page_index || 0 == pages_nr))
        /* there are no metrics to load */
        handle->next_page_time = INVALID_TIME;
}

/* Returns the metric and sets its timestamp into current_time */
storage_number rrdeng_load_metric_next(struct rrddim_query_handle *rrdimm_handle, time_t *current_time)
{
    struct rrdeng_query_handle *handle = (struct rrdeng_query_handle *)rrdimm_handle->handle;
    struct rrdengine_instance *ctx;
    struct rrdeng_page_descr *descr;
    storage_number *page, ret;
    unsigned position, entries;
    usec_t next_page_time = 0, current_position_time, page_end_time = 0;
    uint32_t page_length;

    if (unlikely(INVALID_TIME == handle->next_page_time)) {
        return SN_EMPTY_SLOT;
    }
    ctx = handle->ctx;
    if (unlikely(NULL == (descr = handle->descr))) {
        /* it's the first call */
        next_page_time = handle->next_page_time * USEC_PER_SEC;
    } else {
        pg_cache_atomic_get_pg_info(descr, &page_end_time, &page_length);
    }
    position = handle->position + 1;

    if (unlikely(NULL == descr ||
                 position >= (page_length / sizeof(storage_number)))) {
        /* We need to get a new page */
        if (descr) {
            /* Drop old page's reference */
#ifdef NETDATA_INTERNAL_CHECKS
            rrd_stat_atomic_add(&ctx->stats.metric_API_consumers, -1);
#endif
            pg_cache_put(ctx, descr);
            handle->descr = NULL;
            handle->next_page_time = (page_end_time / USEC_PER_SEC) + 1;
            if (unlikely(handle->next_page_time > rrdimm_handle->end_time)) {
                goto no_more_metrics;
            }
            next_page_time = handle->next_page_time * USEC_PER_SEC;
        }

        descr = pg_cache_lookup_next(ctx, handle->page_index, &handle->page_index->id,
                                     next_page_time, rrdimm_handle->end_time * USEC_PER_SEC);
        if (NULL == descr) {
            goto no_more_metrics;
        }
#ifdef NETDATA_INTERNAL_CHECKS
        rrd_stat_atomic_add(&ctx->stats.metric_API_consumers, 1);
#endif
        handle->descr = descr;
        pg_cache_atomic_get_pg_info(descr, &page_end_time, &page_length);
        if (unlikely(INVALID_TIME == descr->start_time ||
                     INVALID_TIME == page_end_time)) {
            goto no_more_metrics;
        }
        if (unlikely(descr->start_time != page_end_time && next_page_time > descr->start_time)) {
            /* we're in the middle of the page somewhere */
            entries = page_length / sizeof(storage_number);
            position = ((uint64_t)(next_page_time - descr->start_time)) * (entries - 1) /
                       (page_end_time - descr->start_time);
        } else {
            position = 0;
        }
    }
    page = descr->pg_cache_descr->page;
    ret = page[position];
    entries = page_length / sizeof(storage_number);
    if (entries > 1) {
        usec_t dt;

        dt = (page_end_time - descr->start_time) / (entries - 1);
        current_position_time = descr->start_time + position * dt;
    } else {
        current_position_time = descr->start_time;
    }
    handle->position = position;
    handle->now = current_position_time / USEC_PER_SEC;
/*  fatal_assert(handle->now >= rrdimm_handle->start_time && handle->now <= rrdimm_handle->end_time);
    The above assertion is an approximation and needs to take update_every into account */
    if (unlikely(handle->now >= rrdimm_handle->end_time)) {
        /* next calls will not load any more metrics */
        handle->next_page_time = INVALID_TIME;
    }
    *current_time = handle->now;
    return ret;

no_more_metrics:
    handle->next_page_time = INVALID_TIME;
    return SN_EMPTY_SLOT;
}

int rrdeng_load_metric_is_finished(struct rrddim_query_handle *rrdimm_handle)
{
    struct rrdeng_query_handle *handle = (struct rrdeng_query_handle *)rrdimm_handle->handle;
    return (INVALID_TIME == handle->next_page_time);
}

/*
 * Releases the database reference from the handle for loading metrics.
 */
void rrdeng_load_metric_finalize(struct rrddim_query_handle *rrdimm_handle)
{
    struct rrdeng_query_handle *handle = (struct rrdeng_query_handle *)rrdimm_handle->handle;
    struct rrdengine_instance *ctx = handle->ctx;
    struct rrdeng_page_descr *descr = handle->descr;

    if (descr) {
#ifdef NETDATA_INTERNAL_CHECKS
        rrd_stat_atomic_add(&ctx->stats.metric_API_consumers, -1);
#endif
        pg_cache_put(ctx, descr);
    }
}

time_t rrdeng_metric_latest_time(RRDDIM *rd)
{
    struct pg_cache_page_index *page_index;

    page_index = rd->state->page_index;

    return page_index->latest_time / USEC_PER_SEC;
}
time_t rrdeng_metric_oldest_time(RRDDIM *rd)
{
    struct pg_cache_page_index *page_index;

    page_index = rd->state->page_index;

    return page_index->oldest_time / USEC_PER_SEC;
}

int rrdeng_metric_latest_time_by_uuid(uuid_t *dim_uuid, time_t *first_entry_t, time_t *last_entry_t)
{
    struct page_cache *pg_cache;
    struct rrdengine_instance *ctx;
    Pvoid_t *PValue;
    struct pg_cache_page_index *page_index = NULL;

    ctx = get_rrdeng_ctx_from_host(localhost);
    if (unlikely(!ctx)) {
        error("Failed to fetch multidb context");
        return 1;
    }
    pg_cache = &ctx->pg_cache;

    uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
    PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, dim_uuid, sizeof(uuid_t));
    if (likely(NULL != PValue)) {
        page_index = *PValue;
    }
    uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);

    if (likely(page_index)) {
        *first_entry_t = page_index->oldest_time / USEC_PER_SEC;
        *last_entry_t = page_index->latest_time / USEC_PER_SEC;
        return 0;
    }

    return 1;
}

/* Also gets a reference for the page */
void *rrdeng_create_page(struct rrdengine_instance *ctx, uuid_t *id, struct rrdeng_page_descr **ret_descr)
{
    struct rrdeng_page_descr *descr;
    struct page_cache_descr *pg_cache_descr;
    void *page;
    /* TODO: check maximum number of pages in page cache limit */

    descr = pg_cache_create_descr();
    descr->id = id; /* TODO: add page type: metric, log, something? */
    page = mallocz(RRDENG_BLOCK_SIZE); /*TODO: add page size */
    rrdeng_page_descr_mutex_lock(ctx, descr);
    pg_cache_descr = descr->pg_cache_descr;
    pg_cache_descr->page = page;
    pg_cache_descr->flags = RRD_PAGE_DIRTY /*| RRD_PAGE_LOCKED */ | RRD_PAGE_POPULATED /* | BEING_COLLECTED */;
    pg_cache_descr->refcnt = 1;

    debug(D_RRDENGINE, "Created new page:");
    if (unlikely(debug_flags & D_RRDENGINE))
        print_page_cache_descr(descr);
    rrdeng_page_descr_mutex_unlock(ctx, descr);
    *ret_descr = descr;
    return page;
}

/* The page must not be empty */
void rrdeng_commit_page(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr,
                        Word_t page_correlation_id)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    Pvoid_t *PValue;
    unsigned nr_committed_pages;

    if (unlikely(NULL == descr)) {
        debug(D_RRDENGINE, "%s: page descriptor is NULL, page has already been force-committed.", __func__);
        return;
    }
    fatal_assert(descr->page_length);

    uv_rwlock_wrlock(&pg_cache->committed_page_index.lock);
    PValue = JudyLIns(&pg_cache->committed_page_index.JudyL_array, page_correlation_id, PJE0);
    *PValue = descr;
    nr_committed_pages = ++pg_cache->committed_page_index.nr_committed_pages;
    uv_rwlock_wrunlock(&pg_cache->committed_page_index.lock);

    if (nr_committed_pages >= pg_cache_hard_limit(ctx) / 2) {
        /* over 50% of pages have not been committed yet */

        if (ctx->drop_metrics_under_page_cache_pressure &&
            nr_committed_pages >= pg_cache_committed_hard_limit(ctx)) {
            /* 100% of pages are dirty */
            struct rrdeng_cmd cmd;

            cmd.opcode = RRDENG_INVALIDATE_OLDEST_MEMORY_PAGE;
            rrdeng_enq_cmd(&ctx->worker_config, &cmd);
        } else {
            if (0 == (unsigned long) ctx->stats.pg_cache_over_half_dirty_events) {
                /* only print the first time */
                errno = 0;
                error("Failed to flush dirty buffers quickly enough in dbengine instance \"%s\". "
                      "Metric data at risk of not being stored in the database, "
                      "please reduce disk load or use a faster disk.", ctx->dbfiles_path);
            }
            rrd_stat_atomic_add(&ctx->stats.pg_cache_over_half_dirty_events, 1);
            rrd_stat_atomic_add(&global_pg_cache_over_half_dirty_events, 1);
        }
    }

    pg_cache_put(ctx, descr);
}

/* Gets a reference for the page */
void *rrdeng_get_latest_page(struct rrdengine_instance *ctx, uuid_t *id, void **handle)
{
    struct rrdeng_page_descr *descr;
    struct page_cache_descr *pg_cache_descr;

    debug(D_RRDENGINE, "Reading existing page:");
    descr = pg_cache_lookup(ctx, NULL, id, INVALID_TIME);
    if (NULL == descr) {
        *handle = NULL;

        return NULL;
    }
    *handle = descr;
    pg_cache_descr = descr->pg_cache_descr;

    return pg_cache_descr->page;
}

/* Gets a reference for the page */
void *rrdeng_get_page(struct rrdengine_instance *ctx, uuid_t *id, usec_t point_in_time, void **handle)
{
    struct rrdeng_page_descr *descr;
    struct page_cache_descr *pg_cache_descr;

    debug(D_RRDENGINE, "Reading existing page:");
    descr = pg_cache_lookup(ctx, NULL, id, point_in_time);
    if (NULL == descr) {
        *handle = NULL;

        return NULL;
    }
    *handle = descr;
    pg_cache_descr = descr->pg_cache_descr;

    return pg_cache_descr->page;
}

/*
 * Gathers Database Engine statistics.
 * Careful when modifying this function.
 * You must not change the indices of the statistics or user code will break.
 * You must not exceed RRDENG_NR_STATS or it will crash.
 */
void rrdeng_get_37_statistics(struct rrdengine_instance *ctx, unsigned long long *array)
{
    if (ctx == NULL)
        return;

    struct page_cache *pg_cache = &ctx->pg_cache;

    array[0] = (uint64_t)ctx->stats.metric_API_producers;
    array[1] = (uint64_t)ctx->stats.metric_API_consumers;
    array[2] = (uint64_t)pg_cache->page_descriptors;
    array[3] = (uint64_t)pg_cache->populated_pages;
    array[4] = (uint64_t)pg_cache->committed_page_index.nr_committed_pages;
    array[5] = (uint64_t)ctx->stats.pg_cache_insertions;
    array[6] = (uint64_t)ctx->stats.pg_cache_deletions;
    array[7] = (uint64_t)ctx->stats.pg_cache_hits;
    array[8] = (uint64_t)ctx->stats.pg_cache_misses;
    array[9] = (uint64_t)ctx->stats.pg_cache_backfills;
    array[10] = (uint64_t)ctx->stats.pg_cache_evictions;
    array[11] = (uint64_t)ctx->stats.before_compress_bytes;
    array[12] = (uint64_t)ctx->stats.after_compress_bytes;
    array[13] = (uint64_t)ctx->stats.before_decompress_bytes;
    array[14] = (uint64_t)ctx->stats.after_decompress_bytes;
    array[15] = (uint64_t)ctx->stats.io_write_bytes;
    array[16] = (uint64_t)ctx->stats.io_write_requests;
    array[17] = (uint64_t)ctx->stats.io_read_bytes;
    array[18] = (uint64_t)ctx->stats.io_read_requests;
    array[19] = (uint64_t)ctx->stats.io_write_extent_bytes;
    array[20] = (uint64_t)ctx->stats.io_write_extents;
    array[21] = (uint64_t)ctx->stats.io_read_extent_bytes;
    array[22] = (uint64_t)ctx->stats.io_read_extents;
    array[23] = (uint64_t)ctx->stats.datafile_creations;
    array[24] = (uint64_t)ctx->stats.datafile_deletions;
    array[25] = (uint64_t)ctx->stats.journalfile_creations;
    array[26] = (uint64_t)ctx->stats.journalfile_deletions;
    array[27] = (uint64_t)ctx->stats.page_cache_descriptors;
    array[28] = (uint64_t)ctx->stats.io_errors;
    array[29] = (uint64_t)ctx->stats.fs_errors;
    array[30] = (uint64_t)global_io_errors;
    array[31] = (uint64_t)global_fs_errors;
    array[32] = (uint64_t)rrdeng_reserved_file_descriptors;
    array[33] = (uint64_t)ctx->stats.pg_cache_over_half_dirty_events;
    array[34] = (uint64_t)global_pg_cache_over_half_dirty_events;
    array[35] = (uint64_t)ctx->stats.flushing_pressure_page_deletions;
    array[36] = (uint64_t)global_flushing_pressure_page_deletions;
    fatal_assert(RRDENG_NR_STATS == 37);
}

/* Releases reference to page */
void rrdeng_put_page(struct rrdengine_instance *ctx, void *handle)
{
    (void)ctx;
    pg_cache_put(ctx, (struct rrdeng_page_descr *)handle);
}


STORAGE_ENGINE_INSTANCE*
rrdeng_init(STORAGE_ENGINE* eng, RRDHOST *host)
{
    struct rrdengine_instance *ctx;
    int error;
    uint32_t max_open_files;

    max_open_files = rlimit_nofile.rlim_cur / 4;

    /* reserve RRDENG_FD_BUDGET_PER_INSTANCE file descriptors for this instance */
    rrd_stat_atomic_add(&rrdeng_reserved_file_descriptors, RRDENG_FD_BUDGET_PER_INSTANCE);
    if (rrdeng_reserved_file_descriptors > max_open_files) {
        error(
            "Exceeded the budget of available file descriptors (%u/%u), cannot create new dbengine instance.",
            (unsigned)rrdeng_reserved_file_descriptors, (unsigned)max_open_files);

        rrd_stat_atomic_add(&global_fs_errors, 1);
        rrd_stat_atomic_add(&rrdeng_reserved_file_descriptors, -RRDENG_FD_BUDGET_PER_INSTANCE);
        return NULL;//UV_EMFILE;
    }
    char dbfiles_path[FILENAME_MAX + 1];
    int ret;

    snprintfz(dbfiles_path, FILENAME_MAX, "%s/dbengine", host->cache_dir);
    ret = mkdir(dbfiles_path, 0775);

    int page_cache_mb = default_rrdeng_page_cache_mb;
    int disk_space_mb = default_rrdeng_disk_quota_mb;

    ctx = callocz(1, sizeof(*ctx));
    ctx->parent.engine = eng;
    ctx->global_compress_alg = RRD_LZ4;
    if (page_cache_mb < RRDENG_MIN_PAGE_CACHE_SIZE_MB)
        page_cache_mb = RRDENG_MIN_PAGE_CACHE_SIZE_MB;
    ctx->max_cache_pages = page_cache_mb * (1048576LU / RRDENG_BLOCK_SIZE);
    /* try to keep 5% of the page cache free */
    ctx->cache_pages_low_watermark = (ctx->max_cache_pages * 95LLU) / 100;
    if (disk_space_mb < RRDENG_MIN_DISK_SPACE_MB)
        disk_space_mb = RRDENG_MIN_DISK_SPACE_MB;
    ctx->max_disk_space = disk_space_mb * 1048576LLU;
    strncpyz(ctx->dbfiles_path, dbfiles_path, sizeof(ctx->dbfiles_path) - 1);
    ctx->dbfiles_path[sizeof(ctx->dbfiles_path) - 1] = '\0';
    if (NULL == host)
        strncpyz(ctx->machine_guid, registry_get_this_machine_guid(), GUID_LEN);
    else
        strncpyz(ctx->machine_guid, host->machine_guid, GUID_LEN);

    ctx->drop_metrics_under_page_cache_pressure = rrdeng_drop_metrics_under_page_cache_pressure;
    ctx->metric_API_max_producers = 0;
    ctx->quiesce = NO_QUIESCE;
    ctx->metalog_ctx = NULL; /* only set this after the metadata log has finished initializing */
    ctx->host = host;

    memset(&ctx->worker_config, 0, sizeof(ctx->worker_config));
    ctx->worker_config.ctx = ctx;
    init_page_cache(ctx);
    init_commit_log(ctx);
    error = init_rrd_files(ctx);
    if (error) {
        goto error_after_init_rrd_files;
    }

    completion_init(&ctx->rrdengine_completion);
    fatal_assert(0 == uv_thread_create(&ctx->worker_config.thread, rrdeng_worker, &ctx->worker_config));
    /* wait for worker thread to initialize */
    completion_wait_for(&ctx->rrdengine_completion);
    completion_destroy(&ctx->rrdengine_completion);
    uv_thread_set_name_np(ctx->worker_config.thread, "DBENGINE");
    if (ctx->worker_config.error) {
        goto error_after_rrdeng_worker;
    }
    error = metalog_init(ctx);
    if (error) {
        error("Failed to initialize metadata log file event loop.");
        goto error_after_rrdeng_worker;
    }

    return (STORAGE_ENGINE_INSTANCE *)ctx;

error_after_rrdeng_worker:
    finalize_rrd_files(ctx);
error_after_init_rrd_files:
    free_page_cache(ctx);
    freez(ctx);
    rrd_stat_atomic_add(&rrdeng_reserved_file_descriptors, -RRDENG_FD_BUDGET_PER_INSTANCE);
    return NULL;//UV_EIO;
}

/*
 * Returns 0 on success, 1 on error
 */
int rrdeng_exit(struct rrdengine_instance *ctx)
{
    struct rrdeng_cmd cmd;

    if (NULL == ctx) {
        return 1;
    }

    /* TODO: add page to page cache */
    cmd.opcode = RRDENG_SHUTDOWN;
    rrdeng_enq_cmd(&ctx->worker_config, &cmd);

    fatal_assert(0 == uv_thread_join(&ctx->worker_config.thread));

    finalize_rrd_files(ctx);
    //metalog_exit(ctx->metalog_ctx);
    free_page_cache(ctx);
    freez(ctx);
    rrd_stat_atomic_add(&rrdeng_reserved_file_descriptors, -RRDENG_FD_BUDGET_PER_INSTANCE);
    return 0;
}

void rrdeng_prepare_exit(struct rrdengine_instance *ctx)
{
    struct rrdeng_cmd cmd;

    if (NULL == ctx) {
        return;
    }

    completion_init(&ctx->rrdengine_completion);
    cmd.opcode = RRDENG_QUIESCE;
    rrdeng_enq_cmd(&ctx->worker_config, &cmd);

    /* wait for dbengine to quiesce */
    completion_wait_for(&ctx->rrdengine_completion);
    completion_destroy(&ctx->rrdengine_completion);

    //metalog_prepare_exit(ctx->metalog_ctx);
}

