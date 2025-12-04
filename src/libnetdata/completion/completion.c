// SPDX-License-Identifier: GPL-3.0-or-later

#include "completion.h"

ALWAYS_INLINE void completion_reset(struct completion *p)
{
    if (!p)
        return;
    p->completed = 0;
    p->completed_jobs = 0;
}

ALWAYS_INLINE void completion_init(struct completion *p)
{
    p->completed = 0;
    p->completed_jobs = 0;
    fatal_assert(0 == netdata_cond_init(&p->cond));
    fatal_assert(0 == netdata_mutex_init(&p->mutex));
}

ALWAYS_INLINE void completion_destroy(struct completion *p)
{
    netdata_cond_destroy(&p->cond);
    netdata_mutex_destroy(&p->mutex);
}

ALWAYS_INLINE void completion_wait_for(struct completion *p)
{
    netdata_mutex_lock(&p->mutex);
    while (0 == p->completed) {
        netdata_cond_wait(&p->cond, &p->mutex);
    }
    fatal_assert(1 == p->completed);
    netdata_mutex_unlock(&p->mutex);
}

ALWAYS_INLINE bool completion_timedwait_for(struct completion *p, uint64_t timeout_s)
{
    uint64_t timeout_ns = timeout_s * NSEC_PER_SEC;
    if (timeout_ns == 0) timeout_ns = 1;

    uint64_t deadline_ns = uv_hrtime() + timeout_ns;
    bool result = true;

    netdata_mutex_lock(&p->mutex);
    while (!p->completed && result) {
        uint64_t current_time_ns = uv_hrtime();

        // Check if we've already exceeded the deadline
        if (current_time_ns >= deadline_ns) {
            result = false;
            break;
        }

        uint64_t remaining_timeout_ns = deadline_ns - current_time_ns;

        int rc = netdata_cond_timedwait(&p->cond, &p->mutex, remaining_timeout_ns);

        if (rc == UV_ETIMEDOUT)
            result = false;

        // Condition was signaled (or spurious wakeup).
        // The loop condition `!p->completed` will be re-evaluated.
        // If p->completed is true, the loop exits.
        // If p->completed is false (spurious wakeup), the loop continues with a new remaining_timeout_ns.
    }
    netdata_mutex_unlock(&p->mutex);

    return result;
}

ALWAYS_INLINE void completion_mark_complete(struct completion *p)
{
    netdata_mutex_lock(&p->mutex);
    p->completed = 1;
    netdata_cond_broadcast(&p->cond);
    netdata_mutex_unlock(&p->mutex);
}

ALWAYS_INLINE unsigned completion_wait_for_a_job(struct completion *p, unsigned completed_jobs)
{
    netdata_mutex_lock(&p->mutex);
    while (0 == p->completed && p->completed_jobs <= completed_jobs) {
        netdata_cond_wait(&p->cond, &p->mutex);
    }
    completed_jobs = p->completed_jobs;
    netdata_mutex_unlock(&p->mutex);

    return completed_jobs;
}

ALWAYS_INLINE unsigned completion_wait_for_a_job_with_timeout(struct completion *p, unsigned completed_jobs, uint64_t timeout_ms)
{
    uint64_t timeout_ns = timeout_ms * NSEC_PER_MSEC;
    if (timeout_ns == 0) timeout_ns = 1;

    uint64_t deadline_ns = uv_hrtime() + timeout_ns;

    netdata_mutex_lock(&p->mutex);

    while (p->completed == 0 && p->completed_jobs <= completed_jobs) {
        uint64_t current_time_ns = uv_hrtime();

        // Check if we've already exceeded the deadline
        if (current_time_ns >= deadline_ns) {
            break;
        }

        uint64_t remaining_timeout_ns = deadline_ns - current_time_ns;

        int rc = netdata_cond_timedwait(&p->cond, &p->mutex, remaining_timeout_ns);
        if (rc == UV_ETIMEDOUT)
            break;
    }

    completed_jobs = p->completed_jobs;
    netdata_mutex_unlock(&p->mutex);

    return completed_jobs;
}

ALWAYS_INLINE void completion_mark_complete_a_job(struct completion *p)
{
    netdata_mutex_lock(&p->mutex);
    p->completed_jobs++;
    netdata_cond_broadcast(&p->cond);
    netdata_mutex_unlock(&p->mutex);
}

ALWAYS_INLINE bool completion_is_done(struct completion *p)
{
    bool ret;
    netdata_mutex_lock(&p->mutex);
    ret = p->completed;
    netdata_mutex_unlock(&p->mutex);
    return ret;
}
