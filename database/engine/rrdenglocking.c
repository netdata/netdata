// SPDX-License-Identifier: GPL-3.0-or-later
#include "rrdengine.h"

struct page_cache_descr *rrdeng_create_pg_cache_descr(struct rrdengine_instance *ctx)
{
    struct page_cache_descr *pg_cache_descr;

    pg_cache_descr = mallocz(sizeof(*pg_cache_descr));
    rrd_stat_atomic_add(&ctx->stats.page_cache_descriptors, 1);
    pg_cache_descr->page = NULL;
    pg_cache_descr->flags = 0;
    pg_cache_descr->prev = pg_cache_descr->next = NULL;
    pg_cache_descr->refcnt = 0;
    pg_cache_descr->waiters = 0;
    assert(0 == uv_cond_init(&pg_cache_descr->cond));
    assert(0 == uv_mutex_init(&pg_cache_descr->mutex));

    return pg_cache_descr;
}

void rrdeng_destroy_pg_cache_descr(struct rrdengine_instance *ctx, struct page_cache_descr *pg_cache_descr)
{
    uv_cond_destroy(&pg_cache_descr->cond);
    uv_mutex_destroy(&pg_cache_descr->mutex);
    freez(pg_cache_descr);
    rrd_stat_atomic_add(&ctx->stats.page_cache_descriptors, -1);
}

/* also allocates page cache descriptor if missing */
void rrdeng_page_descr_mutex_lock(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr)
{
    unsigned long old_state, old_users, new_state, ret_state;
    struct page_cache_descr *pg_cache_descr = NULL;
    uint8_t we_locked;

    we_locked = 0;
    while (1) { /* spin */
        old_state = descr->pg_cache_descr_state;
        old_users = old_state >> PG_CACHE_DESCR_SHIFT;

        if (unlikely(we_locked)) {
            assert(old_state & PG_CACHE_DESCR_LOCKED);
            new_state = (1 << PG_CACHE_DESCR_SHIFT) | PG_CACHE_DESCR_ALLOCATED;
            ret_state = ulong_compare_and_swap(&descr->pg_cache_descr_state, old_state, new_state);
            if (old_state == ret_state) {
                /* success */
                break;
            }
            continue; /* spin */
        }
        if (old_state & PG_CACHE_DESCR_LOCKED) {
            assert(0 == old_users);
            continue; /* spin */
        }
        if (0 == old_state) {
            /* no page cache descriptor has been allocated */

            if (NULL == pg_cache_descr) {
                pg_cache_descr = rrdeng_create_pg_cache_descr(ctx);
            }
            new_state = PG_CACHE_DESCR_LOCKED;
            ret_state = ulong_compare_and_swap(&descr->pg_cache_descr_state, 0, new_state);
            if (0 == ret_state) {
                we_locked = 1;
                descr->pg_cache_descr = pg_cache_descr;
                pg_cache_descr->descr = descr;
                pg_cache_descr = NULL; /* make sure we don't free pg_cache_descr */
                /* retry */
                continue;
            }
            continue; /* spin */
        }
        /* page cache descriptor is already allocated */
        assert(old_state & PG_CACHE_DESCR_ALLOCATED);

        new_state = (old_users + 1) << PG_CACHE_DESCR_SHIFT;
        new_state |= old_state & PG_CACHE_DESCR_FLAGS_MASK;

        ret_state = ulong_compare_and_swap(&descr->pg_cache_descr_state, old_state, new_state);
        if (old_state == ret_state) {
            /* success */
            break;
        }
        /* spin */
    }

    if (pg_cache_descr) {
        rrdeng_destroy_pg_cache_descr(ctx, pg_cache_descr);
    }
    pg_cache_descr = descr->pg_cache_descr;
    uv_mutex_lock(&pg_cache_descr->mutex);
}

void rrdeng_page_descr_mutex_unlock(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr)
{
    unsigned long old_state, new_state, ret_state, old_users;
    struct page_cache_descr *pg_cache_descr;
    uint8_t we_locked;

    uv_mutex_unlock(&descr->pg_cache_descr->mutex);

    we_locked = 0;
    while (1) { /* spin */
        old_state = descr->pg_cache_descr_state;
        old_users = old_state >> PG_CACHE_DESCR_SHIFT;

        if (unlikely(we_locked)) {
            assert(0 == old_users);

            ret_state = ulong_compare_and_swap(&descr->pg_cache_descr_state, old_state, 0);
            if (old_state == ret_state) {
                /* success */
                break;
            }
            continue; /* spin */
        }
        if (old_state & PG_CACHE_DESCR_LOCKED) {
            assert(0 == old_users);
            continue; /* spin */
        }
        assert(old_state & PG_CACHE_DESCR_ALLOCATED);
        pg_cache_descr = descr->pg_cache_descr;
        /* caller is the only page cache descriptor user and there are no pending references on the page */
        if ((old_state & PG_CACHE_DESCR_DESTROY) && (1 == old_users) &&
            !pg_cache_descr->flags && !pg_cache_descr->refcnt) {
            new_state = PG_CACHE_DESCR_LOCKED;
            ret_state = ulong_compare_and_swap(&descr->pg_cache_descr_state, old_state, new_state);
            if (old_state == ret_state) {
                we_locked = 1;
                rrdeng_destroy_pg_cache_descr(ctx, pg_cache_descr);
                /* retry */
                continue;
            }
            continue; /* spin */
        }
        assert(old_users > 0);
        new_state = (old_users - 1) << PG_CACHE_DESCR_SHIFT;
        new_state |= old_state & PG_CACHE_DESCR_FLAGS_MASK;

        ret_state = ulong_compare_and_swap(&descr->pg_cache_descr_state, old_state, new_state);
        if (old_state == ret_state) {
            /* success */
            break;
        }
        /* spin */
    }

}

/*
 * Tries to deallocate page cache descriptor. If it fails, it postpones deallocation by setting the
 * PG_CACHE_DESCR_DESTROY flag which will be eventually cleared by a different context after doing
 * the deallocation.
 */
void rrdeng_try_deallocate_pg_cache_descr(struct rrdengine_instance *ctx, struct rrdeng_page_descr *descr)
{
    unsigned long old_state, new_state, ret_state, old_users;
    struct page_cache_descr *pg_cache_descr;
    uint8_t just_locked, we_freed, must_unlock;

    just_locked = 0;
    we_freed = 0;
    must_unlock = 0;
    while (1) { /* spin */
        old_state = descr->pg_cache_descr_state;
        old_users = old_state >> PG_CACHE_DESCR_SHIFT;

        if (unlikely(just_locked)) {
            assert(0 == old_users);

            must_unlock = 1;
            just_locked = 0;
            /* Try deallocate if there are no pending references on the page */
            if (!pg_cache_descr->flags && !pg_cache_descr->refcnt) {
                rrdeng_destroy_pg_cache_descr(ctx, pg_cache_descr);
                we_freed = 1;
                /* success */
                continue;
            }
            continue; /* spin */
        }
        if (unlikely(must_unlock)) {
            assert(0 == old_users);

            if (we_freed) {
                /* success */
                new_state = 0;
            } else {
                new_state = old_state | PG_CACHE_DESCR_DESTROY;
                new_state &= ~PG_CACHE_DESCR_LOCKED;
            }
            ret_state = ulong_compare_and_swap(&descr->pg_cache_descr_state, old_state, new_state);
            if (old_state == ret_state) {
                /* unlocked */
                return;
            }
            continue; /* spin */
        }
        if (!(old_state & PG_CACHE_DESCR_ALLOCATED)) {
            /* don't do anything */
            return;
        }
        if (old_state & PG_CACHE_DESCR_LOCKED) {
            assert(0 == old_users);
            continue; /* spin */
        }
        pg_cache_descr = descr->pg_cache_descr;
        /* caller is the only page cache descriptor user */
        if (0 == old_users) {
            new_state = old_state | PG_CACHE_DESCR_LOCKED;
            ret_state = ulong_compare_and_swap(&descr->pg_cache_descr_state, old_state, new_state);
            if (old_state == ret_state) {
                just_locked = 1;
                /* retry */
                continue;
            }
            continue; /* spin */
        }
        if (old_state & PG_CACHE_DESCR_DESTROY) {
            /* don't do anything */
            return;
        }
        /* plant PG_CACHE_DESCR_DESTROY so that other contexts eventually free the page cache descriptor */
        new_state = old_state | PG_CACHE_DESCR_DESTROY;

        ret_state = ulong_compare_and_swap(&descr->pg_cache_descr_state, old_state, new_state);
        if (old_state == ret_state) {
            /* success */
            return;
        }
        /* spin */
    }
}