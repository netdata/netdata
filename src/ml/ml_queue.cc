// SPDX-License-Identifier: GPL-3.0-or-later

#include "ml/ml_queue.h"
#include "ml_private.h"

ml_queue_t *ml_queue_init()
{
    ml_queue_t *q = new ml_queue_t();

    netdata_mutex_init(&q->mutex);
    netdata_cond_init(&q->cond_var);
    q->exit = false;
    return q;
}

void ml_queue_destroy(ml_queue_t *q)
{
    netdata_mutex_destroy(&q->mutex);
    netdata_cond_destroy(&q->cond_var);
    delete q;
}

void ml_queue_push(ml_queue_t *q, const ml_queue_item_t req)
{
    netdata_mutex_lock(&q->mutex);

    switch (req.type) {
        case ML_QUEUE_ITEM_TYPE_CREATE_NEW_MODEL:
            q->create_model_queue.push(req.create_new_model);
            q->stats.total_create_new_model_requests_pushed += 1;
            break;

        case ML_QUEUE_ITEM_TYPE_ADD_EXISTING_MODEL:
            q->add_model_queue.push(req.add_existing_model);
            q->stats.total_add_existing_model_requests_pushed += 1;
            break;

        case ML_QUEUE_ITEM_STOP_REQUEST:
            // Stop requests don't need to be queued
            break;
    }

    netdata_cond_signal(&q->cond_var);
    netdata_mutex_unlock(&q->mutex);
}

ml_queue_item_t ml_queue_pop(ml_queue_t *q)
{
    netdata_mutex_lock(&q->mutex);

    ml_queue_item_t req;
    req.type = ML_QUEUE_ITEM_STOP_REQUEST;

    while (q->create_model_queue.empty() && q->add_model_queue.empty()) {
        netdata_cond_wait(&q->cond_var, &q->mutex);

        if (q->exit) {
            netdata_mutex_unlock(&q->mutex);
            return req;
        }
    }

    // Prioritize adding model requests
    if (!q->add_model_queue.empty()) {
        req.type = ML_QUEUE_ITEM_TYPE_ADD_EXISTING_MODEL;
        req.add_existing_model = q->add_model_queue.front();
        q->add_model_queue.pop();
        q->stats.total_add_existing_model_requests_popped += 1;
    } else if (!q->create_model_queue.empty()) {
        req.type = ML_QUEUE_ITEM_TYPE_CREATE_NEW_MODEL;
        req.create_new_model = q->create_model_queue.front();
        q->create_model_queue.pop();
        q->stats.total_create_new_model_requests_popped += 1;
    }

    netdata_mutex_unlock(&q->mutex);
    return req;
}

ml_queue_size_t ml_queue_size(ml_queue_t *q)
{
    netdata_mutex_lock(&q->mutex);
    ml_queue_size_t qs = ml_queue_size_t {
        q->create_model_queue.size(),
        q->add_model_queue.size(),
    };
    netdata_mutex_unlock(&q->mutex);

    return qs;
}

void ml_queue_signal(ml_queue_t *q)
{
    netdata_mutex_lock(&q->mutex);
    q->exit = true;
    netdata_cond_signal(&q->cond_var);
    netdata_mutex_unlock(&q->mutex);
}

ml_queue_stats_t ml_queue_stats(ml_queue_t *q)
{
    netdata_mutex_lock(&q->mutex);
    ml_queue_stats_t stats = q->stats;
    netdata_mutex_unlock(&q->mutex);

    return stats;
}
