// SPDX-License-Identifier: GPL-3.0-or-later

#include "completion.h"

ALWAYS_INLINE void completion_init(struct completion *p)
{
    p->completed = 0;
    p->completed_jobs = 0;
    fatal_assert(0 == uv_cond_init(&p->cond));
    fatal_assert(0 == uv_mutex_init(&p->mutex));
}

ALWAYS_INLINE void completion_destroy(struct completion *p)
{
    uv_cond_destroy(&p->cond);
    uv_mutex_destroy(&p->mutex);
}

ALWAYS_INLINE void completion_wait_for(struct completion *p)
{
    uv_mutex_lock(&p->mutex);
    while (0 == p->completed) {
        uv_cond_wait(&p->cond, &p->mutex);
    }
    fatal_assert(1 == p->completed);
    uv_mutex_unlock(&p->mutex);
}

ALWAYS_INLINE bool completion_timedwait_for(struct completion *p, uint64_t timeout_s)
{
    timeout_s *= NSEC_PER_SEC;

    uint64_t start_time = uv_hrtime();
    bool result = true;

    uv_mutex_lock(&p->mutex);
    while (!p->completed) {
        int rc = uv_cond_timedwait(&p->cond, &p->mutex, timeout_s);

        if (rc == 0) {
            result = true;
            break;
        } else if (rc == UV_ETIMEDOUT) {
            result = false;
            break;
        }

        /*
         * handle spurious wakeups
        */

        uint64_t elapsed = uv_hrtime() - start_time;
        if (elapsed >= timeout_s) {
            result = false;
            break;
        }
        timeout_s -= elapsed;
    }
    uv_mutex_unlock(&p->mutex);

    return result;
}

ALWAYS_INLINE void completion_mark_complete(struct completion *p)
{
    uv_mutex_lock(&p->mutex);
    p->completed = 1;
    uv_cond_broadcast(&p->cond);
    uv_mutex_unlock(&p->mutex);
}

ALWAYS_INLINE unsigned completion_wait_for_a_job(struct completion *p, unsigned completed_jobs)
{
    uv_mutex_lock(&p->mutex);
    while (0 == p->completed && p->completed_jobs <= completed_jobs) {
        uv_cond_wait(&p->cond, &p->mutex);
    }
    completed_jobs = p->completed_jobs;
    uv_mutex_unlock(&p->mutex);

    return completed_jobs;
}

ALWAYS_INLINE unsigned completion_wait_for_a_job_with_timeout(struct completion *p, unsigned completed_jobs, uint64_t timeout_ms)
{
    uint64_t timeout_ns = timeout_ms * NSEC_PER_MSEC;
    if(!timeout_ns) timeout_ns = 1;

    uint64_t start_time_ns = uv_hrtime();

    uv_mutex_lock(&p->mutex);
    while (0 == p->completed && p->completed_jobs <= completed_jobs) {
        int rc = uv_cond_timedwait(&p->cond, &p->mutex, timeout_ns);
        if(rc == UV_ETIMEDOUT)
            break;

        uint64_t elapsed = uv_hrtime() - start_time_ns;
        if (elapsed >= timeout_ns) break;
        timeout_ns -= elapsed;
    }
    completed_jobs = p->completed_jobs;
    uv_mutex_unlock(&p->mutex);

    return completed_jobs;
}

ALWAYS_INLINE void completion_mark_complete_a_job(struct completion *p)
{
    uv_mutex_lock(&p->mutex);
    p->completed_jobs++;
    uv_cond_broadcast(&p->cond);
    uv_mutex_unlock(&p->mutex);
}

ALWAYS_INLINE bool completion_is_done(struct completion *p)
{
    bool ret;
    uv_mutex_lock(&p->mutex);
    ret = p->completed;
    uv_mutex_unlock(&p->mutex);
    return ret;
}
