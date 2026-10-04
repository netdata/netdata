// SPDX-License-Identifier: GPL-3.0-or-later

#include "ml/ml_queue.h"
#include "ml_private.h"

#include <algorithm>

ml_queue_t *ml_queue_init()
{
    ml_queue_t *q = new ml_queue_t();

    netdata_mutex_init(&q->mutex);
    netdata_cond_init(&q->cond_var);
    q->create_sorting = 0;
    q->waiters = 0;
    q->pass_key_fn = nullptr;
    q->pass_key_arg = nullptr;
    q->exit = false;
    return q;
}

void ml_queue_destroy(ml_queue_t *q)
{
    netdata_mutex_destroy(&q->mutex);
    netdata_cond_destroy(&q->cond_var);
    delete q;
}

void ml_queue_set_pass_key_fn(ml_queue_t *q, ml_queue_pass_key_fn fn, void *arg)
{
    netdata_mutex_lock(&q->mutex);
    q->pass_key_fn = fn;
    q->pass_key_arg = arg;
    netdata_mutex_unlock(&q->mutex);
}

void ml_queue_push(ml_queue_t *q, const ml_queue_item_t &req)
{
    netdata_mutex_lock(&q->mutex);

    switch (req.type) {
        case ML_QUEUE_ITEM_TYPE_CREATE_NEW_MODEL:
            q->create_next.push_back(ml_create_model_entry_t { req.create_new_model, 0 });
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

void ml_queue_sort_pass(std::deque<ml_create_model_entry_t> &pass, size_t *key0_entries)
{
    size_t key0 = 0;
    for (const auto &e : pass) {
        if (!e.key)
            key0++;
    }

    // not stable on purpose: equal keys are pages closing in the same second, whose relative order does not
    // matter to the extent cache, and std::sort needs no auxiliary buffer
    std::sort(pass.begin(), pass.end(), [](const ml_create_model_entry_t &a, const ml_create_model_entry_t &b) {
        return a.key < b.key;
    });

    if (key0_entries)
        *key0_entries = key0;
}

// Start a pass: called with the mutex held, create_current empty, create_next not empty and no other pass being
// sorted. Returns with the mutex held. Resolution and sorting run without the mutex, so pushes, the other consumers
// and the stop signal are never blocked behind them; a stop request ends the key resolution and skips the sort (a
// sort already running completes), and hands the entries back to create_next. Either way every waiting consumer is
// woken: there is a pass to consume, or a stop to return.
static void ml_queue_start_pass(ml_queue_t *q, ml_queue_pass_timing_t *timing)
{
    std::deque<ml_create_model_entry_t> pass;
    pass.swap(q->create_next);
    q->create_sorting = pass.size();

    ml_queue_pass_key_fn key_fn = q->pass_key_fn;
    void *key_arg = q->pass_key_arg;

    netdata_mutex_unlock(&q->mutex);

    usec_t resolve_started_ut = now_monotonic_usec();
    if (key_fn) {
        for (auto &e : pass) {
            if (q->exit.load(std::memory_order_relaxed))
                break;
            e.key = key_fn(e.req, key_arg);
        }
    }

    // checked after the resolution too, so a stop request that arrives during the last key callback does not
    // start the sort
    bool aborted = q->exit.load(std::memory_order_relaxed);

    usec_t sort_started_ut = now_monotonic_usec();
    size_t key0_entries = 0;

    if (!aborted) {
        if (key_fn)
            ml_queue_sort_pass(pass, &key0_entries);
        else
            key0_entries = pass.size();     // no key function: the pass keeps its push order
    }

    usec_t sort_finished_ut = now_monotonic_usec();

    netdata_mutex_lock(&q->mutex);
    q->create_sorting = 0;

    if (aborted) {
        // keep the entries for accounting; nothing consumes them after a stop request, so hand them back
        // without copying whenever nothing arrived during the scan (the common case)
        // (copying only the few arrivals of the scan window, never the pass itself)
        pass.insert(pass.end(), q->create_next.begin(), q->create_next.end());
        q->create_next.swap(pass);
        netdata_cond_broadcast(&q->cond_var);
        return;
    }

    q->create_current.swap(pass);

    q->stats.passes_sorted += 1;
    q->stats.pass_entries = q->create_current.size();
    q->stats.pass_key0_entries = key0_entries;

    if (timing) {
        timing->sorted = true;
        timing->resolve_ut = sort_started_ut - resolve_started_ut;
        timing->sort_ut = sort_finished_ut - sort_started_ut;
    }

    netdata_cond_broadcast(&q->cond_var);
}

ml_queue_item_t ml_queue_pop(ml_queue_t *q, ml_queue_pass_timing_t *timing)
{
    if (timing)
        *timing = ml_queue_pass_timing_t{};

    netdata_mutex_lock(&q->mutex);

    ml_queue_item_t req;
    req.type = ML_QUEUE_ITEM_STOP_REQUEST;

    while (true) {
        if (q->exit) {
            netdata_mutex_unlock(&q->mutex);
            return req;
        }

        // Prioritize adding model requests
        if (!q->add_model_queue.empty()) {
            req.type = ML_QUEUE_ITEM_TYPE_ADD_EXISTING_MODEL;
            req.add_existing_model = q->add_model_queue.front();
            q->add_model_queue.pop();
            q->stats.total_add_existing_model_requests_popped += 1;
            break;
        }

        if (!q->create_current.empty()) {
            req.type = ML_QUEUE_ITEM_TYPE_CREATE_NEW_MODEL;
            req.create_new_model = q->create_current.front().req;
            q->create_current.pop_front();
            q->stats.total_create_new_model_requests_popped += 1;
            break;
        }

        if (!q->create_next.empty() && !q->create_sorting) {
            // the current pass is over and nobody sorts the next one yet: this consumer does it (releases and
            // re-takes the mutex)
            ml_queue_start_pass(q, timing);
            continue;
        }

        // nothing to pop, or another consumer is sorting the next pass: wait for a push, the published pass or a
        // stop
        q->waiters++;
        netdata_cond_wait(&q->cond_var, &q->mutex);
        q->waiters--;
    }

    netdata_mutex_unlock(&q->mutex);
    return req;
}

ml_queue_size_t ml_queue_size(ml_queue_t *q)
{
    netdata_mutex_lock(&q->mutex);
    ml_queue_size_t qs = ml_queue_size_t {
        q->create_current.size() + q->create_next.size() + q->create_sorting,
        q->add_model_queue.size(),
    };
    netdata_mutex_unlock(&q->mutex);

    return qs;
}

void ml_queue_signal(ml_queue_t *q)
{
    netdata_mutex_lock(&q->mutex);
    q->exit = true;
    netdata_cond_broadcast(&q->cond_var);
    netdata_mutex_unlock(&q->mutex);
}

ml_queue_stats_t ml_queue_stats(ml_queue_t *q)
{
    netdata_mutex_lock(&q->mutex);
    ml_queue_stats_t stats = q->stats;
    netdata_mutex_unlock(&q->mutex);

    return stats;
}
