// SPDX-License-Identifier: GPL-3.0-or-later
#include "rrdengine.h"

/*
 * Gets a handle for storing metrics to the database.
 * The handle must be released with rrdeng_release_store_metric().
 */
void rrdeng_store_metric_init(RRDDIM *rd, struct rrdeng_handle *handle)
{
    void *page;
    uuid_t *temp_id;
    uint32_t *hashp;

    /* TODO: this malloc needs to move to a metrics index */
    handle->uuid = calloc(1, sizeof(uuid_t)); /* sets memory to zero */
    if (unlikely(handle->uuid == NULL)) {
        fprintf(stderr, "malloc failed.\n");
        exit(UV_ENOMEM);
    }
    strncpy((char *)handle->uuid, rd->id, sizeof(uuid_t) - 2 * sizeof(uint32_t));
    hashp = ((void *)handle->uuid) + sizeof(uuid_t) - 2 * sizeof(uint32_t);
    hashp[0] = rd->hash;
    hashp[1] = rd->rrdset->hash;
    handle->descr = NULL;
}

void rrdeng_store_metric_next(struct rrdeng_handle *handle, usec_t point_in_time, storage_number number)
{
    struct rrdeng_page_cache_descr *descr;
    storage_number *page;

    descr = handle->descr;
    if (unlikely(NULL == descr || descr->page_length + sizeof(number) > RRDENG_BLOCK_SIZE)) {
        if (descr) {
            descr->handle = NULL;
            rrdeng_commit_page(descr);
        }
        page = rrdeng_create_page(handle->uuid, &descr);
        assert(page);
        handle->descr = descr;
        descr->handle = handle;
    }
    page = descr->page;

    page[descr->page_length / sizeof(number)] = number;
    if (unlikely(INVALID_TIME == descr->start_time)) {
        descr->start_time = point_in_time;
    }
    descr->end_time = point_in_time;
    descr->page_length += sizeof(number);
}

/*
 * Releases the database reference from the handle for storing metrics.
 */
void rrdeng_store_metric_final(struct rrdeng_handle *handle)
{
    struct rrdeng_page_cache_descr *descr = handle->descr;

    if (descr) {
        descr->handle = NULL;
        rrdeng_commit_page(descr);
    }
}

/*
 * Gets a handle for storing metrics to the database.
 * The handle must be released with rrdeng_release_store_metric().
 */
void rrdeng_load_metric_init(uuid_t *uuid, struct rrdeng_handle *handle, usec_t start_time, usec_t end_time)
{
    void *page;
    uuid_t *temp_id;

    handle->uuid = uuid;
    handle->descr = NULL;
    pg_cache_preload(handle->uuid, start_time, end_time);
}


storage_number rrdeng_load_metric_next(struct rrdeng_handle *handle, usec_t point_in_time)
{
    struct rrdeng_page_cache_descr *descr;
    storage_number *page;
    unsigned position;

    assert(INVALID_TIME != point_in_time);
    descr = handle->descr;
    if (unlikely(NULL == descr ||
                 point_in_time < descr->start_time ||
                 point_in_time > descr->end_time)) {
        if (descr) {
            pg_cache_put(descr);
        }
        descr = pg_cache_lookup(handle->uuid, point_in_time);
        if (NULL == descr) {
            return SN_EMPTY_SLOT;
        }
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
    struct rrdeng_page_cache_descr *descr = handle->descr;

    if (descr) {
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
    descr = malloc(sizeof(*descr));
    if (unlikely(descr == NULL)) {
        fprintf(stderr, "malloc failed.\n");
        free(page);
        *ret_descr = NULL;
        return NULL;
    }
    descr->page = page;
    descr->page_length = 0;
    descr->start_time = INVALID_TIME; /* TODO: netdata should populate this */
    descr->end_time = INVALID_TIME; /* TODO: netdata should populate this */
    descr->id = id; /* TODO: add page type: metric, log, something? */
    descr->extent = NULL;
    descr->flags = RRD_PAGE_DIRTY /*| RRD_PAGE_LOCKED */ | RRD_PAGE_POPULATED /* | BEING_COLLECTED */;
    descr->refcnt = 1;
    assert(0 == uv_cond_init(&descr->cond));
    assert(0 == uv_mutex_init(&descr->mutex));

    fprintf(stderr, "-----------------\nCreated new page:\n-----------------\n");
    print_page_cache_descr(descr);
    pg_cache_insert(descr);
    *ret_descr = descr;
    return page;
}

void rrdeng_commit_page(struct rrdeng_page_cache_descr *descr)
{
    int i;
    struct rrdeng_cmd cmd;
    struct rrdeng_page_cache_descr *tmp;

    if (unlikely(NULL == descr)) {
        fprintf(stderr, "%s: page descriptor is NULL, page has already been force-commited.\n", __func__);
        return;
    }
//    uv_mutex_lock(&descr->mutex);
//    descr->page_length = page_length; /* TODO: it may be empty/0 */
//    descr->end_time = now_boottime_usec(); /* TODO: netdata should populate this */
//    uv_mutex_unlock(&descr->mutex);

    uv_rwlock_wrlock(&pg_cache.commited_pages_rwlock);
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
                uv_cond_wait(&tmp->cond, &tmp->mutex);
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
    for (i = 0 ; i < PAGE_CACHE_MAX_COMMITED_PAGES ; ++i) {
        if (NULL == pg_cache.commited_pages[i]) {
            pg_cache.commited_pages[i] = descr;
            ++pg_cache.nr_commited_pages;
            break;
        }
    }
    assert (i != PAGE_CACHE_MAX_COMMITED_PAGES);
    uv_rwlock_wrunlock(&pg_cache.commited_pages_rwlock);

    pg_cache_put(descr);
}

/* Gets a reference for the page */
void *rrdeng_get_latest_page(uuid_t *id, void **handle)
{
    struct rrdeng_page_cache_descr *descr;
    void *page;
    int ret;

    fprintf(stderr, "----------------------\nReading existing page:\n----------------------\n");
    descr = pg_cache_lookup(id, INVALID_TIME);
    if (NULL == descr) {
        *handle = NULL;

        return NULL;
    }
    *handle = descr;

    return descr->page;
}

/* Gets a reference for the page */
void *rrdeng_get_page(uuid_t *id, usec_t point_in_time, void **handle)
{
    struct rrdeng_page_cache_descr *descr;
    void *page;
    int ret;

    fprintf(stderr, "----------------------\nReading existing page:\n----------------------\n");
    descr = pg_cache_lookup(id, point_in_time);
    if (NULL == descr) {
        *handle = NULL;

        return NULL;
    }
    *handle = descr;

    return descr->page;
}

/* Releases reference to page */
void rrdeng_put_page(void *handle)
{
    pg_cache_put((struct rrdeng_page_cache_descr *)handle);
}

/*
 * Returns 0 on success, 1 on error
 */
int rrdeng_init(void)
{
    int error;

    if (rrdengine_state != RRDENGINE_STATUS_UNINITIALIZED) {
        return 1;
    }
    rrdengine_state = RRDENGINE_STATUS_INITIALIZING;
    sanity_check();

    error = 0;
    memset(&worker_config, 0, sizeof(worker_config));
    init_page_cache();
    error = init_rrd_files();
    if (error)
        return error;


    init_completion(&rrdengine_completion);
    assert(0 == uv_thread_create(&worker_config.thread, rrdeng_worker, &worker_config));
    /* wait for worker thread to initialize */
    wait_for_completion(&rrdengine_completion);
    destroy_completion(&rrdengine_completion);

    if (error) {
        rrdengine_state = RRDENGINE_STATUS_UNINITIALIZED;
        return 1;
    }

    rrdengine_state = RRDENGINE_STATUS_INITIALIZED;
    return 0;
}

/*
 * Returns 0 on success, 1 on error
 */
int rrdeng_exit(void)
{
    struct rrdeng_cmd cmd;

    if (rrdengine_state != RRDENGINE_STATUS_INITIALIZED) {
        return 1;
    }

    /* TODO: add page to page cache */
    cmd.opcode = RRDENG_SHUTDOWN;
    rrdeng_enq_cmd(&worker_config, &cmd);

    assert(0 == uv_thread_join(&worker_config.thread));

    return 0;
}