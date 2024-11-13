// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_QUEUE_H
#define ML_QUEUE_H

#include "ml_dimension.h"

#include <atomic>
#include <queue>

typedef struct ml_request_create_new_model {
    DimensionLookupInfo DLI;

    // Creation time of request
    time_t request_time;

    // First/last entry of this dimension in DB
    // at the point the request was made
    time_t first_entry_on_request;
    time_t last_entry_on_request;
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

struct ml_queue_t {
    std::queue<ml_queue_item_t> internal;
    netdata_mutex_t mutex;
    pthread_cond_t cond_var;
    std::atomic<bool> exit;
};

typedef struct {
    size_t queue_size;
    size_t num_popped_items;

    usec_t allotted_ut;
    usec_t consumed_ut;
    usec_t remaining_ut;

    size_t item_result_ok;
    size_t item_result_invalid_query_time_range;
    size_t item_result_not_enough_collected_values;
    size_t item_result_null_acquired_dimension;
    size_t item_result_chart_under_replication;
} ml_queue_stats_t;

ml_queue_t *ml_queue_init();

void ml_queue_destroy(ml_queue_t *q);

void ml_queue_push(ml_queue_t *q, const ml_queue_item_t req);

ml_queue_item_t ml_queue_pop(ml_queue_t *q);

size_t ml_queue_size(ml_queue_t *q);

void ml_queue_signal(ml_queue_t *q);

#endif /* ML_QUEUE_H */
