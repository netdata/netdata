// SPDX-License-Identifier: GPL-3.0-or-later
#include "rrdengine.h"

/* Default global database instance */
static struct rrdengine_instance default_global_ctx;

int default_rrdeng_page_cache_mb = RRDENG_MIN_PAGE_CACHE_SIZE_MB;
int default_rrdeng_disk_quota_mb = RRDENG_MIN_DISK_SPACE_MB;

/*
 * Gets a handle for storing metrics to the database.
 * The handle must be released with rrdeng_store_metric_final().
 */
void rrdeng_store_metric_init(RRDDIM *rd)
{
    struct rrdeng_collect_handle *handle;
    struct page_cache *pg_cache;
    struct rrdengine_instance *ctx;
    uuid_t temp_id;
    Pvoid_t *PValue;
    struct pg_cache_page_index *page_index;
    EVP_MD_CTX *evpctx;
    unsigned char hash_value[EVP_MAX_MD_SIZE];
    unsigned int hash_len;

    //&default_global_ctx; TODO: test this use case or remove it?

    ctx = rd->rrdset->rrdhost->rrdeng_ctx;
    pg_cache = &ctx->pg_cache;
    handle = &rd->state->handle.rrdeng;
    handle->ctx = ctx;

    evpctx = EVP_MD_CTX_create();
    EVP_DigestInit_ex(evpctx, EVP_sha256(), NULL);
    EVP_DigestUpdate(evpctx, rd->id, strlen(rd->id));
    EVP_DigestUpdate(evpctx, rd->rrdset->id, strlen(rd->rrdset->id));
    EVP_DigestFinal_ex(evpctx, hash_value, &hash_len);
    EVP_MD_CTX_destroy(evpctx);
    assert(hash_len > sizeof(temp_id));
    memcpy(&temp_id, hash_value, sizeof(temp_id));

    handle->descr = NULL;
    handle->prev_descr = NULL;

    uv_rwlock_rdlock(&pg_cache->metrics_index.lock);
    PValue = JudyHSGet(pg_cache->metrics_index.JudyHS_array, &temp_id, sizeof(uuid_t));
    if (likely(NULL != PValue)) {
        page_index = *PValue;
    }
    uv_rwlock_rdunlock(&pg_cache->metrics_index.lock);
    if (NULL == PValue) {
        /* First time we see the UUID */
        uv_rwlock_wrlock(&pg_cache->metrics_index.lock);
        PValue = JudyHSIns(&pg_cache->metrics_index.JudyHS_array, &temp_id, sizeof(uuid_t), PJE0);
        assert(NULL == *PValue); /* TODO: figure out concurrency model */
        *PValue = page_index = create_page_index(&temp_id);
        uv_rwlock_wrunlock(&pg_cache->metrics_index.lock);
    }
    rd->state->rrdeng_uuid = &page_index->id;
    handle->page_index = page_index;
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

    handle = &rd->state->handle.rrdeng;
    ctx = handle->ctx;
    descr = handle->descr;
    if (unlikely(NULL == descr)) {
        return;
    }
    if (likely(descr->page_length)) {
        int ret, page_is_empty;

#ifdef NETDATA_INTERNAL_CHECKS
        rrd_stat_atomic_add(&ctx->stats.metric_API_producers, -1);
#endif
        if (handle->prev_descr) {
            /* unpin old second page */
            pg_cache_put(ctx, handle->prev_descr);
        }
        page_is_empty = page_has_only_empty_metrics(descr);
        if (page_is_empty) {
            debug(D_RRDENGINE, "Page has empty metrics only, deleting:");
            if(unlikely(debug_flags & D_RRDENGINE))
                print_page_cache_descr(descr);
            pg_cache_put(ctx, descr);
            pg_cache_punch_hole(ctx, descr, 1);
            handle->prev_descr = NULL;
        } else {
            /* added 1 extra reference to keep 2 dirty pages pinned per metric, expected refcnt = 2 */
            rrdeng_page_descr_mutex_lock(ctx, descr);
            ret = pg_cache_try_get_unsafe(descr, 0);
            rrdeng_page_descr_mutex_unlock(ctx, descr);
            assert (1 == ret);

            rrdeng_commit_page(ctx, descr, handle->page_correlation_id);
        }
        handle->prev_descr = descr;
    } else {
        free(descr->pg_cache_descr->page);
        rrdeng_destroy_pg_cache_descr(ctx, descr->pg_cache_descr);
        free(descr);
    }
    handle->descr = NULL;
}

void rrdeng_store_metric_next(RRDDIM *rd, usec_t point_in_time, storage_number number)
{
    struct rrdeng_collect_handle *handle;
    struct rrdengine_instance *ctx;
    struct page_cache *pg_cache;
    struct rrdeng_page_descr *descr;
    storage_number *page;

    handle = &rd->state->handle.rrdeng;
    ctx = handle->ctx;
    pg_cache = &ctx->pg_cache;
    descr = handle->descr;
    if (unlikely(NULL == descr || descr->page_length + sizeof(number) > RRDENG_BLOCK_SIZE)) {
        rrdeng_store_metric_flush_current_page(rd);

        page = rrdeng_create_page(ctx, &handle->page_index->id, &descr);
        assert(page);

        handle->descr = descr;

        uv_rwlock_wrlock(&pg_cache->commited_page_index.lock);
        handle->page_correlation_id = pg_cache->commited_page_index.latest_corr_id++;
        uv_rwlock_wrunlock(&pg_cache->commited_page_index.lock);
    }
    page = descr->pg_cache_descr->page;

    page[descr->page_length / sizeof(number)] = number;
    descr->end_time = point_in_time;
    descr->page_length += sizeof(number);
    if (unlikely(INVALID_TIME == descr->start_time)) {
        descr->start_time = point_in_time;

#ifdef NETDATA_INTERNAL_CHECKS
        rrd_stat_atomic_add(&ctx->stats.metric_API_producers, 1);
#endif
        pg_cache_insert(ctx, handle->page_index, descr);
    } else {
        pg_cache_add_new_metric_time(handle->page_index, descr);
    }
}

/*
 * Releases the database reference from the handle for storing metrics.
 */
void rrdeng_store_metric_finalize(RRDDIM *rd)
{
    struct rrdeng_collect_handle *handle;
    struct rrdengine_instance *ctx;

    handle = &rd->state->handle.rrdeng;
    ctx = handle->ctx;
    rrdeng_store_metric_flush_current_page(rd);
    if (handle->prev_descr) {
        /* unpin old second page */
        pg_cache_put(ctx, handle->prev_descr);
    }
}

/*
 * Gets a handle for loading metrics from the database.
 * The handle must be released with rrdeng_load_metric_final().
 */
void rrdeng_load_metric_init(RRDDIM *rd, struct rrddim_query_handle *rrdimm_handle, time_t start_time, time_t end_time)
{
    struct rrdeng_query_handle *handle;
    struct rrdengine_instance *ctx;

    ctx = rd->rrdset->rrdhost->rrdeng_ctx;
    rrdimm_handle->start_time = start_time;
    rrdimm_handle->end_time = end_time;
    handle = &rrdimm_handle->rrdeng;
    handle->now = start_time;
    handle->dt = rd->rrdset->update_every;
    handle->ctx = ctx;
    handle->descr = NULL;
    handle->page_index = pg_cache_preload(ctx, rd->state->rrdeng_uuid,
                                          start_time * USEC_PER_SEC, end_time * USEC_PER_SEC);
}

storage_number rrdeng_load_metric_next(struct rrddim_query_handle *rrdimm_handle)
{
    struct rrdeng_query_handle *handle;
    struct rrdengine_instance *ctx;
    struct rrdeng_page_descr *descr;
    storage_number *page, ret;
    unsigned position;
    usec_t point_in_time;

    handle = &rrdimm_handle->rrdeng;
    if (unlikely(INVALID_TIME == handle->now)) {
        return SN_EMPTY_SLOT;
    }
    ctx = handle->ctx;
    point_in_time = handle->now * USEC_PER_SEC;
    descr = handle->descr;

    if (unlikely(NULL == handle->page_index)) {
        ret = SN_EMPTY_SLOT;
        goto out;
    }
    if (unlikely(NULL == descr ||
                 point_in_time < descr->start_time ||
                 point_in_time > descr->end_time)) {
        if (descr) {
#ifdef NETDATA_INTERNAL_CHECKS
            rrd_stat_atomic_add(&ctx->stats.metric_API_consumers, -1);
#endif
            pg_cache_put(ctx, descr);
            handle->descr = NULL;
        }
        descr = pg_cache_lookup(ctx, handle->page_index, &handle->page_index->id, point_in_time);
        if (NULL == descr) {
            ret = SN_EMPTY_SLOT;
            goto out;
        }
#ifdef NETDATA_INTERNAL_CHECKS
        rrd_stat_atomic_add(&ctx->stats.metric_API_consumers, 1);
#endif
        handle->descr = descr;
    }
    if (unlikely(INVALID_TIME == descr->start_time ||
                 INVALID_TIME == descr->end_time)) {
        ret = SN_EMPTY_SLOT;
        goto out;
    }
    page = descr->pg_cache_descr->page;
    if (unlikely(descr->start_time == descr->end_time)) {
        ret = page[0];
        goto out;
    }
    position = ((uint64_t)(point_in_time - descr->start_time)) * (descr->page_length / sizeof(storage_number)) /
               (descr->end_time - descr->start_time + 1);
    ret = page[position];

out:
    handle->now += handle->dt;
    if (unlikely(handle->now > rrdimm_handle->end_time)) {
        handle->now = INVALID_TIME;
    }
    return ret;
}

int rrdeng_load_metric_is_finished(struct rrddim_query_handle *rrdimm_handle)
{
    struct rrdeng_query_handle *handle;

    handle = &rrdimm_handle->rrdeng;
    return (INVALID_TIME == handle->now);
}

/*
 * Releases the database reference from the handle for loading metrics.
 */
void rrdeng_load_metric_finalize(struct rrddim_query_handle *rrdimm_handle)
{
    struct rrdeng_query_handle *handle;
    struct rrdengine_instance *ctx;
    struct rrdeng_page_descr *descr;

    handle = &rrdimm_handle->rrdeng;
    ctx = handle->ctx;
    descr = handle->descr;
    if (descr) {
#ifdef NETDATA_INTERNAL_CHECKS
        rrd_stat_atomic_add(&ctx->stats.metric_API_consumers, -1);
#endif
        pg_cache_put(ctx, descr);
    }
}

time_t rrdeng_metric_latest_time(RRDDIM *rd)
{
    struct rrdeng_collect_handle *handle;
    struct pg_cache_page_index *page_index;

    handle = &rd->state->handle.rrdeng;
    page_index = handle->page_index;

    return page_index->latest_time / USEC_PER_SEC;
}
time_t rrdeng_metric_oldest_time(RRDDIM *rd)
{
    struct rrdeng_collect_handle *handle;
    struct pg_cache_page_index *page_index;

    handle = &rd->state->handle.rrdeng;
    page_index = handle->page_index;

    return page_index->oldest_time / USEC_PER_SEC;
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
    if(unlikely(debug_flags & D_RRDENGINE))
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

    if (unlikely(NULL == descr)) {
        debug(D_RRDENGINE, "%s: page descriptor is NULL, page has already been force-commited.", __func__);
        return;
    }
    assert(descr->page_length);

    uv_rwlock_wrlock(&pg_cache->commited_page_index.lock);
    PValue = JudyLIns(&pg_cache->commited_page_index.JudyL_array, page_correlation_id, PJE0);
    *PValue = descr;
    ++pg_cache->commited_page_index.nr_commited_pages;
    uv_rwlock_wrunlock(&pg_cache->commited_page_index.lock);

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

void rrdeng_get_28_statistics(struct rrdengine_instance *ctx, unsigned long long *array)
{
    struct page_cache *pg_cache = &ctx->pg_cache;

    array[0] = (uint64_t)ctx->stats.metric_API_producers;
    array[1] = (uint64_t)ctx->stats.metric_API_consumers;
    array[2] = (uint64_t)pg_cache->page_descriptors;
    array[3] = (uint64_t)pg_cache->populated_pages;
    array[4] = (uint64_t)pg_cache->commited_page_index.nr_commited_pages;
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
    assert(RRDENG_NR_STATS == 28);
}

/* Releases reference to page */
void rrdeng_put_page(struct rrdengine_instance *ctx, void *handle)
{
    (void)ctx;
    pg_cache_put(ctx, (struct rrdeng_page_descr *)handle);
}

/*
 * Returns 0 on success, 1 on error
 */
int rrdeng_init(struct rrdengine_instance **ctxp, char *dbfiles_path, unsigned page_cache_mb, unsigned disk_space_mb)
{
    struct rrdengine_instance *ctx;
    int error;

    sanity_check();
    if (NULL == ctxp) {
        /* for testing */
        ctx = &default_global_ctx;
        memset(ctx, 0, sizeof(*ctx));
    } else {
        *ctxp = ctx = callocz(1, sizeof(*ctx));
    }
    if (ctx->rrdengine_state != RRDENGINE_STATUS_UNINITIALIZED) {
        return 1;
    }
    ctx->rrdengine_state = RRDENGINE_STATUS_INITIALIZING;
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

    memset(&ctx->worker_config, 0, sizeof(ctx->worker_config));
    ctx->worker_config.ctx = ctx;
    init_page_cache(ctx);
    init_commit_log(ctx);
    error = init_rrd_files(ctx);
    if (error) {
        ctx->rrdengine_state = RRDENGINE_STATUS_UNINITIALIZED;
        if (ctx != &default_global_ctx) {
            freez(ctx);
        }
        return 1;
    }

    init_completion(&ctx->rrdengine_completion);
    assert(0 == uv_thread_create(&ctx->worker_config.thread, rrdeng_worker, &ctx->worker_config));
    /* wait for worker thread to initialize */
    wait_for_completion(&ctx->rrdengine_completion);
    destroy_completion(&ctx->rrdengine_completion);

    ctx->rrdengine_state = RRDENGINE_STATUS_INITIALIZED;
    return 0;
}

/*
 * Returns 0 on success, 1 on error
 */
int rrdeng_exit(struct rrdengine_instance *ctx)
{
    struct rrdeng_cmd cmd;

    if (NULL == ctx) {
        /* TODO: move to per host basis */
        ctx = &default_global_ctx;
    }
    if (ctx->rrdengine_state != RRDENGINE_STATUS_INITIALIZED) {
        return 1;
    }

    /* TODO: add page to page cache */
    cmd.opcode = RRDENG_SHUTDOWN;
    rrdeng_enq_cmd(&ctx->worker_config, &cmd);

    assert(0 == uv_thread_join(&ctx->worker_config.thread));

    if (ctx != &default_global_ctx) {
        freez(ctx);
    }
    return 0;
}