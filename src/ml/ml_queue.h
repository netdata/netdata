// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_QUEUE_H
#define ML_QUEUE_H

#include "ml_dimension.h"

#include <atomic>
#include <deque>
#include <queue>

typedef struct ml_request_create_new_model {
    DimensionLookupInfo DLI;
} ml_request_create_new_model_t;

typedef struct ml_request_add_existing_model {
    DimensionLookupInfo DLI;

    ml_kmeans_inlined_t inlined_km;
} ml_request_add_existing_model_t;

typedef struct ml_queue_item {
    ml_queue_item_type type;
    ml_request_create_new_model_t create_new_model;
    ml_request_add_existing_model add_existing_model;
} ml_queue_item_t;

// One entry of the create-model pass containers. `key` is the local time at which the dimension's current tier-0
// page will complete, estimated when the pass is sorted as the time of the read plus the seconds of data the page
// still needs (0: no open page, not a dbengine dimension, or the dimension could not be resolved). It is measured
// from the dimension's own last stored point, so children whose data lags the parent's clock keep their place.
typedef struct ml_create_model_entry {
    ml_request_create_new_model_t req;
    time_t key;
} ml_create_model_entry_t;

// Called by the queue, without its mutex held, for every create-model entry when a pass is sorted. Returns the
// key for the entry (see ml_create_model_entry_t). Installed by ml_queue_set_pass_key_fn(); unit tests install
// their own. Without one every entry gets key 0 and the pass keeps its push order.
typedef time_t (*ml_queue_pass_key_fn)(const ml_request_create_new_model_t &req, void *arg);

typedef struct {
    size_t create_new_model;
    size_t add_exisiting_model;
} ml_queue_size_t;

// The queue's own counters (the per-thread training values are ml_worker_stats_t, ml_worker.h).
typedef struct {
    size_t total_create_new_model_requests_pushed;
    size_t total_create_new_model_requests_popped;

    size_t total_add_existing_model_requests_pushed;
    size_t total_add_existing_model_requests_popped;

    // pass sorting (see ml_queue_pop()): the number of passes sorted, and the counts of the last one; the time spent
    // resolving and sorting is accounted by the thread that did it (ml_queue_pass_timing_t)
    size_t passes_sorted;
    size_t pass_entries;
    size_t pass_key0_entries;
} ml_queue_stats_t;

// Filled by ml_queue_pop() when that call resolved and sorted a pass (`sorted` set), so only the consumer that
// did the work accounts for it.
typedef struct {
    bool sorted;
    usec_t resolve_ut;
    usec_t sort_ut;
} ml_queue_pass_timing_t;

// One queue serves all training threads. Its create-model requests are trained one PASS at a time, in ascending
// order of the local time at which each dimension's current tier-0 page will complete (the order dbengine packs pages
// into extents), so consecutive trainings load the same extents and the extent cache serves the siblings of every
// extent read. The threads draw nearby entries from the same sorted pass, so they work through the same part of it close
// together instead of each sweeping its own pass from a different starting point.
//
//   create_next     - appended by ml_queue_push() (first enqueues and requeues); becomes the next pass
//   create_current  - the sorted pass being trained, consumed from the front (deque blocks are released as
//                     they empty, so the two containers together hold about one pass of entries)
//
// When create_current is empty and create_next is not, the first consumer to see it in ml_queue_pop() becomes the
// sorter: it swaps create_next out under the mutex, resolves the key of every entry and sorts it OFF the mutex, and
// swaps the sorted pass into create_current under the mutex again. While a pass is being sorted (create_sorting is
// not zero) the other consumers serve add-model requests or wait; they never start a second sort. Entries pushed
// while a pass is being sorted land in the new create_next and belong to the following pass. An entry that is still
// being trained when its pass empties is requeued into whatever pass is collecting at that moment - normally the
// next one, but no bound is guaranteed. Key-0 entries (no open page) sort first, as one group. Add-model requests
// keep their own queue and are always served first.
struct ml_queue_t {
    std::queue<ml_request_add_existing_model_t> add_model_queue;
    std::deque<ml_create_model_entry_t> create_next;
    std::deque<ml_create_model_entry_t> create_current;
    size_t create_sorting;              // entries swapped out for sorting, counted in ml_queue_size(); not zero
                                        // while a pass is being sorted (a pass is never empty)
    size_t waiters;                     // consumers parked on cond_var (read by the tests)
    ml_queue_stats_t stats;

    ml_queue_pass_key_fn pass_key_fn;
    void *pass_key_arg;

    netdata_mutex_t mutex;
    netdata_cond_t cond_var;
    std::atomic<bool> exit;
};

ml_queue_t *ml_queue_init();

void ml_queue_destroy(ml_queue_t *q);

void ml_queue_push(ml_queue_t *q, const ml_queue_item_t &req);

// Blocks until an item is available or the queue is signalled (then returns ML_QUEUE_ITEM_STOP_REQUEST). Safe for
// any number of concurrent consumers. `timing` (optional) is set when this call sorted a pass.
ml_queue_item_t ml_queue_pop(ml_queue_t *q, ml_queue_pass_timing_t *timing = nullptr);

ml_queue_size_t ml_queue_size(ml_queue_t *q);

ml_queue_stats_t ml_queue_stats(ml_queue_t *q);

// Wakes every consumer; each ml_queue_pop() then returns ML_QUEUE_ITEM_STOP_REQUEST.
void ml_queue_signal(ml_queue_t *q);

void ml_queue_set_pass_key_fn(ml_queue_t *q, ml_queue_pass_key_fn fn, void *arg);

// The key function used by the workers: resolves the dimension and reads, under the tier spinlock, how many seconds
// of data its current tier-0 page still needs, returning the local time it will complete (src/ml/ml.cc).
time_t ml_queue_dimension_pass_key(const ml_request_create_new_model_t &req, void *arg);

// Sort a pass in place by key (key-0 entries first, as one group). Exposed for tests.
void ml_queue_sort_pass(std::deque<ml_create_model_entry_t> &pass, size_t *key0_entries);

#endif /* ML_QUEUE_H */
