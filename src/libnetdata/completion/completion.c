// SPDX-License-Identifier: GPL-3.0-or-later

#include "completion.h"

void completion_init(struct completion *p)
{
    p->completed = 0;
    p->completed_jobs = 0;
    fatal_assert(0 == uv_cond_init(&p->cond));
    fatal_assert(0 == uv_mutex_init(&p->mutex));
}

void completion_destroy(struct completion *p)
{
    uv_cond_destroy(&p->cond);
    uv_mutex_destroy(&p->mutex);
}

void completion_wait_for(struct completion *p)
{
    uv_mutex_lock(&p->mutex);
    while (0 == p->completed) {
        uv_cond_wait(&p->cond, &p->mutex);
    }
    fatal_assert(1 == p->completed);
    uv_mutex_unlock(&p->mutex);
}

bool completion_timedwait_for(struct completion *p, uint64_t timeout)
{
    timeout *= NSEC_PER_SEC;

    uint64_t start_time = uv_hrtime();
    bool result = true;

    uv_mutex_lock(&p->mutex);
    while (!p->completed) {
        int rc = uv_cond_timedwait(&p->cond, &p->mutex, timeout);

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
        if (elapsed >= timeout) {
            result = false;
            break;
        }
        timeout -= elapsed;
    }
    uv_mutex_unlock(&p->mutex);

    return result;
}

void completion_mark_complete(struct completion *p)
{
    uv_mutex_lock(&p->mutex);
    p->completed = 1;
    uv_cond_broadcast(&p->cond);
    uv_mutex_unlock(&p->mutex);
}

unsigned completion_wait_for_a_job(struct completion *p, unsigned completed_jobs)
{
    uv_mutex_lock(&p->mutex);
    while (0 == p->completed && p->completed_jobs <= completed_jobs) {
        uv_cond_wait(&p->cond, &p->mutex);
    }
    completed_jobs = p->completed_jobs;
    uv_mutex_unlock(&p->mutex);

    return completed_jobs;
}

void completion_mark_complete_a_job(struct completion *p)
{
    uv_mutex_lock(&p->mutex);
    p->completed_jobs++;
    uv_cond_broadcast(&p->cond);
    uv_mutex_unlock(&p->mutex);
}

bool completion_is_done(struct completion *p)
{
    bool ret;
    uv_mutex_lock(&p->mutex);
    ret = p->completed;
    uv_mutex_unlock(&p->mutex);
    return ret;
}
