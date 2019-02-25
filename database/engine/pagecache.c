// SPDX-License-Identifier: GPL-3.0-or-later
#define NETDATA_RRD_INTERNALS

#include "rrdengine.h"

/* Returns page flags */
unsigned long pg_cache_wait_event(struct rrdeng_page_cache_descr *descr)
{
    unsigned long flags;

    uv_mutex_lock(&descr->mutex);
    uv_cond_wait(&descr->cond, &descr->mutex);
    flags = descr->flags;
    uv_mutex_unlock(&descr->mutex);

    return flags;
}

/*
 * The caller must hold the page cache and the page descriptor locks
 * TODO: implement
 * TODO: last waiter frees descriptor
 */
static void pg_cache_punch_hole_unsafe(struct rrdeng_page_cache_descr *descr)
{
    return;
}

/*
 * The caller must hold page descriptor lock.
 * Gets a reference to the page descriptor.
 * Returns 1 on success and 0 on failure.
 */
int pg_cache_try_get_unsafe(struct rrdeng_page_cache_descr *descr, int exclusive_access)
{
    if ((descr->flags & (RRD_PAGE_LOCKED | RRD_PAGE_READ_PENDING)) ||
        (exclusive_access && descr->refcnt)) {
        return 0;
    }
    if (exclusive_access)
        descr->flags |= RRD_PAGE_LOCKED;
    ++descr->refcnt;

    return 1;
}

/*
 * The caller must hold the page descriptor lock.
 * This function may block doing cleanup.
 */
void pg_cache_put_unsafe(struct rrdeng_page_cache_descr *descr)
{
    --descr->refcnt;
    descr->flags &= ~RRD_PAGE_LOCKED;
    /* TODO: perform cleanup */
}

/*
 * This function may block doing cleanup.
 */
void pg_cache_put(struct rrdeng_page_cache_descr *descr)
{
    uv_mutex_lock(&descr->mutex);
    pg_cache_put_unsafe(descr);
    uv_mutex_unlock(&descr->mutex);
}

/* The caller must hold the page cache and the page descriptor locks */
static void pg_cache_evict_unsafe(struct rrdeng_page_cache_descr *descr)
{
    free(descr->page);
    descr->page = NULL;
    descr->flags &= ~RRD_PAGE_POPULATED;
    --pg_cache.populated_pages;
}

/*
 * The caller must hold the page cache lock.
 * This function iterates all pages and tries to evict one.
 * If it fails it sets dirty_pages to the number of dirty pages iterated.
 * If it fails it sets in_flight_descr to the last descriptor that has write-back in progress,
 * or it sets it to NULL if no write-back is in progress.
 *
 * Returns 1 on success and 0 on failure.
 */
static int pg_cache_try_evict_one_page_unsafe(struct rrdeng_page_cache_descr **in_flight_descr)
{
    int i;
    unsigned long old_flags;
    struct rrdeng_page_cache_descr *tmp, *failed_descr;

    failed_descr = NULL;

    for (i = 0; i < PAGE_CACHE_MAX_SIZE; ++i) {
        tmp = pg_cache.page_cache_array[i];
        if (NULL == tmp)
            continue;
        uv_mutex_lock(&tmp->mutex);
        old_flags = tmp->flags;
        if (old_flags & RRD_PAGE_POPULATED) {
            int locked_page = 0;
            /* must evict */
            if (pg_cache_try_get_unsafe(tmp, 1)) {
                locked_page = 1;
            }
            if (locked_page && !(old_flags & RRD_PAGE_DIRTY)) {
                pg_cache_evict_unsafe(tmp);
                pg_cache_put_unsafe(tmp);
                uv_mutex_unlock(&tmp->mutex);
                break;
            }
            if (old_flags & RRD_PAGE_WRITE_PENDING) {
                failed_descr = tmp;
            }
            if (locked_page)
                pg_cache_put_unsafe(tmp);
        }
        uv_mutex_unlock(&tmp->mutex);
    }
    if (i == PAGE_CACHE_MAX_SIZE) {
        /* failed to evict */
        *in_flight_descr = failed_descr;
        return 0;
    }
    return 1;
}

/*
 * This function will reserve #number populated page. It will trigger evictions and page flushing
 * if the PAGE_CACHE_MAX_PAGES limit is hit.
 */
static void pg_cache_reserve_pages(unsigned number)
{
    struct rrdeng_page_cache_descr *in_flight_descr;

    assert(number < PAGE_CACHE_MAX_PAGES);

    uv_rwlock_wrlock(&pg_cache.pg_cache_rwlock);
    while (pg_cache.populated_pages + number == PAGE_CACHE_MAX_PAGES + 1) {
        fprintf(stderr, "=================================\nPage cache full. Trying to evict.\n=================================\n");
        if (!pg_cache_try_evict_one_page_unsafe(&in_flight_descr)) {
            /* failed to evict */
            if (in_flight_descr) {
                uv_mutex_lock(&in_flight_descr->mutex);
                uv_rwlock_wrunlock(&pg_cache.pg_cache_rwlock);
                fprintf(stderr, "%s: waiting for page to be written to disk before evicting:\n", __func__);
                print_page_cache_descr(in_flight_descr);
                uv_cond_wait(&in_flight_descr->cond, &in_flight_descr->mutex);
                uv_mutex_unlock(&in_flight_descr->mutex);
            } else {
                struct completion compl;
                struct rrdeng_cmd cmd;

                uv_rwlock_wrunlock(&pg_cache.pg_cache_rwlock);

                init_completion(&compl);
                cmd.opcode = RRDENG_FLUSH_PAGES;
                cmd.completion = &compl;
                rrdeng_enq_cmd(&worker_config, &cmd);
                /* wait for some pages to be flushed */
                fprintf(stderr, "%s: waiting for pages to be written to disk before evicting.\n", __func__);
                wait_for_completion(&compl);
                destroy_completion(&compl);
            }
            uv_rwlock_wrlock(&pg_cache.pg_cache_rwlock);
        }
    }
    pg_cache.populated_pages += number;
    uv_rwlock_wrunlock(&pg_cache.pg_cache_rwlock);
}

void pg_cache_insert(struct rrdeng_page_cache_descr *descr)
{
    int i;
    struct rrdeng_page_cache_descr *tmp;
    unsigned long old_flags;
    unsigned dirty_pages;
    struct rrdeng_page_cache_descr *in_flight_descr;

    if (descr->flags & RRD_PAGE_POPULATED)
        pg_cache_reserve_pages(1);

    uv_rwlock_wrlock(&pg_cache.pg_cache_rwlock);
    for (i = 0; i < PAGE_CACHE_MAX_SIZE ; ++i) {
        tmp = pg_cache.page_cache_array[i];
        if (NULL == tmp) {
            pg_cache.page_cache_array[i] = descr;
            ++pg_cache.pages;
            if (descr->flags & RRD_PAGE_POPULATED) {
            }
            break;
        }
    }
    uv_rwlock_wrunlock(&pg_cache.pg_cache_rwlock);

    if (i == PAGE_CACHE_MAX_SIZE) {
        uv_rwlock_wrlock(&pg_cache.pg_cache_rwlock);
        /* cancel reservation */
        --pg_cache.populated_pages;
        uv_rwlock_wrunlock(&pg_cache.pg_cache_rwlock);

        fprintf(stderr, "CRITICAL: Out of page cache descriptors. Cannot insert:\n");
        print_page_cache_descr(descr);
    }
}

/*
 * Searches for a page and triggers disk I/O if necessary and possible.
 * Does not get a reference.
 */
void pg_cache_preload(uuid_t *id, usec_t start_time, usec_t end_time)
{
    struct rrdeng_page_cache_descr *descr = NULL, *preload_array[PAGE_CACHE_MAX_PRELOAD_PAGES];
    int i, j, k, got_ref, count;
    unsigned long flags;

    count = 0;

    uv_rwlock_rdlock(&pg_cache.pg_cache_rwlock);
    for (i = 0 ; i < PAGE_CACHE_MAX_SIZE ; ++i) {
        if (NULL == (descr = pg_cache.page_cache_array[i]) ||
            0 == descr->page_length ||
            uuid_compare(*id, *descr->id) ||
            (descr->start_time < start_time || descr->start_time > end_time) &&
            (descr->end_time < start_time || descr->end_time > end_time)) {
            continue;
        }
        got_ref = 0;

        uv_mutex_lock(&descr->mutex);
        flags = descr->flags;
        if (pg_cache_try_get_unsafe(descr, 0)) {
            if (flags & RRD_PAGE_POPULATED) {
                /* success */
                uv_mutex_unlock(&descr->mutex);
                fprintf(stderr, "%s: Page was found in memory.\n", __func__);
                pg_cache_put_unsafe(descr);
                break;
            }
            got_ref = 1;
        }
        if (got_ref)
            pg_cache_put_unsafe(descr);
        if (!(flags & RRD_PAGE_POPULATED) && pg_cache_try_get_unsafe(descr, 1)) {
            if (flags & RRD_PAGE_POPULATED) {
                /* success */
                pg_cache_put_unsafe(descr);
                uv_mutex_unlock(&descr->mutex);
                break;
            }
            preload_array[count++] = descr;
            if (PAGE_CACHE_MAX_PRELOAD_PAGES == count) {
                uv_mutex_unlock(&descr->mutex);
                break;
            }
        }
        uv_mutex_unlock(&descr->mutex);
    }
    uv_rwlock_rdunlock(&pg_cache.pg_cache_rwlock);

    for (i = 0 ; i < count ; ++i) {
        struct rrdeng_cmd cmd;
        struct rrdeng_page_cache_descr *next;

        descr = preload_array[i];
        if (NULL == descr) {
            continue;
        }
        cmd.opcode = RRDENG_READ_EXTENT;
        cmd.read_extent.page_cache_descr[0] = descr;
        for (j = 0, k = 1 ; j < count ; ++j) {
            next = preload_array[j];
            if (NULL == next) {
                continue;
            }
            if (descr->extent == next->extent) {
                /* same extent, consolidate */
                cmd.read_extent.page_cache_descr[k++] = next;
                /* don't use this page again */
                preload_array[j] = NULL;
            }
        }
        cmd.read_extent.page_count = k;
        pg_cache_reserve_pages(k);
        rrdeng_enq_cmd(&worker_config, &cmd);
    }

    if (i == PAGE_CACHE_MAX_SIZE && !count) {
        /* no such page */
        fprintf(stderr, "%s: No page was found to attempt preload.\n", __func__);
    }
}

/*
 * Searches for a page and gets a reference.
 * When point_in_time is INVALID_TIME get any page.
 */
struct rrdeng_page_cache_descr *pg_cache_lookup(uuid_t *id, usec_t point_in_time)
{
    struct rrdeng_page_cache_descr *descr = NULL;
    int i, page_cache_locked, got_ref;
    unsigned long flags;

    page_cache_locked = 1;
    uv_rwlock_rdlock(&pg_cache.pg_cache_rwlock);
    for (i = 0 ; i < PAGE_CACHE_MAX_SIZE ; ++i) {
        if (NULL == (descr = pg_cache.page_cache_array[i]) ||
            0 == descr->page_length ||
            uuid_compare(*id, *descr->id) ||
            (INVALID_TIME != point_in_time &&
             (point_in_time < descr->start_time || point_in_time > descr->end_time))) {
            continue;
        }
        got_ref = 0;

        uv_mutex_lock(&descr->mutex);
        flags = descr->flags;
        if (pg_cache_try_get_unsafe(descr, 0)) {
            if (flags & RRD_PAGE_POPULATED) {
                /* success */
                uv_mutex_unlock(&descr->mutex);
                fprintf(stderr, "%s: Page was found in memory.\n", __func__);
                break;
            }
            got_ref = 1;
        }
        if (got_ref)
            pg_cache_put_unsafe(descr);
        if (!(flags & RRD_PAGE_POPULATED) && pg_cache_try_get_unsafe(descr, 1)) {
            struct rrdeng_cmd cmd;

            if (flags & RRD_PAGE_POPULATED) {
                /* success */
                /* Downgrade exclusive reference to allow other readers */
                descr->flags &= ~RRD_PAGE_LOCKED;
                uv_mutex_unlock(&descr->mutex);
                break;
            }
            uv_rwlock_rdunlock(&pg_cache.pg_cache_rwlock);
            page_cache_locked = 0;

            pg_cache_reserve_pages(1);
            cmd.opcode = RRDENG_READ_PAGE;
            cmd.read_page.page_cache_descr = descr;
            rrdeng_enq_cmd(&worker_config, &cmd);

            fprintf(stderr, "%s: Waiting for page to be asynchronously read from disk:\n", __func__);
            print_page_cache_descr(descr);
            while (!(descr->flags & RRD_PAGE_POPULATED)) {
                uv_cond_wait(&descr->cond, &descr->mutex);
            }
            /* success */
            /* Downgrade exclusive reference to allow other readers */
            descr->flags &= ~RRD_PAGE_LOCKED;
            uv_mutex_unlock(&descr->mutex);
            break;
        }
        uv_rwlock_rdunlock(&pg_cache.pg_cache_rwlock);
        fprintf(stderr, "%s: Waiting for page to be unlocked:", __func__);
        print_page_cache_descr(descr);
        uv_cond_wait(&descr->cond, &descr->mutex);
        uv_mutex_unlock(&descr->mutex);

        /* reset scan to find again */
        i = -1;
        uv_rwlock_rdlock(&pg_cache.pg_cache_rwlock);
    }
    if (page_cache_locked)
        uv_rwlock_rdunlock(&pg_cache.pg_cache_rwlock);

    if (i == PAGE_CACHE_MAX_SIZE) {
        /* no such page */
        return NULL;
    }

    return descr;
}

void init_page_cache(void)
{
    int i;

    for (i = 0 ; i < PAGE_CACHE_MAX_SIZE ; ++i) {
        pg_cache.page_cache_array[i] = NULL;
    }
    pg_cache.pages = 0;
    pg_cache.populated_pages = 0;
    assert(0 == uv_rwlock_init(&pg_cache.pg_cache_rwlock));

    for (i = 0 ; i < PAGE_CACHE_MAX_COMMITED_PAGES ; ++i) {
        pg_cache.commited_pages[i] = NULL;
    }
    pg_cache.nr_commited_pages = 0;
    assert(0 == uv_rwlock_init(&pg_cache.commited_pages_rwlock));
}