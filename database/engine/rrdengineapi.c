// SPDX-License-Identifier: GPL-3.0-or-later
#include "rrdengine.h"

/* Default global database state */
static struct rrdengine_instance default_global_ctx;

/*
 * Gets a handle for storing metrics to the database.
 * The handle must be released with rrdeng_release_store_metric().
 */
void rrdeng_store_metric_init(struct rrdengine_instance *ctx, RRDDIM *rd, struct rrdeng_handle *handle)
{
    if (NULL == ctx) {
        /* TODO: move to per host basis */
        ctx = &default_global_ctx;
    }
    struct page_cache *pg_cache = &ctx->pg_cache;
    uuid_t temp_id;
    uint32_t *hashp;
    Pvoid_t *PValue;
    struct pg_cache_page_index *page_index;

    handle->ctx = ctx;

    memset(&temp_id, 0, sizeof(temp_id));
    strncpy((char *)&temp_id, rd->id, sizeof(uuid_t) - 2 * sizeof(uint32_t));
    hashp = ((void *)&temp_id) + sizeof(uuid_t) - 2 * sizeof(uint32_t);
    hashp[0] = rd->hash;
    hashp[1] = rd->rrdset->hash;
    handle->descr = NULL;

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
    handle->uuid = &page_index->id;
}

void rrdeng_store_metric_next(struct rrdeng_handle *handle, usec_t point_in_time, storage_number number)
{
    struct rrdengine_instance *ctx = handle->ctx;
    struct page_cache *pg_cache = &ctx->pg_cache;
    struct rrdeng_page_cache_descr *descr;
    storage_number *page;

    descr = handle->descr;
    if (unlikely(NULL == descr || descr->page_length + sizeof(number) > RRDENG_BLOCK_SIZE)) {
        if (descr) {
            descr->handle = NULL;
            if (descr->page_length) {
                uv_rwlock_wrlock(&pg_cache->commited_page_index.lock);
                --pg_cache->producers; /* DEBUG STAT */
                uv_rwlock_wrunlock(&pg_cache->commited_page_index.lock);

                rrdeng_commit_page(ctx, descr, handle->page_correlation_id);
            } else {
                free(descr->page);
                free(descr);
            }
        }
        page = rrdeng_create_page(handle->uuid, &descr);
        assert(page);
        handle->descr = descr;
        descr->handle = handle;
        uv_rwlock_wrlock(&pg_cache->commited_page_index.lock);
        handle->page_correlation_id = pg_cache->commited_page_index.latest_corr_id++;
        uv_rwlock_wrunlock(&pg_cache->commited_page_index.lock);
    }
    page = descr->page;

    page[descr->page_length / sizeof(number)] = number;
    descr->end_time = point_in_time;
    descr->page_length += sizeof(number);
    if (unlikely(INVALID_TIME == descr->start_time)) {
        descr->start_time = point_in_time;

        uv_rwlock_wrlock(&pg_cache->commited_page_index.lock);
        ++pg_cache->producers; /* DEBUG STAT */
        uv_rwlock_wrunlock(&pg_cache->commited_page_index.lock);

        pg_cache_insert(ctx, descr);
    }
}

/*
 * Releases the database reference from the handle for storing metrics.
 */
void rrdeng_store_metric_final(struct rrdeng_handle *handle)
{
    struct rrdengine_instance *ctx = handle->ctx;
    struct rrdeng_page_cache_descr *descr = handle->descr;

    if (descr) {
        descr->handle = NULL;
        if (descr->page_length) {
            struct page_cache *pg_cache = &ctx->pg_cache;
            uv_rwlock_wrlock(&pg_cache->commited_page_index.lock);
            --pg_cache->producers; /* DEBUG STAT */
            uv_rwlock_wrunlock(&pg_cache->commited_page_index.lock);

            rrdeng_commit_page(ctx, descr, handle->page_correlation_id);
        } else {
            free(descr->page);
            free(descr);
        }
    }
}

/*
 * Gets a handle for storing metrics to the database.
 * The handle must be released with rrdeng_release_store_metric().
 */
void rrdeng_load_metric_init(struct rrdengine_instance *ctx, uuid_t *uuid, struct rrdeng_handle *handle,
                             usec_t start_time, usec_t end_time)
{
    if (NULL == ctx) {
        /* TODO: move to per host basis */
        ctx = &default_global_ctx;
    }
    handle->ctx = ctx;
    handle->uuid = uuid;
    handle->descr = NULL;
    pg_cache_preload(ctx, handle->uuid, start_time, end_time);
}


storage_number rrdeng_load_metric_next(struct rrdeng_handle *handle, usec_t point_in_time)
{
    struct rrdengine_instance *ctx = handle->ctx;
    struct page_cache *pg_cache = &ctx->pg_cache;
    struct rrdeng_page_cache_descr *descr;
    storage_number *page;
    unsigned position;

    assert(INVALID_TIME != point_in_time);
    descr = handle->descr;
    if (unlikely(NULL == descr ||
                 point_in_time < descr->start_time ||
                 point_in_time > descr->end_time)) {
        if (descr) {
            uv_rwlock_wrlock(&pg_cache->commited_page_index.lock);
            --pg_cache->consumers; /* DEBUG STAT */
            uv_rwlock_wrunlock(&pg_cache->commited_page_index.lock);

            pg_cache_put(descr);
        }
        descr = pg_cache_lookup(ctx, handle->uuid, point_in_time);
        if (NULL == descr) {
            return SN_EMPTY_SLOT;
        }
        uv_rwlock_wrlock(&pg_cache->commited_page_index.lock);
        ++pg_cache->consumers; /* DEBUG STAT */
        uv_rwlock_wrunlock(&pg_cache->commited_page_index.lock);
        handle->descr = descr;
    }
    if (unlikely(INVALID_TIME == descr->start_time ||
                 INVALID_TIME == descr->end_time)) {
        return SN_EMPTY_SLOT;
    }
    page = descr->page;
    if (unlikely(descr->start_time == descr->end_time)) {
        return page[0];
    }
    position = ((uint64_t)(point_in_time - descr->start_time)) * (descr->page_length / sizeof(storage_number)) /
               (descr->end_time - descr->start_time + 1);
    return page[position];
}

/*
 * Releases the database reference from the handle for storing metrics.
 */
void rrdeng_load_metric_final(struct rrdeng_handle *handle)
{
    struct rrdengine_instance *ctx = handle->ctx;
    struct rrdeng_page_cache_descr *descr = handle->descr;

    if (descr) {
        struct page_cache *pg_cache = &ctx->pg_cache;
        uv_rwlock_wrlock(&pg_cache->commited_page_index.lock);
        --pg_cache->consumers; /* DEBUG STAT */
        uv_rwlock_wrunlock(&pg_cache->commited_page_index.lock);

        pg_cache_put(descr);
    }
}

/* Also gets a reference for the page */
void *rrdeng_create_page(uuid_t *id, struct rrdeng_page_cache_descr **ret_descr)
{
    struct rrdeng_page_cache_descr *descr;
    void *page;
    int ret;

    /* TODO: check maximum number of pages in page cache limit */

    ret = posix_memalign(&page, RRDFILE_ALIGNMENT, RRDENG_BLOCK_SIZE); /*TODO: add page size */
    if (unlikely(ret)) {
        fprintf(stderr, "posix_memalign:%s\n", strerror(ret));
        *ret_descr = NULL;
        return NULL;
    }
    descr = pg_cache_create_descr();
    descr->page = page;
    descr->id = id; /* TODO: add page type: metric, log, something? */
    descr->flags = RRD_PAGE_DIRTY /*| RRD_PAGE_LOCKED */ | RRD_PAGE_POPULATED /* | BEING_COLLECTED */;
    descr->refcnt = 1;

    fprintf(stderr, "-----------------\nCreated new page:\n-----------------\n");
    print_page_cache_descr(descr);
    *ret_descr = descr;
    return page;
}

/* The page must not be empty */
void rrdeng_commit_page(struct rrdengine_instance *ctx, struct rrdeng_page_cache_descr *descr,
                        Word_t page_correlation_id)
{
    struct page_cache *pg_cache = &ctx->pg_cache;
    int i;
    struct rrdeng_cmd cmd;
    struct rrdeng_page_cache_descr *tmp;
    Pvoid_t *PValue;

    if (unlikely(NULL == descr)) {
        fprintf(stderr, "%s: page descriptor is NULL, page has already been force-commited.\n", __func__);
        return;
    }
    assert(descr->page_length);

    uv_rwlock_wrlock(&pg_cache->commited_page_index.lock);
#if 0 /* TODO: examine if this is necessary anymore */
    while (PAGE_CACHE_MAX_COMMITED_PAGES == pg_cache.nr_commited_pages) {
        int found_pending;

        found_pending = 0;
        for (i = 0; i < PAGE_CACHE_MAX_COMMITED_PAGES; ++i) {
            tmp = pg_cache.commited_pages[i];
            assert(tmp);
            uv_mutex_lock(&tmp->mutex);
            if (tmp->flags & RRD_PAGE_WRITE_PENDING) {
                /* wait for the page to be flushed */
                uv_rwlock_wrunlock(&pg_cache.commited_pages_rwlock);
                found_pending = 1;
                fprintf(stderr, "%s: waiting for in-flight page to be written to disk:\n", __func__);
                print_page_cache_descr(tmp);
                pg_cache_wait_event_unsafe(tmp);
                uv_mutex_unlock(&tmp->mutex);
                break;
            }
            uv_mutex_unlock(&tmp->mutex);
        }
        if (!found_pending) {
            struct completion compl;

            uv_rwlock_wrunlock(&pg_cache.commited_pages_rwlock);
            init_completion(&compl);
            cmd.opcode = RRDENG_FLUSH_PAGES;
            cmd.completion = &compl;
            rrdeng_enq_cmd(&worker_config, &cmd);
            /* wait for some pages to be flushed */
            fprintf(stderr, "%s: forcing asynchronous flush of extent. Waiting for completion.\n", __func__);
            wait_for_completion(&compl);
            destroy_completion(&compl);
        }
        uv_rwlock_wrlock(&pg_cache.commited_pages_rwlock);
    }
#endif
    PValue = JudyLIns(&pg_cache->commited_page_index.JudyL_array, page_correlation_id, PJE0);
    *PValue = descr;
    ++pg_cache->commited_page_index.nr_commited_pages;
    uv_rwlock_wrunlock(&pg_cache->commited_page_index.lock);

    pg_cache_put(descr);
}

/* Gets a reference for the page */
void *rrdeng_get_latest_page(struct rrdengine_instance *ctx, uuid_t *id, void **handle)
{
    struct rrdeng_page_cache_descr *descr;
    void *page;
    int ret;

    fprintf(stderr, "----------------------\nReading existing page:\n----------------------\n");
    descr = pg_cache_lookup(ctx, id, INVALID_TIME);
    if (NULL == descr) {
        *handle = NULL;

        return NULL;
    }
    *handle = descr;

    return descr->page;
}

/* Gets a reference for the page */
void *rrdeng_get_page(struct rrdengine_instance *ctx, uuid_t *id, usec_t point_in_time, void **handle)
{
    struct rrdeng_page_cache_descr *descr;
    void *page;
    int ret;

    fprintf(stderr, "----------------------\nReading existing page:\n----------------------\n");
    descr = pg_cache_lookup(ctx, id, point_in_time);
    if (NULL == descr) {
        *handle = NULL;

        return NULL;
    }
    *handle = descr;

    return descr->page;
}

/* Releases reference to page */
void rrdeng_put_page(struct rrdengine_instance *ctx, void *handle)
{
    pg_cache_put((struct rrdeng_page_cache_descr *)handle);
}

/*
 * Returns 0 on success, 1 on error
 */
int rrdeng_init(struct rrdengine_instance *ctx)
{
    int error;

    sanity_check();
    if (NULL == ctx) {
        /* TODO: move to per host basis */
        ctx = &default_global_ctx;
    }
    if (ctx->rrdengine_state != RRDENGINE_STATUS_UNINITIALIZED) {
        return 1;
    }
    ctx->rrdengine_state = RRDENGINE_STATUS_INITIALIZING;
    ctx->global_compress_alg = RRD_LZ4;
    ctx->disk_space = 0;

    error = 0;
    memset(&ctx->worker_config, 0, sizeof(ctx->worker_config));
    ctx->worker_config.ctx = ctx;
    init_page_cache(ctx);
    init_commit_log(ctx);
    error = init_rrd_files(ctx);
    if (error) {
        ctx->rrdengine_state = RRDENGINE_STATUS_UNINITIALIZED;
        return 1;
    }

    init_completion(&ctx->rrdengine_completion);
    assert(0 == uv_thread_create(&ctx->worker_config.thread, rrdeng_worker, &ctx->worker_config));
    /* wait for worker thread to initialize */
    wait_for_completion(&ctx->rrdengine_completion);
    destroy_completion(&ctx->rrdengine_completion);
    if (error) {
        ctx->rrdengine_state = RRDENGINE_STATUS_UNINITIALIZED;
        return 1;
    }

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

    return 0;
}