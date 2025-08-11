// SPDX-License-Identifier: GPL-3.0-or-later

#ifndef ML_QUEUE_H
#define ML_QUEUE_H

#include "ml_dimension.h"

#include <atomic>
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
} ml_queue_stats_t;

struct ml_queue_t {
    std::queue<ml_request_add_existing_model_t> add_model_queue;
    std::queue<ml_request_create_new_model_t> create_model_queue;
    ml_queue_stats_t stats;

    netdata_mutex_t mutex;
    netdata_cond_t cond_var;
    std::atomic<bool> exit;
};

ml_queue_t *ml_queue_init();

void ml_queue_destroy(ml_queue_t *q);

void ml_queue_push(ml_queue_t *q, const ml_queue_item_t req);

ml_queue_item_t ml_queue_pop(ml_queue_t *q);

ml_queue_size_t ml_queue_size(ml_queue_t *q);

ml_queue_stats_t ml_queue_stats(ml_queue_t *q);

void ml_queue_signal(ml_queue_t *q);

#endif /* ML_QUEUE_H */
