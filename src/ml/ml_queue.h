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

// One entry of the create-model pass containers. `key` is the capacity close time of the dimension's current
// tier-0 page, read when the pass is sorted (0: no usable future close - no page, idle page already past its
// close, not a dbengine dimension, or the dimension could not be resolved).
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

typedef struct {
    size_t total_create_new_model_requests_pushed;
    size_t total_create_new_model_requests_popped;

    size_t total_add_existing_model_requests_pushed;
    size_t total_add_existing_model_requests_popped;

    usec_t allotted_ut;
    usec_t consumed_ut;
    usec_t remaining_ut;

    size_t item_result_ok;
    size_t item_result_invalid_query_time_range;
    size_t item_result_not_enough_collected_values;
    size_t item_result_null_acquired_dimension;
    size_t item_result_chart_under_replication;

    // pass sorting (see ml_queue_pop()); the timings are cumulative, the counts are of the last sorted pass
    usec_t pass_resolve_ut;
    usec_t pass_sort_ut;
    size_t passes_sorted;
    size_t pass_entries;
    size_t pass_key0_entries;
} ml_queue_stats_t;

// The create-model requests of a worker are trained one PASS at a time, in ascending order of the capacity
// close time of each dimension's current tier-0 page (the order dbengine packs pages into extents), so that
// consecutive trainings load the same extents and the extent cache serves the siblings of every extent read.
//
//   create_next     - appended by ml_queue_push() (first enqueues and requeues); becomes the next pass
//   create_current  - the sorted pass being trained, consumed from the front (deque blocks are released as
//                     they empty, so the two containers together hold about one pass of entries)
//
// When create_current is empty and create_next is not, ml_queue_pop() (the worker is the only consumer) swaps
// create_next out under the mutex, resolves the key of every entry and sorts it OFF the mutex, and swaps the
// sorted pass into create_current under the mutex again. Entries pushed while a pass is being sorted land in
// the new create_next and belong to the following pass. Keys below the sort time are normalised to 0 and sort
// first, as one group. Add-model requests keep their own queue and are always served first.
struct ml_queue_t {
    std::queue<ml_request_add_existing_model_t> add_model_queue;
    std::deque<ml_create_model_entry_t> create_next;
    std::deque<ml_create_model_entry_t> create_current;
    size_t create_sorting;              // entries swapped out for sorting, counted in ml_queue_size()
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

ml_queue_item_t ml_queue_pop(ml_queue_t *q);

ml_queue_size_t ml_queue_size(ml_queue_t *q);

ml_queue_stats_t ml_queue_stats(ml_queue_t *q);

void ml_queue_signal(ml_queue_t *q);

void ml_queue_set_pass_key_fn(ml_queue_t *q, ml_queue_pass_key_fn fn, void *arg);

// The key function used by the workers: resolves the dimension and reads the capacity close of its current
// tier-0 page under the tier spinlock (src/ml/ml.cc).
time_t ml_queue_dimension_pass_key(const ml_request_create_new_model_t &req, void *arg);

// Sort a pass in place: keys below `now_s` become 0 and the entries are ordered by key. Exposed for tests.
void ml_queue_sort_pass(std::deque<ml_create_model_entry_t> &pass, time_t now_s, size_t *key0_entries);

#endif /* ML_QUEUE_H */
