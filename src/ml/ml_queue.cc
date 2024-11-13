// SPDX-License-Identifier: GPL-3.0-or-later

#include "ml_private.h"

ml_queue_t * ml_queue_init()
{
    ml_queue_t *q = new ml_queue_t();

    netdata_mutex_init(&q->mutex);
    pthread_cond_init(&q->cond_var, NULL);
    q->exit = false;
    return q;
}

void ml_queue_destroy(ml_queue_t *q)
{
    netdata_mutex_destroy(&q->mutex);
    pthread_cond_destroy(&q->cond_var);
    delete q;
}

void ml_queue_push(ml_queue_t *q, const ml_queue_item_t req)
{
    netdata_mutex_lock(&q->mutex);
    q->internal.push(req);
    pthread_cond_signal(&q->cond_var);
    netdata_mutex_unlock(&q->mutex);
}

ml_queue_item_t ml_queue_pop(ml_queue_t *q)
{
    netdata_mutex_lock(&q->mutex);

    ml_queue_item_t req;
    req.type = ML_QUEUE_ITEM_STOP_REQUEST;

    while (q->internal.empty()) {
        pthread_cond_wait(&q->cond_var, &q->mutex);

        if (q->exit) {
            netdata_mutex_unlock(&q->mutex);
            return req;
        }
    }

    req = q->internal.front();
    q->internal.pop();

    netdata_mutex_unlock(&q->mutex);
    return req;
}

size_t ml_queue_size(ml_queue_t *q)
{
    netdata_mutex_lock(&q->mutex);
    size_t size = q->internal.size();
    netdata_mutex_unlock(&q->mutex);
    return size;
}

void ml_queue_signal(ml_queue_t *q)
{
    netdata_mutex_lock(&q->mutex);
    q->exit = true;
    pthread_cond_signal(&q->cond_var);
    netdata_mutex_unlock(&q->mutex);
}
